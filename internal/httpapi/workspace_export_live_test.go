package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestWorkspaceExportReservesManifestName(t *testing.T) {
	srv := newServerUnderTest(t)
	workspaceID := adminWorkspace(t, srv)
	folderID := uuid.New()
	folderName := "내보내기-" + folderID.String()
	if _, err := srv.db.Exec(t.Context(), `INSERT INTO folders(id,workspace_id,owner_id,name)
		SELECT $1,id,owner_id,$2 FROM workspaces WHERE id=$3`, folderID, folderName, workspaceID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := srv.db.Exec(context.Background(), `DELETE FROM folders WHERE id=$1`, folderID); err != nil {
			t.Error(err)
		}
	})

	markers := make([]string, 4)
	for i, title := range []string{"목록", "목록", "목록 (2)", "목록"} {
		id := ownedDocument(t, srv, title)
		t.Cleanup(func() {
			if _, err := srv.db.Exec(context.Background(), `DELETE FROM documents WHERE id=$1`, id); err != nil {
				t.Error(err)
			}
		})
		markers[i] = "본문-" + id.String()
		content, err := json.Marshal(map[string]any{"type": "doc", "content": []any{
			map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": markers[i]}}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		var folder *uuid.UUID
		if i == 3 {
			folder = &folderID
		}
		if _, err := srv.db.Exec(t.Context(), `UPDATE documents SET content_json=$1,folder_id=$2 WHERE id=$3`, content, folder, id); err != nil {
			t.Fatal(err)
		}
	}

	for _, variant := range []struct{ name, query, extension string }{
		{"default", "", "md"},
		{"md", "?format=md", "md"},
		{"html", "?format=html", "html"},
		{"txt", "?format=txt", "txt"},
	} {
		t.Run(variant.name, func(t *testing.T) {
			resp, err := srv.admin.Get(srv.URL + "/api/v1/workspaces/" + workspaceID.String() + "/export.zip" + variant.query)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/zip" {
				t.Fatalf("export = %d, content type = %q", resp.StatusCode, resp.Header.Get("Content-Type"))
			}
			archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
			if err != nil {
				t.Fatal(err)
			}
			entries := map[string]string{}
			for _, file := range archive.File {
				// Check before inserting: a map alone would hide the overwritten document.
				if _, exists := entries[file.Name]; exists {
					t.Fatalf("duplicate ZIP entry: %q", file.Name)
				}
				reader, err := file.Open()
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(reader)
				reader.Close()
				if err != nil {
					t.Fatal(err)
				}
				entries[file.Name] = string(body)
			}
			manifest, exists := entries["목록.md"]
			if !exists {
				t.Fatal("missing 목록.md manifest")
			}
			listed := map[string]bool{}
			for _, line := range strings.Split(manifest, "\n") {
				if !strings.HasPrefix(line, "- ") {
					continue
				}
				name, _, ok := strings.Cut(strings.TrimPrefix(line, "- "), " — ")
				if !ok || listed[name] || name == "목록.md" {
					t.Fatalf("invalid manifest entry: %q", line)
				}
				if _, exists := entries[name]; !exists {
					t.Fatalf("manifest references missing ZIP entry: %q", name)
				}
				listed[name] = true
			}
			if len(listed) != len(entries)-1 {
				t.Fatalf("manifest lists %d documents for %d ZIP entries", len(listed), len(entries))
			}
			fixtureNames := map[string]bool{}
			for i, marker := range markers {
				var names []string
				for name, body := range entries {
					if strings.Contains(body, marker) {
						names = append(names, name)
						if strings.Count(body, marker) != 1 {
							t.Errorf("body repeated in %q", name)
						}
					}
				}
				if len(names) != 1 || !listed[names[0]] {
					t.Fatalf("body %q must appear in exactly one listed document: %v", marker, names)
				}
				if fixtureNames[names[0]] {
					t.Fatalf("distinct bodies share ZIP entry %q", names[0])
				}
				fixtureNames[names[0]] = true
				if !strings.HasSuffix(names[0], "."+variant.extension) {
					t.Errorf("wrong document extension: %q", names[0])
				}
				if i == 3 && names[0] != folderName+"/목록."+variant.extension {
					t.Errorf("nested document renamed: %q", names[0])
				}
				if i < 3 && strings.Contains(names[0], "/") {
					t.Errorf("root document moved: %q", names[0])
				}
			}
			if variant.extension != "md" && !fixtureNames["목록."+variant.extension] {
				t.Error("manifest unnecessarily renamed the first HTML/TXT document")
			}
		})
	}
}
