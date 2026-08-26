// Package mailer sends mail through an organisation's own SMTP server.
//
// muni is installed on a network that already has a mail server, and that is
// the only one it will talk to: there is no hosted sending service here and no
// outbound connection to anywhere the operator did not configure.
package mailer

import (
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

// Security is how the connection is protected.
const (
	// SecurityNone is a plain connection, for a server on the same network
	// that does not offer TLS.
	SecurityNone = "none"
	// SecurityStartTLS connects in the clear and upgrades, which is what most
	// corporate servers on port 587 expect.
	SecurityStartTLS = "starttls"
	// SecurityTLS connects with TLS from the first byte, usually on port 465.
	SecurityTLS = "tls"
)

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	Security string
	// From is the address the mail is sent as, and FromName what a reader
	// sees instead of it.
	From     string
	FromName string
	// SkipVerify accepts a certificate that does not verify. Internal mail
	// servers are often on a private certificate authority, and an operator
	// who cannot install the root should still be able to turn mail on.
	SkipVerify bool
	Timeout    time.Duration
}

// Message is one mail.
type Message struct {
	To      string
	Subject string
	// Body is plain text. muni sends nothing else: a notification that has to
	// render is a notification that can render wrong, and plain text reaches
	// every client and every screen reader.
	Body string
}

const defaultTimeout = 20 * time.Second

// Normalize fills in what an administrator left out.
func (c Config) Normalize() Config {
	c.Host = strings.TrimSpace(c.Host)
	c.Username = strings.TrimSpace(c.Username)
	c.From = strings.TrimSpace(c.From)
	c.FromName = strings.TrimSpace(c.FromName)
	c.Security = strings.ToLower(strings.TrimSpace(c.Security))
	switch c.Security {
	case SecurityNone, SecurityStartTLS, SecurityTLS:
	default:
		c.Security = SecurityStartTLS
	}
	if c.Port <= 0 || c.Port > 65535 {
		switch c.Security {
		case SecurityTLS:
			c.Port = 465
		default:
			c.Port = 587
		}
	}
	if c.Timeout <= 0 {
		c.Timeout = defaultTimeout
	}
	if c.From == "" {
		c.From = c.Username
	}
	return c
}

// Usable reports whether a send can be attempted at all.
func (c Config) Usable() bool {
	c = c.Normalize()
	return c.Host != "" && c.From != ""
}

var errNotConfigured = errors.New("메일 서버가 설정되지 않았습니다")

// Send delivers one message.
func (c Config) Send(message Message) error {
	config := c.Normalize()
	if !config.Usable() {
		return errNotConfigured
	}
	if strings.TrimSpace(message.To) == "" {
		return errors.New("받는 사람이 없습니다")
	}

	client, err := config.connect()
	if err != nil {
		return err
	}
	defer func() {
		// Quit is the polite ending; Close is what actually releases the
		// socket if the server has already gone away.
		if quitErr := client.Quit(); quitErr != nil {
			_ = client.Close()
		}
	}()

	if config.Username != "" {
		if err := config.authenticate(client); err != nil {
			return err
		}
	}
	if err := client.Mail(config.From); err != nil {
		return fmt.Errorf("보내는 주소를 거절했습니다: %w", err)
	}
	if err := client.Rcpt(message.To); err != nil {
		return fmt.Errorf("받는 주소를 거절했습니다: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte(config.compose(message))); err != nil {
		return err
	}
	return writer.Close()
}

func (c Config) connect() (*smtp.Client, error) {
	address := net.JoinHostPort(c.Host, fmt.Sprint(c.Port))
	dialer := &net.Dialer{Timeout: c.Timeout}

	if c.Security == SecurityTLS {
		connection, err := tls.DialWithDialer(dialer, "tcp", address, c.tlsConfig())
		if err != nil {
			return nil, fmt.Errorf("메일 서버에 연결하지 못했습니다: %w", err)
		}
		_ = connection.SetDeadline(time.Now().Add(c.Timeout))
		return smtp.NewClient(connection, c.Host)
	}

	connection, err := dialer.Dial("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("메일 서버에 연결하지 못했습니다: %w", err)
	}
	_ = connection.SetDeadline(time.Now().Add(c.Timeout))
	client, err := smtp.NewClient(connection, c.Host)
	if err != nil {
		return nil, err
	}
	if c.Security == SecurityStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			_ = client.Close()
			return nil, errors.New("메일 서버가 STARTTLS를 지원하지 않습니다. 보안 방식을 확인해 주세요")
		}
		if err := client.StartTLS(c.tlsConfig()); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("STARTTLS에 실패했습니다: %w", err)
		}
	}
	return client, nil
}

func (c Config) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: c.Host, InsecureSkipVerify: c.SkipVerify} //nolint:gosec // the operator opts in
}

// authenticate picks a mechanism the server actually offers.
//
// Go's PlainAuth refuses to send credentials over an unprotected connection,
// which is right, and several corporate servers only offer LOGIN — so both are
// implemented and both are held to the same rule.
func (c Config) authenticate(client *smtp.Client) error {
	encrypted := c.Security != SecurityNone
	if state, ok := client.TLSConnectionState(); ok && state.HandshakeComplete {
		encrypted = true
	}
	if !encrypted {
		return errors.New("암호화되지 않은 연결로는 계정 정보를 보내지 않습니다. STARTTLS 또는 TLS를 사용하세요")
	}

	_, parameters := client.Extension("AUTH")
	mechanisms := strings.ToUpper(parameters)
	switch {
	case strings.Contains(mechanisms, "PLAIN"):
		return client.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host))
	case strings.Contains(mechanisms, "LOGIN"):
		return client.Auth(&loginAuth{username: c.Username, password: c.Password, host: c.Host})
	default:
		// A server that advertises nothing usable still usually takes PLAIN.
		return client.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host))
	}
}

// loginAuth is the AUTH LOGIN exchange, which Go does not ship.
type loginAuth struct {
	username string
	password string
	host     string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS {
		return "", nil, errors.New("암호화되지 않은 연결에서는 LOGIN 인증을 사용하지 않습니다")
	}
	if server.Name != a.host {
		return "", nil, errors.New("예상하지 않은 메일 서버입니다")
	}
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	// The prompts are "Username:" and "Password:", but servers differ in case
	// and punctuation, so the reply is chosen by which one has been sent.
	switch strings.ToLower(strings.TrimRight(string(fromServer), ": ")) {
	case "username":
		return []byte(a.username), nil
	case "password":
		return []byte(a.password), nil
	}
	return nil, fmt.Errorf("메일 서버가 알 수 없는 정보를 요구했습니다: %q", string(fromServer))
}

// compose builds the message.
//
// Korean subjects and bodies are the normal case here, so the subject is
// encoded and the body is declared UTF-8 rather than being sent as bytes and
// hoped over.
func (c Config) compose(message Message) string {
	var out strings.Builder
	// Every value that reaches a header is flattened first. A newline in any
	// of them is how an extra header — a Bcc, say — gets added to a message
	// somebody else composed.
	from := oneLine(c.From)
	if c.FromName != "" {
		from = mime.QEncoding.Encode("utf-8", oneLine(c.FromName)) + " <" + oneLine(c.From) + ">"
	}
	header := textproto.MIMEHeader{}
	header.Set("From", from)
	header.Set("To", oneLine(message.To))
	header.Set("Subject", mime.QEncoding.Encode("utf-8", oneLine(message.Subject)))
	header.Set("Date", time.Now().Format(time.RFC1123Z))
	header.Set("MIME-Version", "1.0")
	header.Set("Content-Type", `text/plain; charset="utf-8"`)
	header.Set("Content-Transfer-Encoding", "8bit")
	header.Set("Auto-Submitted", "auto-generated")
	// Mail clients and out-of-office responders both read this; a notification
	// that triggers an auto-reply loop is worse than no notification.
	header.Set("X-Auto-Response-Suppress", "All")

	for _, key := range []string{"From", "To", "Subject", "Date", "MIME-Version",
		"Content-Type", "Content-Transfer-Encoding", "Auto-Submitted", "X-Auto-Response-Suppress"} {
		out.WriteString(key + ": " + header.Get(key) + "\r\n")
	}
	out.WriteString("\r\n")
	out.WriteString(strings.ReplaceAll(strings.ReplaceAll(message.Body, "\r\n", "\n"), "\n", "\r\n"))
	return out.String()
}

// oneLine keeps a header on one line: a newline in a subject is how a header
// gets injected.
func oneLine(value string) string {
	replacer := strings.NewReplacer("\r", " ", "\n", " ")
	return strings.TrimSpace(replacer.Replace(value))
}
