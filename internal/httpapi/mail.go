package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/mailer"
	"github.com/hkjang/muni/internal/settings"
)

// mailEvent is one kind of thing muni mails about.
//
// The list is short on purpose. A mail is worth sending when its absence costs
// somebody something or keeps them refreshing a screen: a document waiting
// for their approval, the answer to a request they made, a question put to
// them by name, a key about to stop working. "Something changed" is not on
// the list — a first week of that and people write a rule that drops all of
// it, including the four that matter.
type mailEvent struct {
	// Type is the value in notifications.type.
	Type string
	// Key is the suffix of the settings switch, mail.notify_<Key>.
	Key   string
	Label string
	on    func(settings.MailNotify) bool
}

var mailEvents = []mailEvent{
	{Type: "APPROVAL_REQUEST", Key: "approval_request", Label: "검토·결재 요청 (내 차례가 됨)",
		on: func(n settings.MailNotify) bool { return n.ApprovalRequest }},
	{Type: "APPROVAL_DECISION", Key: "approval_decision", Label: "검토 결과 (승인·반려)",
		on: func(n settings.MailNotify) bool { return n.ApprovalDecision }},
	{Type: "MENTION", Key: "mention", Label: "댓글 멘션",
		on: func(n settings.MailNotify) bool { return n.Mention }},
	{Type: "API_KEY_EXPIRING", Key: "api_key_expiring", Label: "API 키 만료 임박",
		on: func(n settings.MailNotify) bool { return n.APIKeyExpiring }},
}

// mailedNotificationTypes is which notification types leave the building
// under the current switches. A type not in the catalogue is never mailed:
// the in-app list can grow without every new row becoming a message.
func mailedNotificationTypes(notify settings.MailNotify) []string {
	types := make([]string, 0, len(mailEvents))
	for _, event := range mailEvents {
		if event.on(notify) {
			types = append(types, event.Type)
		}
	}
	return types
}

// mailEventOf names what a mail was about in the delivery record: the one
// event when there is one, DIGEST when several travelled together.
func mailEventOf(bundle []pendingMail) string {
	if len(bundle) == 0 {
		return ""
	}
	for _, item := range bundle[1:] {
		if item.kind != bundle[0].kind {
			return "DIGEST"
		}
	}
	if len(bundle) > 1 {
		return "DIGEST"
	}
	return bundle[0].kind
}

// mailDelivery is one attempt to send one message, whatever came of it.
type mailDelivery struct {
	event         string
	recipient     string
	subject       string
	userID        *uuid.UUID
	notifications int
	err           error
}

// recordMailDelivery writes what was attempted. Every attempt is written, the
// ones that worked too: an administrator asked "did it go out?" needs a row
// either way. The body is not kept — subject and recipient answer the
// question, and a log that carried bodies would be a copy of everybody's
// mail.
func (s *Server) recordMailDelivery(ctx context.Context, delivery mailDelivery) {
	status, message := "sent", ""
	if delivery.err != nil {
		status, message = "failed", truncateRunes(delivery.err.Error(), 1000)
	}
	if delivery.notifications < 1 {
		delivery.notifications = 1
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO mail_deliveries(id,event,recipient,subject,user_id,notifications,status,error_message)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		uuid.New(), delivery.event, truncateRunes(delivery.recipient, 320), truncateRunes(delivery.subject, 300),
		delivery.userID, delivery.notifications, status, message); err != nil {
		s.logger.Warn("mail delivery was not recorded", "event", delivery.event, "error", err)
	}
}

// listMailDeliveries shows what left the building, newest first, with a
// count by outcome so the screen can say "3 failed" without reading the list.
func (s *Server) listMailDeliveries(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r.URL.Query().Get("limit"), 50)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	rows, err := s.db.Query(r.Context(), `
		SELECT d.id, d.event, d.recipient, d.subject, d.notifications, d.status, d.error_message, d.created_at
		FROM mail_deliveries d
		WHERE $1 = '' OR d.status = $1
		ORDER BY d.created_at DESC, d.id LIMIT $2`, status, limit)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "발송 기록을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0, limit)
	for rows.Next() {
		var id uuid.UUID
		var event, recipient, subject, state, message string
		var count int
		var created time.Time
		if rows.Scan(&id, &event, &recipient, &subject, &count, &state, &message, &created) == nil {
			items = append(items, map[string]any{
				"id": id, "event": event, "recipient": recipient, "subject": subject,
				"notifications": count, "status": state, "error": message, "createdAt": created,
			})
		}
	}
	summary := map[string]any{"total": 0, "sent": 0, "failed": 0}
	counts, err := s.db.Query(r.Context(), `SELECT status, count(*) FROM mail_deliveries GROUP BY 1`)
	if err == nil {
		defer counts.Close()
		total := 0
		for counts.Next() {
			var key string
			var count int
			if counts.Scan(&key, &count) == nil {
				summary[key] = count
				total += count
			}
		}
		summary["total"] = total
	}
	writeData(w, 200, map[string]any{"items": items, "summary": summary})
}

// testMail sends one message to the administrator asking for the test.
//
// A relay that is nearly configured looks exactly like one that is: the only
// way to know is to send something and see it arrive. The form's values are
// used as they stand so the relay can be proved before the settings are
// saved, and the attempt is recorded like any other.
func (s *Server) testMail(w http.ResponseWriter, r *http.Request) {
	var input settings.Mail
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Password == "" {
		// The form does not send a password back, so an unchanged one has to
		// come from what is stored.
		all, _ := s.settings.GetAll(r.Context(), true)
		input.Password = all.Mail.Password
	}
	p, _ := principalFrom(r.Context())
	recipient := strings.TrimSpace(p.User.Email)
	if recipient == "" {
		writeError(w, 409, "NO_ADMIN_EMAIL", "관리자 계정에 이메일 주소가 없어 시험 메일을 보낼 수 없습니다.")
		return
	}

	sender := mailerFor(settings.All{Mail: input})
	if !sender.Usable() {
		writeError(w, 400, "MAIL_CONFIG_REQUIRED", "메일 서버 주소와 보내는 주소가 필요합니다.")
		return
	}

	all, _ := s.settings.GetAll(r.Context(), false)
	serviceName := strings.TrimSpace(all.General.ServiceName)
	if serviceName == "" {
		serviceName = "muni"
	}
	subject := serviceName + " 메일 설정 시험"
	err := sender.Send(mailer.Message{
		To:      recipient,
		Subject: subject,
		Body: "이 메일이 도착했다면 " + serviceName +
			"이 사내 메일 서버로 알림을 보낼 수 있습니다.\n\n" +
			"보낸 서버: " + sender.Host + ":" + fmt.Sprint(sender.Port) +
			" (" + sender.Security + ")\n",
	})
	s.recordMailDelivery(r.Context(), mailDelivery{event: "TEST", recipient: recipient, subject: subject, userID: &p.User.ID, err: err})
	if err != nil {
		writeError(w, 502, "MAIL_TEST_FAILED", err.Error())
		return
	}
	s.audit(r, &p.User.ID, "TEST_MAIL", "SETTINGS", nil, map[string]any{"host": sender.Host})
	writeData(w, 200, map[string]any{"ok": true, "sentTo": recipient})
}

// keyExpiryScanInterval is how often expiring keys are looked for. A key
// expires on a day, not a minute, so an hour is plenty.
const keyExpiryScanInterval = time.Hour

// keyExpiryNotice is how far ahead of expiry the owner is told: long enough
// to issue a new key and swap it into whatever uses it, short enough that
// the mail is not forgotten before it matters.
const keyExpiryNotice = 7 * 24 * time.Hour

// remindExpiringAPIKeys writes one notification per API key that expires
// within the notice period. An expired key stops an integration silently —
// the person finds out when something downstream breaks — which is exactly
// the kind of thing a mail is for. Each key is told about once: the
// notification is keyed to it, and a second scan finds the first.
func (s *Server) remindExpiringAPIKeys(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO notifications(user_id, type, title, body, resource_type, resource_id)
		SELECT k.user_id, 'API_KEY_EXPIRING', 'API 키 만료 임박',
			'API 키 ''' || k.name || ''' (' || k.prefix || '…)이(가) ' || to_char(k.expires_at, 'YYYY-MM-DD') ||
			'에 만료됩니다. 계속 쓰려면 새 키를 발급하고 연동을 바꿔 주세요.',
			'API_KEY', k.id
		FROM api_keys k JOIN users u ON u.id = k.user_id
		WHERE k.revoked_at IS NULL
			AND k.expires_at IS NOT NULL
			AND k.expires_at > now()
			AND k.expires_at <= now() + make_interval(hours => $1)
			AND u.status = 'ACTIVE'
			AND NOT EXISTS (
				SELECT 1 FROM notifications n
				WHERE n.resource_type = 'API_KEY' AND n.resource_id = k.id AND n.type = 'API_KEY_EXPIRING')`,
		int(keyExpiryNotice.Hours()))
	return err
}
