package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestAWorkspaceExportWarnsOnlyWhenDocumentsAreOmitted(t *testing.T) {
	srv := newServerUnderTest(t)
	for _, count := range []int{1999, 2000, 2001} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			status, data := postJSON(t, srv.admin, srv.URL+"/api/v1/workspaces", map[string]any{
				"name": "내보내기 경계", "slug": "export-" + uuid.NewString(),
			})
			if status != 201 {
				t.Fatalf("create workspace = %d %v", status, data)
			}
			workspaceID := uuid.MustParse(data["id"].(string))
			t.Cleanup(func() {
				if _, err := srv.db.Exec(context.Background(), `DELETE FROM workspaces WHERE id=$1`, workspaceID); err != nil {
					t.Error(err)
				}
			})
			// Bulk seed real rows so the production query, its LIMIT and the ZIP
			// writer are all exercised without thousands of setup HTTP requests.
			if _, err := srv.db.Exec(t.Context(), `
				INSERT INTO documents(workspace_id,owner_id,title)
				SELECT w.id,w.owner_id,'문서-' || lpad(n::text,4,'0')
				FROM workspaces w CROSS JOIN generate_series(1,$2::int) n
				WHERE w.id=$1`, workspaceID, count); err != nil {
				t.Fatal(err)
			}
			resp, err := srv.admin.Get(srv.URL + "/api/v1/workspaces/" + workspaceID.String() + "/export.zip?format=md")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != 200 {
				t.Fatalf("export = %d: %s", resp.StatusCode, raw)
			}
			archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
			if err != nil {
				t.Fatal(err)
			}
			wantCount := min(count, 2000)
			if len(archive.File) != wantCount+1 {
				t.Fatalf("ZIP entries = %d, want %d documents and one index", len(archive.File), wantCount)
			}
			index, err := archive.Open("목록.md")
			if err != nil {
				t.Fatal(err)
			}
			defer index.Close()
			body, err := io.ReadAll(index)
			if err != nil {
				t.Fatal(err)
			}
			manifest := string(body)
			if got := strings.Contains(manifest, "그만큼만 담았습니다"); got != (count > 2000) {
				t.Errorf("omission warning = %t for %d documents, want %t", got, count, count > 2000)
			}
			if !strings.Contains(manifest, fmt.Sprintf("문서 %d건\n", wantCount)) || strings.Count(manifest, "\n- ") != wantCount {
				t.Error("index count does not match the exported documents")
			}
			for _, file := range archive.File {
				if file.Name != "목록.md" && !strings.Contains(manifest, "- "+file.Name+" —") {
					t.Errorf("index does not list exported entry %q", file.Name)
				}
			}
		})
	}
}

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

func TestADocumentTitledLikeTheIndexKeepsItsOwnEntry(t *testing.T) {
	// uniqueEntryName exists because two documents with one title would become
	// one file, and that is how an export loses a document without saying so.
	// The index the export writes last is a file in the same archive under a
	// fixed name, and it never went through that bookkeeping — so a document
	// called 목록 sitting at the root of the workspace produces 목록.md, the
	// index is written as 목록.md too, and an unpacker writing entries in order
	// leaves one file where there should be two.
	srv := newServerUnderTest(t)
	workspaceID := adminWorkspace(t, srv)
	documentInFolder(t, srv, workspaceID, nil, "목록", false)
	// A second document under a folder of the same name is untouched by any of
	// this: only the archive root can clash with the index.
	folder := folderNamed(t, srv, workspaceID, "회의", nil)
	documentInFolder(t, srv, workspaceID, &folder, "목록", false)

	names := exportEntryNames(t, srv, workspaceID)

	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			t.Errorf("two entries are called %q; one of them will not survive unpacking:\n%v", name, names)
		}
		seen[name] = true
	}
	if !seen["목록.md"] {
		t.Fatalf("the index is missing from the archive: %v", names)
	}
	if !seen["회의/목록.md"] {
		t.Errorf("the document in a folder changed name: %v", names)
	}
	// The root document has to be in there under some name of its own, and the
	// index has to point at that name rather than at itself.
	root := ""
	for _, name := range names {
		if !strings.Contains(name, "/") && name != "목록.md" && strings.HasPrefix(name, "목록") {
			root = name
		}
	}
	if root == "" {
		t.Fatalf("the document titled 목록 has no entry of its own: %v", names)
	}
	manifest := exportManifest(t, srv, workspaceID)
	if !strings.Contains(manifest, "- "+root+" —") {
		t.Errorf("목록.md does not list %q; it says:\n%s", root, manifest)
	}
}

// exportManifest reads the index the workspace archive carries.
func exportManifest(t *testing.T, srv *serverUnderTest, workspaceID uuid.UUID) string {
	t.Helper()
	resp, err := srv.admin.Get(srv.URL + "/api/v1/workspaces/" + workspaceID.String() + "/export.zip?format=md&trash=true")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		if file.Name != "목록.md" {
			continue
		}
		body, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer body.Close()
		text, err := io.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}
		return string(text)
	}
	t.Fatalf("the archive has no 목록.md")
	return ""
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
