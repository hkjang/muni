package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"
)

// listNotifications returns what has happened that concerns this person.
//
// The rows have been written since the beginning and, until now, could only be
// read out of the database: there was no endpoint and no screen, so a review
// request reached its reader only if they happened to open the right document.
func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	limit := parseLimit(r.URL.Query().Get("limit"), 30)
	unreadOnly := r.URL.Query().Get("unread") == "true"

	rows, err := s.db.Query(r.Context(), `
		SELECT id, type, title, body, resource_type, resource_id, read_at, created_at
		FROM notifications
		WHERE user_id = $1 AND ($2 = false OR read_at IS NULL)
		ORDER BY created_at DESC LIMIT $3`, p.User.ID, unreadOnly, limit)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "알림을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0, limit)
	for rows.Next() {
		var id uuid.UUID
		var kind, title, body string
		var resourceType *string
		var resourceID *uuid.UUID
		var readAt *time.Time
		var created time.Time
		if rows.Scan(&id, &kind, &title, &body, &resourceType, &resourceID, &readAt, &created) == nil {
			items = append(items, map[string]any{
				"id": id, "type": kind, "title": title, "body": body,
				"resourceType": resourceType, "resourceId": resourceID,
				"readAt": readAt, "createdAt": created,
			})
		}
	}

	var unread int
	_ = s.db.QueryRow(r.Context(),
		`SELECT count(*) FROM notifications WHERE user_id=$1 AND read_at IS NULL`, p.User.ID).Scan(&unread)

	writeData(w, 200, map[string]any{"items": items, "unread": unread})
}

// readNotification marks one as read.
func (s *Server) readNotification(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	// The user id is part of the condition, not a check before it: a
	// notification belongs to one person and nobody else can mark it.
	result, err := s.db.Exec(r.Context(),
		`UPDATE notifications SET read_at=now() WHERE id=$1 AND user_id=$2 AND read_at IS NULL`, id, p.User.ID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "알림을 읽음 처리하지 못했습니다.")
		return
	}
	writeData(w, 200, map[string]any{"id": id, "changed": result.RowsAffected()})
}

// readAllNotifications clears the badge in one go.
func (s *Server) readAllNotifications(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	result, err := s.db.Exec(r.Context(),
		`UPDATE notifications SET read_at=now() WHERE user_id=$1 AND read_at IS NULL`, p.User.ID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "알림을 읽음 처리하지 못했습니다.")
		return
	}
	writeData(w, 200, map[string]any{"changed": result.RowsAffected()})
}
