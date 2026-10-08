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

func TestAFolderNamedLikeTheIndexKeepsItsDocuments(t *testing.T) {
	srv := newServerUnderTest(t)
	titles := []string{"하위", "둘째", "셋째", "넷째"}
	for _, tc := range []struct {
		name  string
		roots []string
	}{
		{"lowercase", []string{"목록.md"}},
		{"uppercase", []string{"목록.MD"}},
		{"competing folders", []string{"목록.md", "목록.MD", "목록.md", "목록.md (2)"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, data := postJSON(t, srv.admin, srv.URL+"/api/v1/workspaces", map[string]any{
				"name": "안내와 폴더 보존", "slug": "export-" + uuid.NewString(),
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
			for i, name := range tc.roots {
				folder := folderNamed(t, srv, workspaceID, name, nil)
				child := folderNamed(t, srv, workspaceID, "자식", &folder)
				documentInFolder(t, srv, workspaceID, &folder, titles[i], false)
				documentInFolder(t, srv, workspaceID, &child, fmt.Sprintf("중첩%d", i), false)
				documentInFolder(t, srv, workspaceID, &child, fmt.Sprintf("버린 문서%d", i), true)
			}
			ordinary := folderNamed(t, srv, workspaceID, "README", nil)
			nested := folderNamed(t, srv, workspaceID, "목록.md", &ordinary)
			documentInFolder(t, srv, workspaceID, &ordinary, "그대로", false)
			documentInFolder(t, srv, workspaceID, &nested, "중첩 그대로", false)
			documentInFolder(t, srv, workspaceID, nil, "목록", false)
			// Give each document a distinct body, so an entry with only a title
			// cannot masquerade as a preserved document.
			if _, err := srv.db.Exec(t.Context(), `UPDATE documents SET content_json =
				jsonb_build_object('type','doc','content',jsonb_build_array(
					jsonb_build_object('type','paragraph','content',jsonb_build_array(
						jsonb_build_object('type','text','text','보존 본문 ' || title)))))
				WHERE workspace_id=$1`, workspaceID); err != nil {
				t.Fatal(err)
			}
			for _, format := range []string{"md", "html", "txt"} {
				for _, trash := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/trash=%t", format, trash), func(t *testing.T) {
						resp, err := srv.admin.Get(fmt.Sprintf("%s/api/v1/workspaces/%s/export.zip?format=%s&trash=%t", srv.URL, workspaceID, format, trash))
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
						bodies := map[string]string{}
						seen := map[string]bool{}
						indexCount := 0
						for _, file := range archive.File {
							key := strings.ToLower(file.Name)
							if seen[key] {
								t.Errorf("duplicate archive path %q", file.Name)
							}
							seen[key] = true
							if strings.HasPrefix(key, "목록.md/") {
								t.Errorf("index file 목록.md is a directory prefix of document %q", file.Name)
							}
							if file.Name == "목록.md" {
								indexCount++
							}
							body, err := file.Open()
							if err != nil {
								t.Fatal(err)
							}
							content, err := io.ReadAll(body)
							body.Close()
							if err != nil {
								t.Fatal(err)
							}
							bodies[file.Name] = string(content)
						}
						if indexCount != 1 {
							t.Errorf("index entries = %d, want exactly one", indexCount)
						}
						want := map[string]string{
							"README/그대로." + format:          "그대로",
							"README/목록.md/중첩 그대로." + format: "중첩 그대로",
						}
						rootDocument := "목록." + format
						if format == "md" {
							rootDocument = "목록 (2).md"
						}
						want[rootDocument] = "목록"
						directories := map[string]bool{}
						for i, root := range tc.roots {
							title := titles[i]
							directory := ""
							for name := range bodies {
								if strings.HasSuffix(name, "/"+title+"."+format) {
									directory = strings.TrimSuffix(name, "/"+title+"."+format)
								}
							}
							valid := directory == root && strings.ToLower(root) != "목록.md"
							for suffix := 2; suffix <= len(tc.roots)+1; suffix++ {
								valid = valid || directory == fmt.Sprintf("%s (%d)", root, suffix)
							}
							if !valid {
								t.Errorf("folder %q exported as %q, want a distinct directory preserving its case", root, directory)
							}
							if directories[strings.ToLower(directory)] {
								t.Errorf("user folders merged into %q", directory)
							}
							directories[strings.ToLower(directory)] = true
							want[directory+"/"+title+"."+format] = title
							childTitle := fmt.Sprintf("중첩%d", i)
							want[directory+"/자식/"+childTitle+"."+format] = childTitle
							if trash {
								trashTitle := fmt.Sprintf("버린 문서%d", i)
								want["휴지통/"+directory+"/자식/"+trashTitle+"."+format] = trashTitle
							}
						}
						if len(archive.File) != len(want)+1 {
							t.Errorf("ZIP entries = %d, want %d documents and one index", len(archive.File), len(want))
						}
						for name, title := range want {
							if !strings.Contains(bodies[name], "보존 본문 "+title) {
								t.Errorf("document body missing at %q", name)
							}
							if !strings.Contains(bodies["목록.md"], "- "+name+" —") {
								t.Errorf("index does not list final document path %q", name)
							}
						}
					})
				}
			}
		})
	}
}

func TestTwoTitlesDifferingOnlyInCaseStayTwoEntries(t *testing.T) {
	// The archive is unpacked wherever it is downloaded, and the deployment
	// target is Windows — where `Report.md` and `report.md` are one file name.
	// An unpacker writing entries in the order it finds them therefore keeps one
	// of the two, which is the same quiet loss uniqueEntryName was written to
	// prevent; it just never saw it, because its bookkeeping compared the names
	// byte for byte. What this can prove on a case-sensitive file system is the
	// entry names themselves: no two of them may differ only in case.
	srv := newServerUnderTest(t)
	workspaceID := adminWorkspace(t, srv)
	documentInFolder(t, srv, workspaceID, nil, "Report", false)
	documentInFolder(t, srv, workspaceID, nil, "report", false)
	// The same question one level down: the clash is in the whole path, so two
	// folders that differ only in case have to be told apart as well.
	upper := folderNamed(t, srv, workspaceID, "Report", nil)
	lower := folderNamed(t, srv, workspaceID, "report", nil)
	documentInFolder(t, srv, workspaceID, &upper, "회의록", false)
	documentInFolder(t, srv, workspaceID, &lower, "회의록", false)

	names := exportEntryNames(t, srv, workspaceID)

	folded := map[string]string{}
	for _, name := range names {
		key := strings.ToLower(name)
		if first, ok := folded[key]; ok {
			t.Errorf("entries %q and %q differ only in case; on a case-insensitive file system one of them will not survive unpacking:\n%v", first, name, names)
		}
		folded[key] = name
	}

	roots := []string{}
	for _, name := range names {
		if !strings.Contains(name, "/") && strings.HasPrefix(strings.ToLower(name), "report") {
			roots = append(roots, name)
		}
	}
	if len(roots) != 2 {
		t.Fatalf("the two documents did not produce two entries of their own: %v", names)
	}
	// Folding belongs to the bookkeeping and not to the name anyone unpacks: one
	// entry still reads Report and the other report, exactly as they were typed.
	upperRoots, lowerRoots, suffixed := 0, 0, 0
	for _, name := range roots {
		if strings.HasPrefix(name, "Report") {
			upperRoots++
		}
		if strings.HasPrefix(name, "report") {
			lowerRoots++
		}
		if strings.Contains(name, " (2).") {
			suffixed++
		}
	}
	if upperRoots != 1 || lowerRoots != 1 {
		t.Errorf("a title lost the casing it was given: %v", roots)
	}
	if suffixed != 1 {
		t.Errorf("exactly one of the two should carry the (2) suffix: %v", roots)
	}

	// One level down the two folders are told apart the same way, and that is
	// why the directory part is no longer `report/` for both: folding the entry
	// name only kept the two documents from becoming one file, while the two
	// directories stayed one directory. folderPaths now claims a directory as
	// it builds it, so the second of the pair comes out as `report (2)/` —
	// which is what these expectations moved for. The filter has to be loose
	// enough to find that name, so it matches the prefix rather than `report/`.
	inFolders := []string{}
	for _, name := range names {
		if strings.HasPrefix(strings.ToLower(name), "report") && strings.Contains(name, "/") {
			inFolders = append(inFolders, name)
		}
	}
	if len(inFolders) != 2 {
		t.Fatalf("the two folders did not produce two entries: %v", names)
	}
	upperDir, lowerDir, suffixedDirs := 0, 0, 0
	directories := []string{}
	for _, name := range inFolders {
		directory := name[:strings.Index(name, "/")]
		directories = append(directories, directory)
		if strings.HasPrefix(directory, "Report") {
			upperDir++
		}
		if strings.HasPrefix(directory, "report") {
			lowerDir++
		}
		if strings.Contains(directory, " (2)") {
			suffixedDirs++
		}
	}
	if strings.EqualFold(directories[0], directories[1]) {
		t.Errorf("directories %q and %q are one directory on Windows: %v", directories[0], directories[1], names)
	}
	// Folding belongs to the bookkeeping here too: one directory still reads
	// Report and the other report. Which of them takes the suffix is not pinned
	// down — resolve walks parents before siblings.
	if upperDir != 1 || lowerDir != 1 {
		t.Errorf("a folder lost the casing it was given: %v", directories)
	}
	if suffixedDirs != 1 {
		t.Errorf("exactly one of %q and %q should carry a suffix", directories[0], directories[1])
	}
}

func TestTwoFoldersWithOneNameStayTwoDirectories(t *testing.T) {
	// Nothing stops two folders from carrying the same name: the folder name
	// check looks at emptiness and length alone, so the editor makes this pair
	// without complaint, and a user who has one 기획 folder per team sees two
	// folders in the sidebar. The archive flattened them into the one path
	// string, so both teams' documents came out of the same directory and the
	// structure the user was looking at was gone from the download. The
	// documents themselves survived — uniqueEntryName moves the second one to
	// `문서 (2).md` — which is why this is only visible in the directory part of
	// the entry names, and why that is what this reads off the wire.
	srv := newServerUnderTest(t)
	workspaceID := adminWorkspace(t, srv)
	first := folderNamed(t, srv, workspaceID, "기획", nil)
	second := folderNamed(t, srv, workspaceID, "기획", nil)
	documentInFolder(t, srv, workspaceID, &first, "첫째 문서", false)
	documentInFolder(t, srv, workspaceID, &second, "둘째 문서", false)
	// And what must not move, in the same archive so one request proves both: a
	// folder whose name nothing else shares keeps every letter it was given —
	// casing included — a child still sits under its parent, and a deleted
	// document still sits under 휴지통.
	plain := folderNamed(t, srv, workspaceID, "README", nil)
	child := folderNamed(t, srv, workspaceID, "하위", &plain)
	documentInFolder(t, srv, workspaceID, &plain, "그대로 문서", false)
	documentInFolder(t, srv, workspaceID, &child, "중첩 문서", false)
	documentInFolder(t, srv, workspaceID, &child, "버린 문서", true)

	names := exportEntryNames(t, srv, workspaceID)

	// Every entry is read as directory plus base name; the titles above are
	// unique, so each one names the directory its folder produced.
	directories := map[string]string{}
	for _, name := range names {
		base := name[strings.LastIndex(name, "/")+1:]
		directories[base] = strings.TrimSuffix(name, base)
	}
	firstDir, secondDir := directories["첫째 문서.md"], directories["둘째 문서.md"]
	if firstDir == "" || secondDir == "" {
		t.Fatalf("a document in one of the two 기획 folders is missing: %v", names)
	}
	if firstDir == secondDir {
		t.Errorf("both 기획 folders became the one directory %q, so two folders unpack as one: %v", firstDir, names)
	}
	// Which of the two keeps the plain name is deliberately not pinned down:
	// resolve walks parents before siblings, so a folder with children can
	// claim its name ahead of a sibling listed before it.
	suffixed := 0
	for _, directory := range []string{firstDir, secondDir} {
		if !strings.HasPrefix(directory, "기획") {
			t.Errorf("a folder lost the name it was given: %q", directory)
		}
		if strings.Contains(directory, " (2)") {
			suffixed++
		}
	}
	if suffixed != 1 {
		t.Errorf("exactly one of %q and %q should carry a suffix", firstDir, secondDir)
	}
	// No two directories in the archive may differ only in case either, for the
	// same reason no two entry names may: on Windows they are one directory.
	folded := map[string]string{}
	for _, name := range names {
		base := name[strings.LastIndex(name, "/")+1:]
		directory := strings.TrimSuffix(name, base)
		if directory == "" {
			continue
		}
		key := strings.ToLower(directory)
		if other, ok := folded[key]; ok && other != directory {
			t.Errorf("directories %q and %q differ only in case: %v", other, directory, names)
		}
		folded[key] = directory
	}

	if got := directories["그대로 문서.md"]; got != "README/" {
		t.Errorf("a folder nothing clashes with changed shape: %q", got)
	}
	if got := directories["중첩 문서.md"]; got != "README/하위/" {
		t.Errorf("a nested folder left its parent: %q", got)
	}
	if got := directories["버린 문서.md"]; got != "휴지통/README/하위/" {
		t.Errorf("a deleted document left 휴지통 or its folder: %q", got)
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

// documentWithTheDefaultTitle makes a document the way the editor's new-document
// button does — with no title at all — so the title it ends up with is whatever
// createDocument gives an untitled one, and not a string this test chose. The
// body says which document it is, because that is the only thing in the archive
// that tells these documents apart.
func documentWithTheDefaultTitle(t *testing.T, srv *serverUnderTest, workspaceID uuid.UUID, marker string) uuid.UUID {
	t.Helper()
	status, data := postJSON(t, srv.admin, srv.URL+"/api/v1/documents", map[string]any{
		"workspaceId": workspaceID.String(),
		"content": map[string]any{"type": "doc", "content": []any{
			map[string]any{"type": "paragraph", "content": []any{
				map[string]any{"type": "text", "text": marker},
			}},
		}},
	})
	if status != 200 {
		t.Fatalf("create untitled document %q = %d %v", marker, status, data)
	}
	id, err := uuid.Parse(data["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	// The admin's workspace outlives the test, so what goes into it has to come
	// back out or the next run reads this run's documents too.
	t.Cleanup(func() {
		_, _ = srv.db.Exec(context.Background(), `DELETE FROM documents WHERE id=$1`, id)
	})
	return id
}

// exportEntryBodies downloads the workspace archive and reads every entry, so a
// caller can check which document ended up under which name rather than only
// which names came out.
func exportEntryBodies(t *testing.T, srv *serverUnderTest, workspaceID uuid.UUID) map[string]string {
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
	if resp.StatusCode != 200 {
		t.Fatalf("export = %d: %s", resp.StatusCode, raw)
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	bodies := make(map[string]string, len(archive.File))
	for _, file := range archive.File {
		body, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		text, err := io.ReadAll(body)
		body.Close()
		if err != nil {
			t.Fatal(err)
		}
		bodies[file.Name] = string(text)
	}
	return bodies
}

func TestDocumentsSharingOneTitleKeepTheEntryTheyWereGiven(t *testing.T) {
	// Two documents of one title is not an edge case here: createDocument calls
	// every untitled document 제목 없는 문서, so a workspace where nobody renames
	// anything is a workspace where every document has the same title. The
	// document query ordered by folder and title alone, which leaves all of
	// those rows tied — and PostgreSQL does not promise an order among tied
	// rows. That order is what decides which of them keeps 제목 없는 문서.md and
	// which is moved to 제목 없는 문서 (2).md, so downloading an unchanged
	// workspace twice could swap the contents of those two files and swap the
	// authors 목록.md credits them to. Breaking the tie on d.id settles it.
	//
	// The set of entry names is the same either way — both names come out in
	// every order — so a test comparing names would pass on the broken code.
	// What has to hold still is the pairing, which is why this reads the bodies.
	srv := newServerUnderTest(t)
	workspaceID := adminWorkspace(t, srv)
	markers := []string{"본문 가", "본문 나", "본문 다", "본문 라"}
	ids := make([]uuid.UUID, 0, len(markers))
	for _, marker := range markers {
		ids = append(ids, documentWithTheDefaultTitle(t, srv, workspaceID, marker))
	}

	pairing := exportEntryBodies(t, srv, workspaceID)
	entryOf := func(bodies map[string]string, marker string) string {
		t.Helper()
		for name, body := range bodies {
			if name != workspaceManifestName && strings.Contains(body, marker) {
				return name
			}
		}
		t.Fatalf("%s is in no entry of the archive: %v", marker, bodies)
		return ""
	}
	want := map[string]string{}
	for _, marker := range markers {
		want[marker] = entryOf(pairing, marker)
	}
	// All four documents must actually be competing for the one name, or there
	// is no tie to break and this test proves nothing.
	if len(want) != len(markers) {
		t.Fatalf("the four documents did not land in four entries: %v", want)
	}
	for _, marker := range markers {
		if !strings.HasPrefix(want[marker], "제목 없는 문서") {
			t.Fatalf("%s came out as %q, so these documents are not sharing one title", marker, want[marker])
		}
	}

	// Rewriting one row moves it to the end of the sequential scan, and that
	// scan is where the sort gets its input order — so with the tie unbroken
	// the row that was read first is now read last, and the sort has no reason
	// to put it back. Exactly one row is rewritten on purpose: rewriting all of
	// them in order would move them all to the end in the order they already
	// had, and the pairing would survive by luck rather than by the fix.
	//
	// The title is written back as itself, so every column the archive reports
	// — title, body, owner, timestamp — is byte for byte what it was. Nothing
	// about this workspace has changed, which is why the archive must not
	// change either.
	if _, err := srv.db.Exec(t.Context(), `UPDATE documents SET title=title WHERE id=$1`, ids[0]); err != nil {
		t.Fatal(err)
	}
	// Twelve more downloads on top of that, because a tie can also come out
	// differently from one request to the next without anything being written.
	for attempt := range 12 {
		bodies := exportEntryBodies(t, srv, workspaceID)
		for _, marker := range markers {
			if got := entryOf(bodies, marker); got != want[marker] {
				t.Fatalf("download %d put %s in %q, but the first download put it in %q; the same unchanged workspace downloaded twice hands these files different contents",
					attempt+2, marker, got, want[marker])
			}
		}
		manifest := bodies[workspaceManifestName]
		for _, marker := range markers {
			if !strings.Contains(manifest, "- "+want[marker]+" —") {
				t.Errorf("download %d: 목록.md does not list %q; it says:\n%s", attempt+2, want[marker], manifest)
			}
		}
	}
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
