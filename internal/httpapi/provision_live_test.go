package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/cryptoutil"
	"github.com/hkjang/muni/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These run against a real PostgreSQL, because what they are checking is a
// transaction across four tables and a constraint the schema enforces — a fake
// would only confirm that the fake agrees with itself. CI starts a container
// and sets MUNI_TEST_DSN; locally:
//
//	docker run -d --name muni-test -e POSTGRES_PASSWORD=muni -e POSTGRES_DB=muni -p 5433:5432 postgres:16-alpine
//	MUNI_TEST_DSN="postgres://postgres:muni@127.0.0.1:5433/muni?sslmode=disable" go test ./internal/httpapi/
//
// Without it they skip.
func liveServer(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("MUNI_TEST_DSN")
	if dsn == "" {
		t.Skip("no live database")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	sealer, err := cryptoutil.NewSealer(key)
	if err != nil {
		t.Fatal(err)
	}
	// Clear only the accounts these tests make. Truncating users cascades
	// into the settings row and the instance comes back with local login off.
	// workspaces.owner_id has no ON DELETE, so the workspaces go first.
	for _, statement := range []string{
		`DELETE FROM workspaces WHERE owner_id IN (SELECT id FROM users WHERE email LIKE '%@example.com')`,
		`DELETE FROM users WHERE email LIKE '%@example.com'`,
	} {
		if _, err := db.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Bootstrap(ctx, db, "admin@muni.local", "provision-check-1234", sealer); err != nil {
		t.Fatal(err)
	}
	api := New(db, sealer, BuildInfo{Version: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(func() { srv.Close(); db.Close() })
	return srv, db
}

func signIn(t *testing.T, srv *httptest.Server, identity, password string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	body, _ := json.Marshal(map[string]string{"identity": identity, "password": password})
	resp, err := client.Post(srv.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("login as %s = %d: %s", identity, resp.StatusCode, raw)
	}
	return client
}

func postJSON(t *testing.T, client *http.Client, url string, payload any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(payload)
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Data  map[string]any `json:"data"`
		Error map[string]any `json:"error"`
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	if out.Data == nil {
		out.Data = map[string]any{}
	}
	if out.Error != nil {
		out.Data["_errorCode"] = out.Error["code"]
	}
	return resp.StatusCode, out.Data
}

func TestAnAdministratorCanCreateAnAccountThatWorks(t *testing.T) {
	srv, db := liveServer(t)
	ctx := context.Background()
	admin := signIn(t, srv, "admin@muni.local", "provision-check-1234")

	status, data := postJSON(t, admin, srv.URL+"/api/v1/admin/users", map[string]any{
		"email": "minsu@example.com", "displayName": "김민수",
	})
	if status != 201 {
		t.Fatalf("create = %d %v", status, data)
	}
	temporary, _ := data["temporaryPassword"].(string)
	username, _ := data["username"].(string)
	if temporary == "" || username == "" {
		t.Fatalf("response = %v", data)
	}

	// The account is four rows, not one. Three of them make it able to work.
	var users, workspaces, members, keys int
	_ = db.QueryRow(ctx, `SELECT count(*) FROM users WHERE email='minsu@example.com'`).Scan(&users)
	_ = db.QueryRow(ctx, `SELECT count(*) FROM workspaces w JOIN users u ON u.id=w.owner_id WHERE u.email='minsu@example.com' AND w.kind='PERSONAL'`).Scan(&workspaces)
	_ = db.QueryRow(ctx, `SELECT count(*) FROM workspace_members m JOIN users u ON u.id=m.user_id WHERE u.email='minsu@example.com' AND m.role='OWNER'`).Scan(&members)
	_ = db.QueryRow(ctx, `SELECT count(*) FROM user_keys k JOIN users u ON u.id=k.user_id WHERE u.email='minsu@example.com' AND k.status='ACTIVE'`).Scan(&keys)
	if users != 1 || workspaces != 1 || members != 1 || keys != 1 {
		t.Fatalf("users=%d workspaces=%d members=%d keys=%d — an account missing any of these signs in and cannot do anything", users, workspaces, members, keys)
	}

	// The password muni chose actually signs in.
	newcomer := signIn(t, srv, "minsu@example.com", temporary)

	// And it can do exactly one thing until it is replaced.
	blocked, err := newcomer.Get(srv.URL + "/api/v1/workspaces")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(blocked.Body)
	blocked.Body.Close()
	if blocked.StatusCode != 403 || !strings.Contains(string(raw), "PASSWORD_CHANGE_REQUIRED") {
		t.Fatalf("temporary password should gate everything: %d %s", blocked.StatusCode, raw)
	}

	// /me still answers, so the browser can tell why it is stuck.
	me, err := newcomer.Get(srv.URL + "/api/v1/auth/me")
	if err != nil {
		t.Fatal(err)
	}
	meBody, _ := io.ReadAll(me.Body)
	me.Body.Close()
	if me.StatusCode != 200 || !strings.Contains(string(meBody), "mustChangePassword") {
		t.Fatalf("/me = %d %s", me.StatusCode, meBody)
	}

	status, data = postJSON(t, newcomer, srv.URL+"/api/v1/me/password", map[string]any{
		"currentPassword": temporary, "newPassword": "제가고른비밀번호입니다요",
	})
	if status != 200 {
		t.Fatalf("password change = %d %v", status, data)
	}

	// Now the account is a normal one.
	after, err := newcomer.Get(srv.URL + "/api/v1/workspaces")
	if err != nil {
		t.Fatal(err)
	}
	afterBody, _ := io.ReadAll(after.Body)
	after.Body.Close()
	if after.StatusCode != 200 {
		t.Fatalf("after the change the account must work: %d %s", after.StatusCode, afterBody)
	}
	// Its personal workspace is there and readable.
	if !strings.Contains(string(afterBody), "PERSONAL") {
		t.Fatalf("no personal workspace in %s", afterBody)
	}
}

func TestTheSameEmailIsRefusedRatherThanDuplicated(t *testing.T) {
	srv, _ := liveServer(t)
	admin := signIn(t, srv, "admin@muni.local", "provision-check-1234")
	payload := map[string]any{"email": "dup@example.com", "displayName": "중복"}
	if status, data := postJSON(t, admin, srv.URL+"/api/v1/admin/users", payload); status != 201 {
		t.Fatalf("first create = %d %v", status, data)
	}
	status, data := postJSON(t, admin, srv.URL+"/api/v1/admin/users", payload)
	if status != 409 || data["_errorCode"] != "EMAIL_TAKEN" {
		t.Fatalf("second create = %d %v", status, data)
	}
}

func TestTwoPeopleWithTheSameNameBothGetAnAccount(t *testing.T) {
	srv, _ := liveServer(t)
	admin := signIn(t, srv, "admin@muni.local", "provision-check-1234")
	first := map[string]any{"email": "a@example.com", "username": "kimminsu", "displayName": "김민수"}
	second := map[string]any{"email": "b@example.com", "username": "kimminsu", "displayName": "김민수"}
	_, one := postJSON(t, admin, srv.URL+"/api/v1/admin/users", first)
	status, two := postJSON(t, admin, srv.URL+"/api/v1/admin/users", second)
	if status != 201 {
		t.Fatalf("the second 김민수 must still get an account: %d %v", status, two)
	}
	if one["username"] == two["username"] {
		t.Fatalf("both got %v", one["username"])
	}
	if two["username"] != "kimminsu-2" {
		t.Fatalf("second username = %v, want kimminsu-2", two["username"])
	}
}

func TestABadRowDoesNotRejectTheGoodOnes(t *testing.T) {
	srv, _ := liveServer(t)
	admin := signIn(t, srv, "admin@muni.local", "provision-check-1234")
	// Row two collides with the bootstrap administrator; the rest are fine.
	csv := "이름,email\n" +
		"좋은행1,csv1@example.com\n" +
		"충돌,admin@muni.local\n" +
		"좋은행2,csv2@example.com\n"

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "staff.csv")
	_, _ = part.Write([]byte(csv))
	writer.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/admin/users/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("import = %d %s", resp.StatusCode, raw)
	}
	var out struct {
		Data struct {
			Created int `json:"created"`
			Failed  int `json:"failed"`
			Results []struct {
				Line              int    `json:"line"`
				OK                bool   `json:"ok"`
				Error             string `json:"error"`
				TemporaryPassword string `json:"temporaryPassword"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.Created != 2 || out.Data.Failed != 1 {
		t.Fatalf("created=%d failed=%d; the collision must not take the others down", out.Data.Created, out.Data.Failed)
	}
	for _, row := range out.Data.Results {
		if row.Line == 3 {
			if row.OK || row.Error == "" {
				t.Fatalf("the colliding row must report why: %+v", row)
			}
			continue
		}
		if !row.OK || row.TemporaryPassword == "" {
			t.Fatalf("a good row must come back with its password: %+v", row)
		}
	}
	// And the good rows really can sign in.
	signIn(t, srv, "csv1@example.com", out.Data.Results[0].TemporaryPassword)
}

func TestAnAdminResetAlsoDemandsAChange(t *testing.T) {
	srv, _ := liveServer(t)
	admin := signIn(t, srv, "admin@muni.local", "provision-check-1234")
	_, created := postJSON(t, admin, srv.URL+"/api/v1/admin/users", map[string]any{
		"email": "reset@example.com", "displayName": "잠긴사람", "password": "관리자가정한비밀번호입니다",
	})
	if created["temporaryPassword"] != nil {
		t.Fatal("a password the administrator typed must not be echoed back")
	}
	// Locked out on Monday: the administrator sets a new one.
	id, _ := created["id"].(string)
	status, data := postJSON(t, admin, srv.URL+"/api/v1/admin/users/"+id+"/password", map[string]any{
		"password": "월요일에쓸임시비밀번호입니다",
	})
	if status != 200 {
		t.Fatalf("reset = %d %v", status, data)
	}
	client := signIn(t, srv, "reset@example.com", "월요일에쓸임시비밀번호입니다")
	resp, err := client.Get(srv.URL + "/api/v1/workspaces")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 403 || !strings.Contains(string(raw), "PASSWORD_CHANGE_REQUIRED") {
		t.Fatalf("a password the administrator knows must be replaced too: %d %s", resp.StatusCode, raw)
	}
}
