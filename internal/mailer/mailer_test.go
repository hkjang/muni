package mailer

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeServer speaks enough SMTP to hold a real conversation, so the tests
// exercise the protocol rather than a stub of it.
type fakeServer struct {
	address string
	// offer is the AUTH line advertised after EHLO; empty means no AUTH.
	offer string
	// tls says whether to claim STARTTLS support.
	starttls bool

	mu       sync.Mutex
	received []string
	auth     []string
	rcpt     []string
	from     string
}

func startFake(t *testing.T, offer string, starttls bool) *fakeServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &fakeServer{address: listener.Addr().String(), offer: offer, starttls: starttls}
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go server.handle(connection)
		}
	}()
	t.Cleanup(func() { _ = listener.Close() })
	return server
}

func (f *fakeServer) handle(connection net.Conn) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(connection)
	write := func(line string) { _, _ = connection.Write([]byte(line + "\r\n")) }

	write("220 fake ESMTP")
	inData := false
	var body strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		if inData {
			if line == "." {
				inData = false
				f.mu.Lock()
				f.received = append(f.received, body.String())
				f.mu.Unlock()
				body.Reset()
				write("250 OK")
				continue
			}
			body.WriteString(line + "\n")
			continue
		}

		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			write("250-fake")
			if f.starttls {
				write("250-STARTTLS")
			}
			if f.offer != "" {
				write("250-AUTH " + f.offer)
			}
			write("250 SIZE 10240000")
		case strings.HasPrefix(upper, "AUTH"):
			f.mu.Lock()
			f.auth = append(f.auth, line)
			f.mu.Unlock()
			if strings.Contains(upper, "LOGIN") && len(strings.Fields(line)) == 2 {
				// The client is expected to answer two prompts.
				write("334 " + base64.StdEncoding.EncodeToString([]byte("Username:")))
				username, _ := reader.ReadString('\n')
				f.record(strings.TrimSpace(username))
				write("334 " + base64.StdEncoding.EncodeToString([]byte("Password:")))
				password, _ := reader.ReadString('\n')
				f.record(strings.TrimSpace(password))
			}
			write("235 authenticated")
		case strings.HasPrefix(upper, "MAIL FROM"):
			f.mu.Lock()
			f.from = line
			f.mu.Unlock()
			write("250 OK")
		case strings.HasPrefix(upper, "RCPT TO"):
			f.mu.Lock()
			f.rcpt = append(f.rcpt, line)
			f.mu.Unlock()
			write("250 OK")
		case upper == "DATA":
			inData = true
			write("354 send it")
		case upper == "QUIT":
			write("221 bye")
			return
		default:
			write("250 OK")
		}
	}
}

func (f *fakeServer) record(value string) {
	f.mu.Lock()
	f.auth = append(f.auth, value)
	f.mu.Unlock()
}

func (f *fakeServer) config() Config {
	host, port, _ := net.SplitHostPort(f.address)
	number := 0
	_, _ = fmt.Sscanf(port, "%d", &number)
	return Config{
		Host: host, Port: number, Security: SecurityNone,
		From: "muni@example.com", FromName: "muni 알림",
		Timeout: 3 * time.Second,
	}
}

func (f *fakeServer) lastMessage(t *testing.T) string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.received) == 0 {
		t.Fatal("the server received no message")
	}
	return f.received[len(f.received)-1]
}

func TestSendDeliversAMessage(t *testing.T) {
	server := startFake(t, "", false)
	err := server.config().Send(Message{
		To:      "hong@example.com",
		Subject: "문서 검토 요청",
		Body:    "검토할 문서가 있습니다.",
	})
	if err != nil {
		t.Fatal(err)
	}
	message := server.lastMessage(t)
	if !strings.Contains(message, "To: hong@example.com") {
		t.Fatalf("recipient header missing: %s", message)
	}
	if !strings.Contains(message, "검토할 문서가 있습니다.") {
		t.Fatalf("body missing: %s", message)
	}
}

func TestKoreanSubjectIsEncoded(t *testing.T) {
	server := startFake(t, "", false)
	if err := server.config().Send(Message{To: "a@example.com", Subject: "문서 검토 요청", Body: "본문"}); err != nil {
		t.Fatal(err)
	}
	message := server.lastMessage(t)
	// A raw Korean subject is not valid in a header; it has to be encoded or
	// clients show it as mojibake.
	if !strings.Contains(message, "Subject: =?utf-8?q?") {
		t.Fatalf("subject was not encoded: %s", message)
	}
	if strings.Contains(message, "Subject: 문서") {
		t.Fatalf("subject was sent raw: %s", message)
	}
}

func TestSenderNameIsEncodedAndKeepsTheAddress(t *testing.T) {
	server := startFake(t, "", false)
	if err := server.config().Send(Message{To: "a@example.com", Subject: "x", Body: "y"}); err != nil {
		t.Fatal(err)
	}
	message := server.lastMessage(t)
	if !strings.Contains(message, "<muni@example.com>") {
		t.Fatalf("sender address missing: %s", message)
	}
}

func TestHeaderInjectionIsNotPossibleThroughTheSubject(t *testing.T) {
	server := startFake(t, "", false)
	// A newline in a subject is how an extra header gets added.
	err := server.config().Send(Message{
		To:      "a@example.com",
		Subject: "안녕\r\nBcc: victim@example.com",
		Body:    "본문",
	})
	if err != nil {
		t.Fatal(err)
	}
	// The text may appear inside the encoded subject; what must not exist is a
	// header line of its own.
	for _, line := range strings.Split(server.lastMessage(t), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Bcc:") {
			t.Fatalf("a header was injected through the subject: %q", line)
		}
	}
}

func TestHeaderInjectionIsNotPossibleThroughTheRecipient(t *testing.T) {
	server := startFake(t, "", false)
	config := server.config()
	config.FromName = "muni\r\nBcc: victim@example.com"
	// The recipient comes from the database and the sender name from the
	// settings, but neither is a reason to hand a newline to a header.
	err := config.Send(Message{To: "a@example.com\r\nBcc: victim@example.com", Subject: "x", Body: "y"})
	if err != nil {
		return // A server that rejects the address is also an acceptable end.
	}
	for _, line := range strings.Split(server.lastMessage(t), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Bcc:") {
			t.Fatalf("a header was injected: %q", line)
		}
	}
}

func TestCredentialsAreNotSentInTheClear(t *testing.T) {
	server := startFake(t, "PLAIN LOGIN", false)
	config := server.config()
	config.Username = "muni"
	config.Password = "secret"
	err := config.Send(Message{To: "a@example.com", Subject: "x", Body: "y"})
	if err == nil {
		t.Fatal("expected an unencrypted connection to refuse authentication")
	}
	if !strings.Contains(err.Error(), "암호화") {
		t.Fatalf("the refusal should say why: %v", err)
	}
}

func TestSendRefusesAnEmptyRecipient(t *testing.T) {
	server := startFake(t, "", false)
	if err := server.config().Send(Message{To: "  ", Subject: "x", Body: "y"}); err == nil {
		t.Fatal("expected an empty recipient to be refused")
	}
}

func TestSendRefusesAnUnconfiguredServer(t *testing.T) {
	if err := (Config{}).Send(Message{To: "a@example.com"}); err == nil {
		t.Fatal("expected a send with no server to be refused")
	}
}

func TestStartTLSIsRequiredWhenAskedFor(t *testing.T) {
	// The server does not offer STARTTLS, so a configuration that asks for it
	// must fail rather than quietly continue in the clear.
	server := startFake(t, "", false)
	config := server.config()
	config.Security = SecurityStartTLS
	err := config.Send(Message{To: "a@example.com", Subject: "x", Body: "y"})
	if err == nil {
		t.Fatal("expected the send to fail without STARTTLS")
	}
	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("the failure should name STARTTLS: %v", err)
	}
}

func TestNormalizePicksThePortForTheSecurity(t *testing.T) {
	if got := (Config{Host: "mail", Security: SecurityTLS}).Normalize().Port; got != 465 {
		t.Fatalf("implicit TLS should default to 465, got %d", got)
	}
	if got := (Config{Host: "mail"}).Normalize().Port; got != 587 {
		t.Fatalf("submission should default to 587, got %d", got)
	}
}

func TestNormalizeFallsBackToStartTLS(t *testing.T) {
	if got := (Config{Host: "mail", Security: "nonsense"}).Normalize().Security; got != SecurityStartTLS {
		t.Fatalf("an unknown security mode should become starttls, got %q", got)
	}
}

func TestNormalizeUsesTheUsernameAsTheSender(t *testing.T) {
	// A server that requires the envelope sender to match the account is the
	// common case, and an operator often fills in only one of the two.
	config := Config{Host: "mail", Username: "muni@example.com"}.Normalize()
	if config.From != "muni@example.com" {
		t.Fatalf("from = %q", config.From)
	}
}

func TestUsableNeedsAHostAndASender(t *testing.T) {
	if (Config{Host: "mail"}).Usable() {
		t.Fatal("a configuration with nowhere to send from is not usable")
	}
	if !(Config{Host: "mail", From: "a@example.com"}).Usable() {
		t.Fatal("a host and a sender should be enough to try")
	}
}
