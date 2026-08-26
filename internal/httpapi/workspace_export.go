package httpapi

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Exporting a whole workspace.
//
// Documents could only be taken out one at a time. Handing a department over,
// answering an audit, keeping a copy before a workspace is wound up — all of
// them meant clicking through every document, and none of them were things
// anyone actually did.

// maxWorkspaceExport bounds one archive. A workspace larger than this is a
// migration rather than a download, and building it would hold a request open
// long past anything a browser will wait for.
const maxWorkspaceExport = 2000

// exportWorkspace streams every document in a workspace as one archive.
func (s *Server) exportWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	// Taking a copy of everything is a decision about the whole workspace, so
	// it belongs to whoever administers it.
	if !s.workspaceManager(r.Context(), workspaceID, p.User) {
		writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "워크스페이스를 내보낼 권한이 없습니다.")
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "md"
	}
	if !contains([]string{"md", "html", "txt"}, format) {
		// The heavier formats are left out on purpose: a thousand documents
		// through Chromium is not a download, and DOCX would mean holding
		// every rendered file while the archive is built.
		writeError(w, 400, "UNSUPPORTED_EXPORT", "워크스페이스 내보내기는 md, html, txt를 지원합니다.")
		return
	}
	includeTrash := r.URL.Query().Get("trash") == "true"

	var workspaceName string
	if err := s.db.QueryRow(r.Context(), `SELECT name FROM workspaces WHERE id=$1`, workspaceID).Scan(&workspaceName); err != nil {
		writeError(w, 404, "WORKSPACE_NOT_FOUND", "워크스페이스를 찾을 수 없습니다.")
		return
	}

	rows, err := s.db.Query(r.Context(), `
		SELECT d.id, d.title, d.content_json, d.folder_id, d.updated_at, d.deleted_at, u.display_name
		FROM documents d JOIN users u ON u.id = d.owner_id
		WHERE d.workspace_id = $1 AND ($2 OR d.deleted_at IS NULL)
		ORDER BY d.folder_id NULLS FIRST, d.title
		LIMIT $3`, workspaceID, includeTrash, maxWorkspaceExport)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "문서를 불러오지 못했습니다.")
		return
	}
	type exportItem struct {
		id       uuid.UUID
		title    string
		content  json.RawMessage
		folderID *uuid.UUID
		updated  time.Time
		trashed  bool
		owner    string
	}
	items := make([]exportItem, 0, 64)
	for rows.Next() {
		var item exportItem
		var deleted *time.Time
		if rows.Scan(&item.id, &item.title, &item.content, &item.folderID,
			&item.updated, &deleted, &item.owner) == nil {
			item.trashed = deleted != nil
			items = append(items, item)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		writeError(w, 500, "DATABASE_ERROR", "문서를 불러오지 못했습니다.")
		return
	}

	folders, err := s.folderPaths(r, workspaceID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "폴더를 불러오지 못했습니다.")
		return
	}

	// The headers go out before the first document is rendered: the archive is
	// written straight to the connection so a large workspace never has to fit
	// in memory or on disk.
	filename := safeFilename(workspaceName) + "-" + time.Now().Format("20060102")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`.zip"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	archive := zip.NewWriter(w)
	defer archive.Close()

	used := map[string]bool{}
	var manifest strings.Builder
	fmt.Fprintf(&manifest, "# %s\n\n내보낸 시각: %s\n문서 %d건\n\n",
		workspaceName, time.Now().Format(time.RFC3339), len(items))

	for _, item := range items {
		body := ""
		switch format {
		case "md":
			body = renderMarkdown(item.title, item.content)
		case "html":
			body = fullHTML(item.title, renderHTML(item.content))
		case "txt":
			body = renderPlainText(item.title, item.content)
		}

		directory := folders[folderKey(item.folderID)]
		if item.trashed {
			// The trash is kept apart, so restoring an archive does not put
			// deleted documents back among the live ones.
			directory = path.Join("휴지통", directory)
		}
		name := uniqueEntryName(used, directory, safeFilename(item.title), format)

		entry, err := archive.Create(name)
		if err != nil {
			// The response has begun, so there is nowhere to report this but
			// the log; the archive ends where it ends.
			s.logger.Warn("workspace export entry failed", "document", item.id, "error", err)
			return
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			s.logger.Warn("workspace export write failed", "document", item.id, "error", err)
			return
		}
		fmt.Fprintf(&manifest, "- %s — %s, %s\n", name, item.owner, item.updated.Format("2006-01-02"))
	}

	if len(items) == maxWorkspaceExport {
		fmt.Fprintf(&manifest, "\n문서가 %d건을 넘어 그만큼만 담았습니다.\n", maxWorkspaceExport)
	}
	if entry, err := archive.Create("목록.md"); err == nil {
		_, _ = entry.Write([]byte(manifest.String()))
	}

	s.audit(r, &p.User.ID, "EXPORT_WORKSPACE", "WORKSPACE", &workspaceID,
		map[string]any{"format": format, "documents": len(items)})
}

// folderPaths reads the workspace's folders as directory paths, so the archive
// comes out arranged the way the workspace is.
func (s *Server) folderPaths(r *http.Request, workspaceID uuid.UUID) (map[string]string, error) {
	rows, err := s.db.Query(r.Context(),
		`SELECT id, parent_id, name FROM folders WHERE workspace_id=$1 AND deleted_at IS NULL`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type folder struct {
		parent *uuid.UUID
		name   string
	}
	all := map[uuid.UUID]folder{}
	for rows.Next() {
		var id uuid.UUID
		var parent *uuid.UUID
		var name string
		if rows.Scan(&id, &parent, &name) == nil {
			all[id] = folder{parent: parent, name: name}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	paths := map[string]string{"": ""}
	var resolve func(id uuid.UUID, depth int) string
	resolve = func(id uuid.UUID, depth int) string {
		if existing, ok := paths[id.String()]; ok {
			return existing
		}
		// Depth is bounded in case the data ever contains a cycle: a lost
		// branch is better than a request that never returns.
		if depth > 32 {
			return ""
		}
		entry, ok := all[id]
		if !ok {
			return ""
		}
		prefix := ""
		if entry.parent != nil {
			prefix = resolve(*entry.parent, depth+1)
		}
		result := path.Join(prefix, safeFilename(entry.name))
		paths[id.String()] = result
		return result
	}
	for id := range all {
		resolve(id, 0)
	}
	return paths, nil
}

func folderKey(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// uniqueEntryName keeps two documents with the same title from becoming one
// file in the archive, which is how an export quietly loses a document.
func uniqueEntryName(used map[string]bool, directory, base, extension string) string {
	if base == "" {
		base = "제목 없는 문서"
	}
	name := path.Join(directory, base+"."+extension)
	if !used[name] {
		used[name] = true
		return name
	}
	for suffix := 2; ; suffix++ {
		candidate := path.Join(directory, fmt.Sprintf("%s (%d).%s", base, suffix, extension))
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}
