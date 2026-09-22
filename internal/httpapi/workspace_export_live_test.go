package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// A folder name goes into the workspace archive as a path element, and the name
// check on a folder lets through anything non-empty under 120 runes — `..`
// included. What that does to the archive is only visible in the entry names
// the real route writes, so this reads them off the wire.

// folderNamed makes a folder over the API the way the editor does.
func folderNamed(t *testing.T, srv *serverUnderTest, workspaceID uuid.UUID, name string, parent *uuid.UUID) uuid.UUID {
	t.Helper()
	payload := map[string]any{"name": name}
	if parent != nil {
		payload["parentId"] = parent.String()
	}
	status, data := postJSON(t, srv.admin, srv.URL+"/api/v1/workspaces/"+workspaceID.String()+"/folders", payload)
	if status != 201 {
		t.Fatalf("create folder %q = %d %v", name, status, data)
	}
	id, err := uuid.Parse(data["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	// The admin's workspace outlives the test — liveServer only clears the
	// accounts these tests make — so what this test puts in it has to come
	// back out, or a second run reads the first run's folders too.
	t.Cleanup(func() {
		_, _ = srv.db.Exec(context.Background(), `DELETE FROM folders WHERE id=$1`, id)
	})
	return id
}

// documentInFolder puts a document where the export will find it.
func documentInFolder(t *testing.T, srv *serverUnderTest, workspaceID uuid.UUID, folder *uuid.UUID, title string, trashed bool) {
	t.Helper()
	ctx := context.Background()
	var adminID uuid.UUID
	if err := srv.db.QueryRow(ctx, `SELECT id FROM users WHERE email='admin@muni.local'`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	deleted := "NULL"
	if trashed {
		deleted = "now()"
	}
	id := uuid.New()
	if _, err := srv.db.Exec(ctx,
		`INSERT INTO documents(id,workspace_id,owner_id,folder_id,title,deleted_at) VALUES($1,$2,$3,$4,$5,`+deleted+`)`,
		id, workspaceID, adminID, folder, title); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = srv.db.Exec(context.Background(), `DELETE FROM documents WHERE id=$1`, id)
	})
}

// exportEntryNames downloads the workspace archive and lists what is in it.
func exportEntryNames(t *testing.T, srv *serverUnderTest, workspaceID uuid.UUID) []string {
	t.Helper()
	resp, err := srv.admin.Get(srv.URL + "/api/v1/workspaces/" + workspaceID.String() + "/export.zip?format=md&trash=true")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("export = %d: %s", resp.StatusCode, raw)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(archive.File))
	for _, file := range archive.File {
		names = append(names, file.Name)
	}
	return names
}

func TestAWorkspaceArchiveStaysInsideItsOwnRoot(t *testing.T) {
	srv := newServerUnderTest(t)
	ctx := context.Background()
	var adminID, workspaceID uuid.UUID
	if err := srv.db.QueryRow(ctx, `SELECT id FROM users WHERE email='admin@muni.local'`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if err := srv.db.QueryRow(ctx, `SELECT id FROM workspaces WHERE owner_id=$1 LIMIT 1`, adminID).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}

	up := folderNamed(t, srv, workspaceID, "..", nil)
	under := folderNamed(t, srv, workspaceID, "하위", &up)
	here := folderNamed(t, srv, workspaceID, ".", nil)
	ordinary := folderNamed(t, srv, workspaceID, "2026 회의", nil)

	documentInFolder(t, srv, workspaceID, &up, "경로탈출 확인", false)
	documentInFolder(t, srv, workspaceID, &under, "하위 경로탈출 확인", false)
	documentInFolder(t, srv, workspaceID, &here, "점 폴더 확인", false)
	documentInFolder(t, srv, workspaceID, &ordinary, "평범한 폴더 확인", false)
	documentInFolder(t, srv, workspaceID, &up, "휴지통 확인", true)

	names := exportEntryNames(t, srv, workspaceID)
	if len(names) < 6 {
		t.Fatalf("archive holds only %d entries: %v", len(names), names)
	}

	directories := map[string]string{}
	for _, name := range names {
		if strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") {
			t.Errorf("entry %q points outside the archive", name)
		}
		for _, element := range strings.Split(name, "/") {
			if element == ".." || element == "." {
				t.Errorf("entry %q has a navigation element", name)
			}
		}
		// Two folders must not become one directory: remember which document
		// landed where.
		base := name[strings.LastIndex(name, "/")+1:]
		directory := strings.TrimSuffix(name, base)
		directories[base] = directory
	}
	if directories["점 폴더 확인.md"] == directories["평범한 폴더 확인.md"] {
		t.Errorf("the folder named \".\" collapsed onto another: %q", directories["점 폴더 확인.md"])
	}
	if directories["점 폴더 확인.md"] == "" {
		t.Errorf("the folder named \".\" collapsed onto the archive root")
	}
	if got := directories["휴지통 확인.md"]; !strings.HasPrefix(got, "휴지통/") {
		t.Errorf("a trashed document left 휴지통: %q", got)
	}
	// The ordinary folder is untouched by any of this.
	if got := directories["평범한 폴더 확인.md"]; got != "2026 회의/" {
		t.Errorf("an ordinary folder changed shape: %q", got)
	}
}
