package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// createAccount makes an account through the API and returns its id.
func createAccount(t *testing.T, srv *serverUnderTest, email, name string) uuid.UUID {
	t.Helper()
	status, data := postJSON(t, srv.admin, srv.URL+"/api/v1/admin/users",
		map[string]any{"email": email, "displayName": name})
	if status != 201 {
		t.Fatalf("create %s = %d %v", email, status, data)
	}
	id, err := uuid.Parse(data["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestOffboardingHandsOverEverythingAtOnce(t *testing.T) {
	srv := newServerUnderTest(t)
	ctx := context.Background()
	leaver := createAccount(t, srv, "leaver@example.com", "떠나는사람")
	successor := createAccount(t, srv, "successor@example.com", "인계받는사람")

	// Two documents in the leaver's own workspace, one of them in the trash.
	var workspaceID uuid.UUID
	if err := srv.db.QueryRow(ctx, `SELECT id FROM workspaces WHERE owner_id=$1`, leaver).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	live := uuid.New()
	trashed := uuid.New()
	for id, deleted := range map[uuid.UUID]bool{live: false, trashed: true} {
		deletedAt := "NULL"
		if deleted {
			deletedAt = "now()"
		}
		if _, err := srv.db.Exec(ctx, `INSERT INTO documents(id,workspace_id,owner_id,title,deleted_at)
			VALUES($1,$2,$3,'인계 대상',`+deletedAt+`)`, id, workspaceID, leaver); err != nil {
			t.Fatal(err)
		}
	}
	// An open session and a live API key.
	if _, err := srv.db.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '1 day')`,
		[]byte("0123456789abcdef0123456789abcdef"), leaver); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.db.Exec(ctx, `INSERT INTO api_keys(user_id,name,prefix,secret_hash) VALUES($1,'남은 키','muni_leaver01',$2)`,
		leaver, []byte("hash")); err != nil {
		t.Fatal(err)
	}

	// What is held, before touching anything.
	resp, err := srv.admin.Get(srv.URL + "/api/v1/admin/users/" + leaver.String() + "/belongings")
	if err != nil {
		t.Fatal(err)
	}
	var held struct {
		Data struct {
			Counts belongings `json:"counts"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&held)
	resp.Body.Close()
	if held.Data.Counts.Documents != 1 || held.Data.Counts.TrashedDocuments != 1 ||
		held.Data.Counts.OpenSessions != 1 || held.Data.Counts.ActiveAPIKeys != 1 {
		t.Fatalf("belongings = %+v", held.Data.Counts)
	}

	status, data := postJSON(t, srv.admin, srv.URL+"/api/v1/admin/users/"+leaver.String()+"/offboard",
		map[string]any{
			"transferTo": successor.String(), "includeTrashed": true,
			"reassignApprovals": true, "revokeApiKeys": true,
			"endSessions": true, "suspend": true,
		})
	if status != 200 {
		t.Fatalf("offboard = %d %v", status, data)
	}

	var stillOwned, keysLeft, sessionsLeft int
	var accountStatus string
	_ = srv.db.QueryRow(ctx, `SELECT count(*) FROM documents WHERE owner_id=$1`, leaver).Scan(&stillOwned)
	_ = srv.db.QueryRow(ctx, `SELECT count(*) FROM api_keys WHERE user_id=$1 AND revoked_at IS NULL`, leaver).Scan(&keysLeft)
	_ = srv.db.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1`, leaver).Scan(&sessionsLeft)
	_ = srv.db.QueryRow(ctx, `SELECT status FROM users WHERE id=$1`, leaver).Scan(&accountStatus)
	if stillOwned != 0 || keysLeft != 0 || sessionsLeft != 0 || accountStatus != "SUSPENDED" {
		t.Fatalf("documents=%d keys=%d sessions=%d status=%s", stillOwned, keysLeft, sessionsLeft, accountStatus)
	}

	// The successor owns them, and can find them: owning a document in a
	// workspace you are not in is not owning it.
	var owned, member int
	_ = srv.db.QueryRow(ctx, `SELECT count(*) FROM documents WHERE owner_id=$1`, successor).Scan(&owned)
	_ = srv.db.QueryRow(ctx, `SELECT count(*) FROM workspace_members WHERE user_id=$1 AND workspace_id=$2`, successor, workspaceID).Scan(&member)
	if owned != 2 || member != 1 {
		t.Fatalf("successor owns %d documents, member rows %d", owned, member)
	}

	// The account stays. Everything that records what this person did still
	// points at a real row.
	var exists bool
	_ = srv.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, leaver).Scan(&exists)
	if !exists {
		t.Fatal("the account must survive: the audit log references it")
	}
}

func TestADepartedApproverStopsBlockingTheLine(t *testing.T) {
	srv := newServerUnderTest(t)
	ctx := context.Background()
	leaver := createAccount(t, srv, "approver@example.com", "떠나는부장")
	successor := createAccount(t, srv, "newboss@example.com", "새부장")
	author := createAccount(t, srv, "author@example.com", "기안자")

	var workspaceID uuid.UUID
	_ = srv.db.QueryRow(ctx, `SELECT id FROM workspaces WHERE owner_id=$1`, author).Scan(&workspaceID)
	documentID := uuid.New()
	if _, err := srv.db.Exec(ctx, `INSERT INTO documents(id,workspace_id,owner_id,title,workflow_status)
		VALUES($1,$2,$3,'결재 대기 문서','PENDING')`, documentID, workspaceID, author); err != nil {
		t.Fatal(err)
	}
	requestID := uuid.New()
	if _, err := srv.db.Exec(ctx, `INSERT INTO approval_requests(id,document_id,revision_no,requested_by,mode)
		VALUES($1,$2,1,$3,'SEQUENTIAL')`, requestID, documentID, author); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.db.Exec(ctx, `INSERT INTO approval_steps(request_id,position,approver_id) VALUES($1,1,$2)`,
		requestID, leaver); err != nil {
		t.Fatal(err)
	}

	// The document is stuck: only the person whose turn it is may act, and
	// they have left.
	resp, _ := srv.admin.Get(srv.URL + "/api/v1/admin/users/" + leaver.String() + "/belongings")
	var held struct {
		Data struct {
			Counts belongings `json:"counts"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&held)
	resp.Body.Close()
	if held.Data.Counts.BlockingApprovals != 1 {
		t.Fatalf("the blocked approval must be visible before it is fixed: %+v", held.Data.Counts)
	}

	status, data := postJSON(t, srv.admin, srv.URL+"/api/v1/admin/users/"+leaver.String()+"/offboard",
		map[string]any{"transferTo": successor.String(), "reassignApprovals": true, "suspend": true})
	if status != 200 {
		t.Fatalf("offboard = %d %v", status, data)
	}

	var approver uuid.UUID
	var stepStatus, requestStatus string
	_ = srv.db.QueryRow(ctx, `SELECT approver_id,status FROM approval_steps WHERE request_id=$1`, requestID).Scan(&approver, &stepStatus)
	_ = srv.db.QueryRow(ctx, `SELECT status FROM approval_requests WHERE id=$1`, requestID).Scan(&requestStatus)
	if approver != successor || stepStatus != "PENDING" || requestStatus != "PENDING" {
		t.Fatalf("approver=%v step=%s request=%s — the line should now wait on the successor", approver, stepStatus, requestStatus)
	}
}

func TestAnEmptiedApprovalLineIsCancelledNotApproved(t *testing.T) {
	// The successor is already on the line, so moving the departed step there
	// would put one person in it twice. Skipping the step empties the line —
	// and a line with no approvers has not approved anything.
	srv := newServerUnderTest(t)
	ctx := context.Background()
	leaver := createAccount(t, srv, "double@example.com", "떠나는사람")
	successor := createAccount(t, srv, "already@example.com", "이미결재선에있는사람")
	author := createAccount(t, srv, "writer@example.com", "기안자")

	var workspaceID uuid.UUID
	_ = srv.db.QueryRow(ctx, `SELECT id FROM workspaces WHERE owner_id=$1`, author).Scan(&workspaceID)
	documentID := uuid.New()
	_, _ = srv.db.Exec(ctx, `INSERT INTO documents(id,workspace_id,owner_id,title,workflow_status,status)
		VALUES($1,$2,$3,'문서','PENDING','REVIEW')`, documentID, workspaceID, author)
	requestID := uuid.New()
	_, _ = srv.db.Exec(ctx, `INSERT INTO approval_requests(id,document_id,revision_no,requested_by,mode)
		VALUES($1,$2,1,$3,'SEQUENTIAL')`, requestID, documentID, author)
	// Position 1 already decided by the successor; position 2 waits on the leaver.
	_, _ = srv.db.Exec(ctx, `INSERT INTO approval_steps(request_id,position,approver_id,status,decided_at)
		VALUES($1,1,$2,'APPROVED',now())`, requestID, successor)
	_, _ = srv.db.Exec(ctx, `INSERT INTO approval_steps(request_id,position,approver_id) VALUES($1,2,$2)`,
		requestID, leaver)

	status, data := postJSON(t, srv.admin, srv.URL+"/api/v1/admin/users/"+leaver.String()+"/offboard",
		map[string]any{"transferTo": successor.String(), "reassignApprovals": true, "suspend": true})
	if status != 200 {
		t.Fatalf("offboard = %d %v", status, data)
	}

	var requestStatus, workflowStatus, docStatus string
	_ = srv.db.QueryRow(ctx, `SELECT status FROM approval_requests WHERE id=$1`, requestID).Scan(&requestStatus)
	_ = srv.db.QueryRow(ctx, `SELECT workflow_status,status FROM documents WHERE id=$1`, documentID).Scan(&workflowStatus, &docStatus)
	if requestStatus != "CANCELLED" {
		t.Fatalf("request = %s; an emptied line must be cancelled, never approved", requestStatus)
	}
	if workflowStatus != "DRAFT" || docStatus != "DRAFT" {
		t.Fatalf("document = %s/%s; it must go back to something the author can resubmit", workflowStatus, docStatus)
	}
	// Nobody is left waiting on a person who is gone.
	var pending int
	_ = srv.db.QueryRow(ctx, `SELECT count(*) FROM approval_steps WHERE request_id=$1 AND status='PENDING'`, requestID).Scan(&pending)
	if pending != 0 {
		t.Fatalf("%d steps still pending", pending)
	}
}

func TestOffboardingRefusesTheCasesThatWouldBackfire(t *testing.T) {
	srv := newServerUnderTest(t)
	leaver := createAccount(t, srv, "refuse@example.com", "떠나는사람")
	suspended := createAccount(t, srv, "suspended@example.com", "정지된사람")
	if _, err := srv.db.Exec(context.Background(), `UPDATE users SET status='SUSPENDED' WHERE id=$1`, suspended); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		body   map[string]any
		status int
		code   string
		target uuid.UUID
	}{
		{"handing over to a suspended account", map[string]any{"transferTo": suspended.String()}, 409, "RECIPIENT_INACTIVE", leaver},
		{"handing over to themselves", map[string]any{"transferTo": leaver.String()}, 400, "SAME_USER", leaver},
		{"no recipient at all", map[string]any{}, 400, "RECIPIENT_REQUIRED", leaver},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, data := postJSON(t, srv.admin, srv.URL+"/api/v1/admin/users/"+c.target.String()+"/offboard", c.body)
			if status != c.status || data["_errorCode"] != c.code {
				t.Fatalf("= %d %v, want %d %s", status, data, c.status, c.code)
			}
		})
	}

	// Nothing moved on any of the refusals.
	var accountStatus string
	_ = srv.db.QueryRow(context.Background(), `SELECT status FROM users WHERE id=$1`, leaver).Scan(&accountStatus)
	if accountStatus != "ACTIVE" {
		t.Fatalf("a refused offboarding must not suspend anyone: %s", accountStatus)
	}
}

var _ = http.StatusOK
