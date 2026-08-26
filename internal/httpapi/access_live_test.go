package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type accessView struct {
	Data struct {
		Document struct {
			Visibility string `json:"visibility"`
		} `json:"document"`
		Entries []struct {
			UserID    uuid.UUID `json:"userId"`
			Role      string    `json:"role"`
			Via       string    `json:"via"`
			Suspended bool      `json:"suspended"`
			AlsoAdmin bool      `json:"alsoAdmin"`
		} `json:"entries"`
		Everyone struct {
			Applies bool   `json:"applies"`
			Role    string `json:"role"`
		} `json:"everyone"`
		Admins int      `json:"admins"`
		Notes  []string `json:"notes"`
	} `json:"data"`
}

func fetchAccess(t *testing.T, srv *serverUnderTest, documentID uuid.UUID) accessView {
	t.Helper()
	resp, err := srv.admin.Get(srv.URL + "/api/v1/admin/documents/" + documentID.String() + "/access")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("access = %d", resp.StatusCode)
	}
	var view accessView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	return view
}

// This is the test that matters. The access screen reimplements the precedence
// documentRole applies, in SQL instead of Go. If the two ever disagree the
// screen tells an administrator the wrong thing about who can read a document,
// which is worse than having no screen — so they are compared directly,
// against a real database, for every user and every visibility.
func TestTheAccessViewAgreesWithTheCodeThatEnforcesIt(t *testing.T) {
	srv := newServerUnderTest(t)
	ctx := context.Background()

	owner := createAccount(t, srv, "owner@example.com", "소유자")
	direct := createAccount(t, srv, "direct@example.com", "직접권한")
	teammate := createAccount(t, srv, "teammate@example.com", "같은워크스페이스")
	manager := createAccount(t, srv, "manager@example.com", "워크스페이스관리자")
	stranger := createAccount(t, srv, "stranger@example.com", "남")
	boss := createAccount(t, srv, "accessadmin@example.com", "관리자")
	if _, err := srv.db.Exec(ctx, `UPDATE users SET role='ADMIN' WHERE id=$1`, boss); err != nil {
		t.Fatal(err)
	}

	var workspaceID uuid.UUID
	_ = srv.db.QueryRow(ctx, `SELECT id FROM workspaces WHERE owner_id=$1`, owner).Scan(&workspaceID)
	for user, role := range map[uuid.UUID]string{teammate: "MEMBER", manager: "MANAGER"} {
		if _, err := srv.db.Exec(ctx,
			`INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,$3)`,
			workspaceID, user, role); err != nil {
			t.Fatal(err)
		}
	}

	documentID := uuid.New()
	if _, err := srv.db.Exec(ctx,
		`INSERT INTO documents(id,workspace_id,owner_id,title) VALUES($1,$2,$3,'권한 확인용')`,
		documentID, workspaceID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.db.Exec(ctx, `INSERT INTO document_permissions(document_id,subject_type,subject_id,role,created_by)
		VALUES($1,'USER',$2,'COMMENTER',$3)`, documentID, direct, owner); err != nil {
		t.Fatal(err)
	}

	everybody := map[string]uuid.UUID{
		"owner": owner, "direct": direct, "teammate": teammate,
		"manager": manager, "stranger": stranger,
	}

	for _, visibility := range []string{"RESTRICTED", "WORKSPACE", "ORGANIZATION"} {
		t.Run(visibility, func(t *testing.T) {
			if _, err := srv.db.Exec(ctx, `UPDATE documents SET visibility=$2 WHERE id=$1`, documentID, visibility); err != nil {
				t.Fatal(err)
			}
			view := fetchAccess(t, srv, documentID)
			listed := map[uuid.UUID]string{}
			for _, entry := range view.Data.Entries {
				listed[entry.UserID] = entry.Role
			}

			for name, userID := range everybody {
				var user User
				if err := srv.db.QueryRow(ctx,
					`SELECT id,username,email,display_name,role,status,locale,created_at FROM users WHERE id=$1`, userID).
					Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role,
						&user.Status, &user.Locale, &user.CreatedAt); err != nil {
					t.Fatal(err)
				}
				actual, err := srv.api.documentRole(ctx, user, documentID, false)
				if errors.Is(err, errForbidden) {
					actual = ""
				} else if err != nil {
					t.Fatal(err)
				}

				claimed, isListed := listed[userID]
				if !isListed && view.Data.Everyone.Applies {
					claimed = view.Data.Everyone.Role
					isListed = true
				}
				if actual == "" && isListed {
					t.Errorf("%s: the screen says %s but documentRole refuses", name, claimed)
				}
				if actual != "" && !isListed {
					t.Errorf("%s: documentRole grants %s but the screen does not list them", name, actual)
				}
				if actual != "" && isListed && actual != claimed {
					t.Errorf("%s: the screen says %s, documentRole gives %s", name, claimed, actual)
				}
			}
		})
	}
}

func TestAdministratorsAreCountedNotListed(t *testing.T) {
	// An administrator opens every document in the installation. Putting each
	// of them on every document's list would make every document look
	// individually over-shared, when what is true is a property of the role.
	srv := newServerUnderTest(t)
	ctx := context.Background()
	owner := createAccount(t, srv, "listowner@example.com", "소유자")
	var workspaceID uuid.UUID
	_ = srv.db.QueryRow(ctx, `SELECT id FROM workspaces WHERE owner_id=$1`, owner).Scan(&workspaceID)
	documentID := uuid.New()
	_, _ = srv.db.Exec(ctx, `INSERT INTO documents(id,workspace_id,owner_id,title) VALUES($1,$2,$3,'문서')`,
		documentID, workspaceID, owner)

	view := fetchAccess(t, srv, documentID)
	if view.Data.Admins < 1 {
		t.Fatalf("admins = %d; the bootstrap administrator exists", view.Data.Admins)
	}
	for _, entry := range view.Data.Entries {
		if entry.Via == "ADMIN" {
			t.Fatal("administrators must be counted, not enumerated per document")
		}
	}
}

func TestAnExpiredGrantIsNotAccess(t *testing.T) {
	srv := newServerUnderTest(t)
	ctx := context.Background()
	owner := createAccount(t, srv, "expowner@example.com", "소유자")
	guest := createAccount(t, srv, "guest@example.com", "기간만료손님")
	var workspaceID uuid.UUID
	_ = srv.db.QueryRow(ctx, `SELECT id FROM workspaces WHERE owner_id=$1`, owner).Scan(&workspaceID)
	documentID := uuid.New()
	_, _ = srv.db.Exec(ctx, `INSERT INTO documents(id,workspace_id,owner_id,title) VALUES($1,$2,$3,'문서')`,
		documentID, workspaceID, owner)
	if _, err := srv.db.Exec(ctx, `INSERT INTO document_permissions(document_id,subject_type,subject_id,role,created_by,expires_at)
		VALUES($1,'USER',$2,'VIEWER',$3, now() - interval '1 day')`, documentID, guest, owner); err != nil {
		t.Fatal(err)
	}
	view := fetchAccess(t, srv, documentID)
	for _, entry := range view.Data.Entries {
		if entry.UserID == guest {
			t.Fatal("a grant that has expired is not access")
		}
	}
}

func TestOwningBeatsADirectGrant(t *testing.T) {
	// The owner also holding a VIEWER row must not be reported as a viewer:
	// documentRole checks ownership first, so that is what they get.
	srv := newServerUnderTest(t)
	ctx := context.Background()
	owner := createAccount(t, srv, "bothowner@example.com", "소유자")
	var workspaceID uuid.UUID
	_ = srv.db.QueryRow(ctx, `SELECT id FROM workspaces WHERE owner_id=$1`, owner).Scan(&workspaceID)
	documentID := uuid.New()
	_, _ = srv.db.Exec(ctx, `INSERT INTO documents(id,workspace_id,owner_id,title) VALUES($1,$2,$3,'문서')`,
		documentID, workspaceID, owner)
	_, _ = srv.db.Exec(ctx, `INSERT INTO document_permissions(document_id,subject_type,subject_id,role,created_by)
		VALUES($1,'USER',$2,'VIEWER',$2)`, documentID, owner)

	view := fetchAccess(t, srv, documentID)
	var found int
	for _, entry := range view.Data.Entries {
		if entry.UserID == owner {
			found++
			if entry.Role != "OWNER" || entry.Via != "OWNER" {
				t.Fatalf("owner reported as %s via %s", entry.Role, entry.Via)
			}
		}
	}
	if found != 1 {
		t.Fatalf("the owner appears %d times; one person is one row", found)
	}
}

func TestLinkVisibilityIsReportedAsTheNothingItIs(t *testing.T) {
	// LINK passes validation and is gated behind an admin setting, but no code
	// grants access on it and no route serves a document by link. The screen
	// must neither invent a hole nor stay quiet about the setting doing
	// nothing.
	srv := newServerUnderTest(t)
	ctx := context.Background()
	owner := createAccount(t, srv, "linkowner@example.com", "소유자")
	var workspaceID uuid.UUID
	_ = srv.db.QueryRow(ctx, `SELECT id FROM workspaces WHERE owner_id=$1`, owner).Scan(&workspaceID)
	documentID := uuid.New()
	_, _ = srv.db.Exec(ctx, `INSERT INTO documents(id,workspace_id,owner_id,title,visibility)
		VALUES($1,$2,$3,'문서','LINK')`, documentID, workspaceID, owner)

	view := fetchAccess(t, srv, documentID)
	if view.Data.Everyone.Applies {
		t.Fatal("LINK grants nobody anything today; claiming otherwise invents a leak")
	}
	var warned bool
	for _, note := range view.Data.Notes {
		if len(note) > 0 && note != "" {
			warned = true
		}
	}
	if !warned {
		t.Fatal("an administrator who set LINK should be told it does nothing")
	}
}
