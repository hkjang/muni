package httpapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/mailer"
	"github.com/hkjang/muni/internal/settings"
)

// mailerFor builds a sender from the stored settings.
func mailerFor(all settings.All) mailer.Config {
	return mailer.Config{
		Host:       all.Mail.SMTPHost,
		Port:       all.Mail.SMTPPort,
		Username:   all.Mail.Username,
		Password:   all.Mail.Password,
		Security:   all.Mail.Security,
		From:       all.Mail.FromAddress,
		FromName:   all.Mail.FromName,
		SkipVerify: all.Mail.SkipTLSVerify,
		Timeout:    time.Duration(all.Mail.TimeoutSeconds) * time.Second,
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
		var lastKeyScan time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			if time.Since(lastKeyScan) >= keyExpiryScanInterval {
				lastKeyScan = time.Now()
				if err := s.remindExpiringAPIKeys(ctx); err != nil {
					s.logger.Warn("api key expiry reminders failed", "error", err)
				}
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
	userID       uuid.UUID
	email        string
	displayName  string
	kind         string
	title        string
	body         string
	resourceType string
	resourceID   *uuid.UUID
}

// flushNotificationMail sends what is waiting and reports how many mails
// left. One person gets one mail per pass however many notifications are
// waiting for them: a comment that mentions somebody twice, or a busy minute,
// is one message and not a burst that teaches them to filter muni out.
func (s *Server) flushNotificationMail(ctx context.Context) (int, error) {
	all, err := s.settings.GetAll(ctx, true)
	if err != nil {
		return 0, err
	}
	if !all.Mail.Enabled {
		return 0, nil
	}
	sender := mailerFor(all)
	if !sender.Usable() {
		return 0, nil
	}

	rows, err := s.db.Query(ctx, `
		SELECT n.id, u.id, u.email, u.display_name, n.type, n.title, n.body, n.resource_type, n.resource_id
		FROM notifications n JOIN users u ON u.id = n.user_id
		WHERE n.emailed_at IS NULL
			AND n.email_attempts < $1
			AND n.created_at > now() - make_interval(hours => $2)
			AND n.type = ANY($3)
			AND u.status = 'ACTIVE'
			AND coalesce(btrim(u.email), '') <> ''
		ORDER BY n.created_at LIMIT $4`,
		maxEmailAttempts, int(outboxHorizon.Hours()), mailedNotificationTypes(all.Mail.Notify), outboxBatch)
	if err != nil {
		return 0, err
	}
	pending := make([]pendingMail, 0, outboxBatch)
	for rows.Next() {
		var item pendingMail
		var resourceType *string
		if rows.Scan(&item.id, &item.userID, &item.email, &item.displayName, &item.kind,
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
	for _, bundle := range bundleByRecipient(pending) {
		ids := make([]uuid.UUID, 0, len(bundle))
		for _, item := range bundle {
			ids = append(ids, item.id)
		}
		// The attempt is counted before it is made: a send that fails in a way
		// that repeats would otherwise be retried every minute forever.
		if _, err := s.db.Exec(ctx, `UPDATE notifications SET email_attempts = email_attempts + 1 WHERE id = ANY($1)`, ids); err != nil {
			continue
		}
		message := mailer.Message{
			To:      bundle[0].email,
			Subject: mailSubject(bundle, all.General.ServiceName),
			Body:    notificationBody(bundle, all.General.ServiceName, all.Mail.BaseURL),
		}
		err := sender.Send(message)
		s.recordMailDelivery(ctx, mailDelivery{
			event: mailEventOf(bundle), recipient: message.To, subject: message.Subject,
			userID: &bundle[0].userID, notifications: len(bundle), err: err,
		})
		if err != nil {
			s.logger.Warn("notification mail was not delivered",
				"notifications", len(bundle), "error", err)
			continue
		}
		if _, err := s.db.Exec(ctx, `UPDATE notifications SET emailed_at = now() WHERE id = ANY($1)`, ids); err != nil {
			s.logger.Warn("notification was sent but not marked", "error", err)
		}
		sent++
	}
	return sent, nil
}

// bundleByRecipient groups what is waiting by the person it is for, keeping
// the order things happened in both across people and within one person's
// bundle.
func bundleByRecipient(pending []pendingMail) [][]pendingMail {
	index := map[uuid.UUID]int{}
	bundles := make([][]pendingMail, 0, len(pending))
	for _, item := range pending {
		at, seen := index[item.userID]
		if !seen {
			at = len(bundles)
			index[item.userID] = at
			bundles = append(bundles, nil)
		}
		bundles[at] = append(bundles[at], item)
	}
	return bundles
}

// mailSubject is the notification's own title when there is one, and a count
// when several are travelling together — the first title is kept so the
// subject still says what kind of thing is inside.
func mailSubject(bundle []pendingMail, serviceName string) string {
	if strings.TrimSpace(serviceName) == "" {
		serviceName = "muni"
	}
	if len(bundle) == 1 {
		return bundle[0].title
	}
	return fmt.Sprintf("[%s] %s 외 %d건", serviceName, bundle[0].title, len(bundle)-1)
}

// notificationBody writes the mail.
//
// Plain text, the reader's name, what happened, and a link if there is
// somewhere to point. Nothing from the document itself: a notification that
// carries content sends that content to whatever mail system the recipient
// forwards to. A bundle lists each notification in turn, oldest first.
func notificationBody(bundle []pendingMail, serviceName, baseURL string) string {
	if strings.TrimSpace(serviceName) == "" {
		serviceName = "muni"
	}
	var out strings.Builder
	fmt.Fprintf(&out, "%s님,\n\n", bundle[0].displayName)
	for index, item := range bundle {
		if len(bundle) > 1 {
			fmt.Fprintf(&out, "%d. %s\n", index+1, strings.TrimSpace(item.title))
		}
		out.WriteString(strings.TrimSpace(item.body))
		out.WriteString("\n")
		if link := notificationLink(baseURL, item.resourceType, item.resourceID); link != "" {
			out.WriteString(link + "\n")
		}
		if index < len(bundle)-1 {
			out.WriteString("\n")
		}
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
	case "API_KEY":
		return base + "/settings"
	default:
		return base
	}
}
