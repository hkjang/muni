package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/settings"
)

// These prove the five rules of the mail standard against a real database and
// a real SMTP conversation: nothing blocks a request, every attempt is
// recorded, the user table is the only directory, the password is never read
// back, and a bundle is one mail. A relay that is switched off or dead is the
// interesting case, not the one that works.

// fakeRelay speaks enough SMTP to accept a message and keep it.
type fakeRelay struct {
	address  string
	mu       sync.Mutex
	messages []string
}

func startFakeRelay(t *testing.T) *fakeRelay {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	relay := &fakeRelay{address: listener.Addr().String()}
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go relay.handle(connection)
		}
	}()
	t.Cleanup(func() { _ = listener.Close() })
	return relay
}

func (f *fakeRelay) handle(connection net.Conn) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(connection)
	write := func(line string) { _, _ = connection.Write([]byte(line + "\r\n")) }
	write("220 relay.internal ESMTP")
	var body strings.Builder
	inData := false
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
				f.messages = append(f.messages, body.String())
				f.mu.Unlock()
				body.Reset()
				write("250 queued")
				continue
			}
			body.WriteString(line + "\n")
			continue
		}
		switch upper := strings.ToUpper(line); {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			write("250-relay.internal")
			write("250 SIZE 10240000")
		case upper == "DATA":
			inData = true
			write("354 go ahead")
		case upper == "QUIT":
			write("221 bye")
			return
		default:
			write("250 OK")
		}
	}
}

func (f *fakeRelay) hostPort() (string, int) {
	host, port, _ := net.SplitHostPort(f.address)
	number := 0
	_, _ = fmt.Sscanf(port, "%d", &number)
	return host, number
}

func (f *fakeRelay) received() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.messages...)
}

// mailUnderTest is a live server with a fresh delivery log and, on cleanup,
// mail switched back off so the next test starts where a new install does.
func mailUnderTest(t *testing.T) (*serverUnderTest, uuid.UUID) {
	t.Helper()
	srv := newServerUnderTest(t)
	ctx := context.Background()
	var adminID uuid.UUID
	if err := srv.db.QueryRow(ctx, `SELECT id FROM users WHERE email='admin@muni.local'`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	reset := func() {
		for _, statement := range []string{
			`DELETE FROM mail_deliveries`,
			`DELETE FROM notifications WHERE type='API_KEY_EXPIRING'`,
			`DELETE FROM app_settings WHERE key LIKE 'mail.%' OR key LIKE 'smtp.%'`,
		} {
			if _, err := srv.db.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
		}
	}
	reset()
	t.Cleanup(reset)
	return srv, adminID
}

// enableMail points the stored settings at a relay.
func enableMail(t *testing.T, srv *serverUnderTest, actor uuid.UUID, host string, port int, change func(*settings.Mail)) {
	t.Helper()
	ctx := context.Background()
	all, err := srv.api.settings.GetAll(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	all.Mail = settings.Mail{Enabled: true, SMTPHost: host, SMTPPort: port, Security: "none",
		FromAddress: "muni@example.com", FromName: "muni", BaseURL: "https://muni.example.com",
		TimeoutSeconds: 2, Notify: settings.AllMailNotifications()}
	if change != nil {
		change(&all.Mail)
	}
	if err := srv.api.settings.Save(ctx, all, actor); err != nil {
		t.Fatal(err)
	}
}

func notify(t *testing.T, srv *serverUnderTest, user uuid.UUID, kind, title string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := srv.db.Exec(context.Background(),
		`INSERT INTO notifications(id,user_id,type,title,body,resource_type,resource_id) VALUES($1,$2,$3,$4,'본문','DOCUMENT',$5)`,
		id, user, kind, title, uuid.New()); err != nil {
		t.Fatal(err)
	}
	return id
}

func deliveries(t *testing.T, srv *serverUnderTest) (items []map[string]any, summary map[string]any) {
	t.Helper()
	resp, err := srv.admin.Get(srv.URL + "/api/v1/admin/mail/deliveries")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Data struct {
			Items   []map[string]any `json:"items"`
			Summary map[string]any   `json:"summary"`
		} `json:"data"`
	}
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("deliveries = %d: %s", resp.StatusCode, raw)
	}
	_ = json.Unmarshal(raw, &out)
	return out.Data.Items, out.Data.Summary
}

func TestMailOutboxBundlesPerPersonAndRecordsEverySend(t *testing.T) {
	srv, admin := mailUnderTest(t)
	ctx := context.Background()
	relay := startFakeRelay(t)
	host, port := relay.hostPort()
	enableMail(t, srv, admin, host, port, nil)
	hong := createAccount(t, srv, "hong@example.com", "홍길동")

	// Two mentions and one approval for one person, one decision for another.
	first := notify(t, srv, hong, "MENTION", "문서 댓글에서 회원님을 멘션했습니다.")
	notify(t, srv, hong, "MENTION", "문서 댓글에서 회원님을 멘션했습니다.")
	notify(t, srv, hong, "APPROVAL_REQUEST", "문서 검토 요청")
	notify(t, srv, admin, "APPROVAL_DECISION", "문서 검토 결과")

	sent, err := srv.api.flushNotificationMail(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sent != 2 {
		t.Fatalf("two people, two mails; got %d", sent)
	}
	messages := relay.received()
	if len(messages) != 2 {
		t.Fatalf("the relay should have two messages, got %d", len(messages))
	}
	var hongMail string
	for _, message := range messages {
		if strings.Contains(message, "To: hong@example.com") {
			hongMail = message
		}
	}
	if hongMail == "" {
		t.Fatalf("no mail reached hong: %v", messages)
	}
	// Three notifications travelled as one message, listed in order.
	if !strings.Contains(hongMail, "1. 문서 댓글에서") || !strings.Contains(hongMail, "3. 문서 검토 요청") {
		t.Fatalf("hong's mail should list all three: %s", hongMail)
	}
	var emailed *time.Time
	var attempts int
	if err := srv.db.QueryRow(ctx, `SELECT emailed_at, email_attempts FROM notifications WHERE id=$1`, first).Scan(&emailed, &attempts); err != nil {
		t.Fatal(err)
	}
	if emailed == nil || attempts != 1 {
		t.Fatalf("a sent notification is marked once: emailed=%v attempts=%d", emailed, attempts)
	}

	items, summary := deliveries(t, srv)
	if len(items) != 2 || summary["sent"].(float64) != 2 || summary["failed"].(float64) != 0 {
		t.Fatalf("every send is recorded: %v %v", items, summary)
	}
	for _, item := range items {
		if item["status"] != "sent" {
			t.Fatalf("delivery should be sent: %v", item)
		}
		if item["recipient"] == "hong@example.com" {
			if item["event"] != "DIGEST" || item["notifications"].(float64) != 3 {
				t.Fatalf("hong's record should say three notifications went as a digest: %v", item)
			}
			if !strings.Contains(item["subject"].(string), "외 2건") {
				t.Fatalf("the digest subject counts the rest: %v", item)
			}
		}
		if _, hasBody := item["body"]; hasBody {
			t.Fatalf("the delivery log must not carry bodies: %v", item)
		}
	}
	// Nothing is left waiting.
	if sent, _ := srv.api.flushNotificationMail(ctx); sent != 0 {
		t.Fatalf("a second pass has nothing to send, sent %d", sent)
	}
}

func TestMailStaysOffUntilAnAdministratorTurnsItOn(t *testing.T) {
	srv, admin := mailUnderTest(t)
	relay := startFakeRelay(t)
	host, port := relay.hostPort()
	// Fully configured but not enabled: a new install with the form filled
	// in and the switch left alone.
	enableMail(t, srv, admin, host, port, func(m *settings.Mail) { m.Enabled = false })
	hong := createAccount(t, srv, "hong@example.com", "홍길동")
	notify(t, srv, hong, "APPROVAL_REQUEST", "문서 검토 요청")

	if sent, err := srv.api.flushNotificationMail(context.Background()); err != nil || sent != 0 {
		t.Fatalf("nothing leaves while mail is off: sent=%d err=%v", sent, err)
	}
	if len(relay.received()) != 0 {
		t.Fatal("the relay was contacted while mail was off")
	}
	if items, _ := deliveries(t, srv); len(items) != 0 {
		t.Fatalf("nothing was attempted, nothing is recorded: %v", items)
	}
}

func TestMailEventSwitchStopsOnlyThatKind(t *testing.T) {
	srv, admin := mailUnderTest(t)
	relay := startFakeRelay(t)
	host, port := relay.hostPort()
	enableMail(t, srv, admin, host, port, func(m *settings.Mail) { m.Notify.Mention = false })
	hong := createAccount(t, srv, "hong@example.com", "홍길동")
	notify(t, srv, hong, "MENTION", "문서 댓글에서 회원님을 멘션했습니다.")
	notify(t, srv, hong, "APPROVAL_REQUEST", "문서 검토 요청")

	if sent, err := srv.api.flushNotificationMail(context.Background()); err != nil || sent != 1 {
		t.Fatalf("the approval still goes: sent=%d err=%v", sent, err)
	}
	messages := relay.received()
	if len(messages) != 1 {
		t.Fatalf("only the approval should have gone: %v", messages)
	}
	// The subject is Q-encoded on the wire; the delivery record holds it plain.
	items, _ := deliveries(t, srv)
	if len(items) != 1 || items[0]["event"] != "APPROVAL_REQUEST" || items[0]["subject"] != "문서 검토 요청" {
		t.Fatalf("the record should show the approval alone: %v", items)
	}
	var waiting int
	if err := srv.db.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE user_id=$1 AND type='MENTION' AND emailed_at IS NULL AND email_attempts=0`, hong).Scan(&waiting); err != nil || waiting != 1 {
		t.Fatalf("the mention is left alone, not counted as an attempt: %d %v", waiting, err)
	}
}

func TestADeadRelayDoesNotFailTheRequestAndTheAttemptIsRecorded(t *testing.T) {
	srv, admin := mailUnderTest(t)
	ctx := context.Background()
	// A port with nothing listening.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, port := (&fakeRelay{address: listener.Addr().String()}).hostPort()
	_ = listener.Close()
	enableMail(t, srv, admin, host, port, nil)
	hong := createAccount(t, srv, "hong@example.com", "홍길동")

	// The request that produces a notification is a comment mentioning hong.
	var username string
	if err := srv.db.QueryRow(ctx, `SELECT username FROM users WHERE id=$1`, hong).Scan(&username); err != nil {
		t.Fatal(err)
	}
	var workspaceID uuid.UUID
	if err := srv.db.QueryRow(ctx, `SELECT id FROM workspaces WHERE owner_id=$1`, admin).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	status, doc := postJSON(t, srv.admin, srv.URL+"/api/v1/documents", map[string]any{"workspaceId": workspaceID, "title": "메일 시험"})
	if status != 200 {
		t.Fatalf("create document = %d %v", status, doc)
	}
	started := time.Now()
	status, comment := postJSON(t, srv.admin, srv.URL+"/api/v1/documents/"+doc["id"].(string)+"/comments",
		map[string]any{"body": "@" + username + " 확인 부탁드립니다"})
	if status != 201 {
		t.Fatalf("the comment must succeed whatever the relay is doing: %d %v", status, comment)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("the request waited on the relay: %s", time.Since(started))
	}

	// The background pass fails, says so, and leaves the notification to retry.
	if sent, err := srv.api.flushNotificationMail(ctx); err != nil || sent != 0 {
		t.Fatalf("a dead relay sends nothing: sent=%d err=%v", sent, err)
	}
	items, summary := deliveries(t, srv)
	if len(items) != 1 || items[0]["status"] != "failed" || items[0]["error"] == "" || summary["failed"].(float64) != 1 {
		t.Fatalf("the failed attempt is recorded with its reason: %v %v", items, summary)
	}
	if items[0]["recipient"] != "hong@example.com" || items[0]["event"] != "MENTION" {
		t.Fatalf("the record says who and what: %v", items[0])
	}
	var emailed *time.Time
	var attempts int
	if err := srv.db.QueryRow(ctx, `SELECT emailed_at, email_attempts FROM notifications WHERE user_id=$1 AND type='MENTION'`, hong).Scan(&emailed, &attempts); err != nil {
		t.Fatal(err)
	}
	if emailed != nil || attempts != 1 {
		t.Fatalf("the notification waits for a retry: emailed=%v attempts=%d", emailed, attempts)
	}
}

func TestMailPasswordIsNeverReadBack(t *testing.T) {
	srv, admin := mailUnderTest(t)
	ctx := context.Background()
	enableMail(t, srv, admin, "relay.internal", 25, func(m *settings.Mail) { m.Username = "muni"; m.Password = "relay-secret" })

	resp, err := srv.admin.Get(srv.URL + "/api/v1/admin/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(raw), "relay-secret") {
		t.Fatal("the settings API returned the relay password")
	}
	var out struct {
		Data struct {
			Mail settings.Mail `json:"mail"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &out)
	if !out.Data.Mail.PasswordSet || out.Data.Mail.Password != "" {
		t.Fatalf("the screen learns only that a password is set: %+v", out.Data.Mail)
	}

	// The stored keys are the standard's names, and the password is sealed.
	var kind string
	for _, key := range []string{"mail.enabled", "mail.smtp_host", "mail.smtp_port", "mail.security", "mail.from_address", "mail.notify_mention"} {
		if err := srv.db.QueryRow(ctx, `SELECT 'plain' FROM app_settings WHERE key=$1 AND is_secret=false`, key).Scan(&kind); err != nil {
			t.Fatalf("%s should be stored as a plain setting: %v", key, err)
		}
	}
	if err := srv.db.QueryRow(ctx, `SELECT 'secret' FROM app_settings WHERE key='mail.password' AND is_secret=true AND value IS NULL`).Scan(&kind); err != nil {
		t.Fatalf("the password should be stored sealed under mail.password: %v", err)
	}
}

func TestMailKeysMigrateFromTheirOldNames(t *testing.T) {
	srv, admin := mailUnderTest(t)
	ctx := context.Background()
	// An installation configured before the keys were renamed.
	for key, value := range map[string]string{"smtp.enabled": "true", "smtp.host": `"old-relay.internal"`, "smtp.port": "2525",
		"smtp.security": `"starttls"`, "smtp.from": `"noreply@example.com"`, "smtp.base_url": `"https://old.example.com"`} {
		if _, err := srv.db.Exec(ctx, `INSERT INTO app_settings(key,category,value,is_secret,updated_by,updated_at) VALUES($1,'smtp',$2::jsonb,false,$3,now())`, key, value, admin); err != nil {
			t.Fatal(err)
		}
	}
	sealed, err := srv.api.sealer.Seal([]byte("legacy-secret"), "setting:smtp.password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.db.Exec(ctx, `INSERT INTO app_settings(key,category,encrypted_value,is_secret,updated_by,updated_at) VALUES('smtp.password','smtp',$1,true,$2,now())`, sealed, admin); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../database/migrations/019_mail.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.db.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("the migration should be safe to run again: %v", err)
	}

	all, err := srv.api.settings.GetAll(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if !all.Mail.Enabled || all.Mail.SMTPHost != "old-relay.internal" || all.Mail.SMTPPort != 2525 || all.Mail.Security != "starttls" ||
		all.Mail.FromAddress != "noreply@example.com" || all.Mail.BaseURL != "https://old.example.com" {
		t.Fatalf("the old values should come back under the new names: %+v", all.Mail)
	}
	if !all.Mail.PasswordSet || all.Mail.Password != "legacy-secret" {
		t.Fatalf("the sealed password keeps working under its old row: set=%v", all.Mail.PasswordSet)
	}
	var stale int
	if err := srv.db.QueryRow(ctx, `SELECT count(*) FROM app_settings WHERE key LIKE 'smtp.%' AND key<>'smtp.password'`).Scan(&stale); err != nil {
		t.Fatal(err)
	}
	if stale != 0 {
		t.Fatalf("%d old rows were left behind", stale)
	}

	// Saving a new password retires the old row.
	all.Mail.Password = "new-secret"
	if err := srv.api.settings.Save(ctx, all, admin); err != nil {
		t.Fatal(err)
	}
	if err := srv.db.QueryRow(ctx, `SELECT count(*) FROM app_settings WHERE key='smtp.password'`).Scan(&stale); err != nil || stale != 0 {
		t.Fatalf("the old password row should be gone: %d %v", stale, err)
	}
	if again, _ := srv.api.settings.GetAll(ctx, true); again.Mail.Password != "new-secret" {
		t.Fatal("the new password should be read from mail.password")
	}
}

func TestTestMailSendsToTheAdministratorAndRecordsTheAttempt(t *testing.T) {
	srv, _ := mailUnderTest(t)
	relay := startFakeRelay(t)
	host, port := relay.hostPort()
	form := settings.Mail{SMTPHost: host, SMTPPort: port, Security: "auto", FromAddress: "muni@example.com"}
	status, data := postJSON(t, srv.admin, srv.URL+"/api/v1/admin/settings/test-mail", form)
	if status != 200 || data["sentTo"] != "admin@muni.local" {
		t.Fatalf("test mail = %d %v", status, data)
	}
	if messages := relay.received(); len(messages) != 1 || !strings.Contains(messages[0], "To: admin@muni.local") {
		t.Fatalf("the relay should hold the test message: %v", messages)
	}

	// A dead relay reports the failure in place and records it too.
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	deadHost, deadPort := (&fakeRelay{address: listener.Addr().String()}).hostPort()
	_ = listener.Close()
	form.SMTPHost, form.SMTPPort, form.TimeoutSeconds = deadHost, deadPort, 1
	if status, data = postJSON(t, srv.admin, srv.URL+"/api/v1/admin/settings/test-mail", form); status != 502 || data["_errorCode"] != "MAIL_TEST_FAILED" {
		t.Fatalf("a dead relay should be reported: %d %v", status, data)
	}
	items, summary := deliveries(t, srv)
	if len(items) != 2 || summary["sent"].(float64) != 1 || summary["failed"].(float64) != 1 {
		t.Fatalf("both attempts are recorded: %v %v", items, summary)
	}
	for _, item := range items {
		if item["event"] != "TEST" {
			t.Fatalf("a test send is recorded as one: %v", item)
		}
	}
}

func TestAnExpiringAPIKeyIsNotifiedOnce(t *testing.T) {
	srv, admin := mailUnderTest(t)
	ctx := context.Background()
	relay := startFakeRelay(t)
	host, port := relay.hostPort()
	enableMail(t, srv, admin, host, port, nil)
	hong := createAccount(t, srv, "hong@example.com", "홍길동")
	soon, later := uuid.New(), uuid.New()
	for id, expires := range map[uuid.UUID]time.Duration{soon: 3 * 24 * time.Hour, later: 30 * 24 * time.Hour} {
		if _, err := srv.db.Exec(ctx, `INSERT INTO api_keys(id,user_id,name,prefix,secret_hash,expires_at) VALUES($1,$2,'배치 연동',$3,'\x00',now()+$4::interval)`,
			id, hong, "muni_"+id.String()[:8], fmt.Sprintf("%d hours", int(expires.Hours()))); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _, _ = srv.db.Exec(ctx, `DELETE FROM api_keys WHERE id IN ($1,$2)`, soon, later) })

	for pass := 0; pass < 2; pass++ {
		if err := srv.api.remindExpiringAPIKeys(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := srv.db.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE type='API_KEY_EXPIRING' AND user_id=$1`, hong).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("the key expiring this week is told about exactly once, got %d", count)
	}
	if sent, err := srv.api.flushNotificationMail(ctx); err != nil || sent != 1 {
		t.Fatalf("the reminder is mailed: sent=%d err=%v", sent, err)
	}
	messages := relay.received()
	if len(messages) != 1 || !strings.Contains(messages[0], "배치 연동") || !strings.Contains(messages[0], "https://muni.example.com/settings") {
		t.Fatalf("the mail names the key and points at personal settings: %v", messages)
	}
	if !strings.Contains(messages[0], "To: hong@example.com") {
		t.Fatalf("the key's owner is the one told: %v", messages)
	}
}

func TestDeliveryLogNeedsAnAdministrator(t *testing.T) {
	srv, _ := mailUnderTest(t)
	resp, err := http.Get(srv.URL + "/api/v1/admin/mail/deliveries")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("anonymous = %d", resp.StatusCode)
	}
}
