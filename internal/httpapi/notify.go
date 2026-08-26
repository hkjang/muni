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

// mailerFor builds a sender from the stored settings.
func mailerFor(all settings.All) mailer.Config {
	return mailer.Config{
		Host:       all.SMTP.Host,
		Port:       all.SMTP.Port,
		Username:   all.SMTP.Username,
		Password:   all.SMTP.Password,
		Security:   all.SMTP.Security,
		From:       all.SMTP.From,
		FromName:   all.SMTP.FromName,
		SkipVerify: all.SMTP.SkipVerify,
	}.Normalize()
}

// outboxInterval is how often the waiting notifications are looked at. A
// minute is soon enough for a review request and rare enough that a mail
// server sees muni as a well-behaved client.
const outboxInterval = time.Minute

// outboxBatch bounds one pass, so a backlog is worked through steadily rather
// than as one long burst at whatever the mail server's rate limit is.
const outboxBatch = 20

// maxEmailAttempts stops muni retrying an address that will never accept mail.
const maxEmailAttempts = 3

// outboxHorizon is how far back a pass will look. A notification from last
// month is not worth sending now, and after an outage nobody wants a hundred
// of them at once.
const outboxHorizon = 24 * time.Hour

// StartNotificationMail sends the notifications muni already writes.
//
// Notifications existed from the beginning and never left the building, so a
// review request reached someone only if they happened to open muni. The
// events are already recorded, one row each, which makes the mail an outbox
// over that table rather than a second path bolted onto every handler: nothing
// blocks a request, a failure is retried, and a notification muni did not
// write does not get emailed.
func (s *Server) StartNotificationMail(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(outboxInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			if sent, err := s.flushNotificationMail(ctx); err != nil {
				s.logger.Warn("notification mail failed", "error", err)
			} else if sent > 0 {
				s.logger.Info("notification mail sent", "count", sent)
			}
		}
	}()
}

type pendingMail struct {
	id           uuid.UUID
	email        string
	displayName  string
	kind         string
	title        string
	body         string
	resourceType string
	resourceID   *uuid.UUID
}

func (s *Server) flushNotificationMail(ctx context.Context) (int, error) {
	all, err := s.settings.GetAll(ctx, true)
	if err != nil {
		return 0, err
	}
	if !all.SMTP.Enabled {
		return 0, nil
	}
	sender := mailerFor(all)
	if !sender.Usable() {
		return 0, nil
	}

	rows, err := s.db.Query(ctx, `
		SELECT n.id, u.email, u.display_name, n.type, n.title, n.body, n.resource_type, n.resource_id
		FROM notifications n JOIN users u ON u.id = n.user_id
		WHERE n.emailed_at IS NULL
			AND n.email_attempts < $1
			AND n.created_at > now() - make_interval(hours => $2)
			AND u.status = 'ACTIVE'
			AND coalesce(btrim(u.email), '') <> ''
		ORDER BY n.created_at LIMIT $3`,
		maxEmailAttempts, int(outboxHorizon.Hours()), outboxBatch)
	if err != nil {
		return 0, err
	}
	pending := make([]pendingMail, 0, outboxBatch)
	for rows.Next() {
		var item pendingMail
		var resourceType *string
		if rows.Scan(&item.id, &item.email, &item.displayName, &item.kind,
			&item.title, &item.body, &resourceType, &item.resourceID) == nil {
			if resourceType != nil {
				item.resourceType = *resourceType
			}
			pending = append(pending, item)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	sent := 0
	for _, item := range pending {
		// The attempt is counted before it is made: a send that fails in a way
		// that repeats would otherwise be retried every minute forever.
		if _, err := s.db.Exec(ctx, `UPDATE notifications SET email_attempts = email_attempts + 1 WHERE id=$1`, item.id); err != nil {
			continue
		}
		message := mailer.Message{
			To:      item.email,
			Subject: item.title,
			Body:    notificationBody(item, all.General.ServiceName, all.SMTP.BaseURL),
		}
		if err := sender.Send(message); err != nil {
			s.logger.Warn("notification mail was not delivered",
				"notification", item.id, "error", err)
			continue
		}
		if _, err := s.db.Exec(ctx, `UPDATE notifications SET emailed_at = now() WHERE id=$1`, item.id); err != nil {
			s.logger.Warn("notification was sent but not marked", "notification", item.id, "error", err)
		}
		sent++
	}
	return sent, nil
}

// notificationBody writes the mail.
//
// Plain text, the reader's name, what happened, and a link if there is
// somewhere to point. Nothing from the document itself: a notification that
// carries content sends that content to whatever mail system the recipient
// forwards to.
func notificationBody(item pendingMail, serviceName, baseURL string) string {
	if strings.TrimSpace(serviceName) == "" {
		serviceName = "muni"
	}
	var out strings.Builder
	fmt.Fprintf(&out, "%s님,\n\n", item.displayName)
	out.WriteString(strings.TrimSpace(item.body))
	out.WriteString("\n")

	if link := notificationLink(baseURL, item.resourceType, item.resourceID); link != "" {
		out.WriteString("\n" + link + "\n")
	}
	fmt.Fprintf(&out, "\n—\n%s에서 보낸 알림입니다. 이 메일에는 답장할 수 없습니다.\n", serviceName)
	return out.String()
}

func notificationLink(baseURL, resourceType string, resourceID *uuid.UUID) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" || resourceID == nil {
		return ""
	}
	switch strings.ToUpper(resourceType) {
	case "DOCUMENT":
		return base + "/docs/" + resourceID.String()
	case "WORKSPACE":
		return base + "/workspace/" + resourceID.String()
	default:
		return base
	}
}

// testSMTP sends one message to the administrator asking for the test.
//
// A mail server that is nearly configured looks exactly like one that is: the
// only way to know is to send something and see it arrive.
func (s *Server) testSMTP(w http.ResponseWriter, r *http.Request) {
	var input settings.SMTP
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Password == "" {
		// The form does not send a password back, so an unchanged one has to
		// come from what is stored.
		all, _ := s.settings.GetAll(r.Context(), true)
		input.Password = all.SMTP.Password
	}
	p, _ := principalFrom(r.Context())
	recipient := strings.TrimSpace(p.User.Email)
	if recipient == "" {
		writeError(w, 409, "NO_ADMIN_EMAIL", "관리자 계정에 이메일 주소가 없어 시험 메일을 보낼 수 없습니다.")
		return
	}

	sender := mailerFor(settings.All{SMTP: input})
	if !sender.Usable() {
		writeError(w, 400, "SMTP_CONFIG_REQUIRED", "메일 서버 주소와 보내는 주소가 필요합니다.")
		return
	}

	all, _ := s.settings.GetAll(r.Context(), false)
	serviceName := strings.TrimSpace(all.General.ServiceName)
	if serviceName == "" {
		serviceName = "muni"
	}
	err := sender.Send(mailer.Message{
		To:      recipient,
		Subject: serviceName + " 메일 설정 시험",
		Body: "이 메일이 도착했다면 " + serviceName +
			"이 사내 메일 서버로 알림을 보낼 수 있습니다.\n\n" +
			"보낸 서버: " + sender.Host + ":" + fmt.Sprint(sender.Port) +
			" (" + sender.Security + ")\n",
	})
	if err != nil {
		writeError(w, 502, "SMTP_TEST_FAILED", err.Error())
		return
	}
	s.audit(r, &p.User.ID, "TEST_SMTP", "SETTINGS", nil, map[string]any{"host": sender.Host})
	writeData(w, 200, map[string]any{"ok": true, "sentTo": recipient})
}
