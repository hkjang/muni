package httpapi

import (
	"net/http"
	"sort"
	"strings"
	"testing"
)

// The published API document is what anyone writing an integration reads. It
// is maintained by hand, and by the time anybody looked it described a third
// of the service — so this holds it against the routes actually served.

func TestEveryRouteIsInTheAPIDocument(t *testing.T) {
	documented := documentedPaths(t)
	missing := make([]string, 0)
	for _, pattern := range routePatterns() {
		path, ok := specPath(pattern)
		if !ok {
			continue
		}
		if !documented[path] {
			missing = append(missing, path)
		}
	}
	sort.Strings(missing)
	missing = unique(missing)
	if len(missing) > 0 {
		t.Fatalf("these routes are served but not in openapi.yaml:\n  %s",
			strings.Join(missing, "\n  "))
	}
}

func TestTheAPIDocumentDescribesNothingImaginary(t *testing.T) {
	served := map[string]bool{}
	for _, pattern := range routePatterns() {
		if path, ok := specPath(pattern); ok {
			served[path] = true
		}
	}
	extra := make([]string, 0)
	for path := range documentedPaths(t) {
		if !served[path] {
			extra = append(extra, path)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Fatalf("openapi.yaml describes routes that are not served:\n  %s",
			strings.Join(extra, "\n  "))
	}
}

// routePatterns builds a server far enough to register its routes. Nothing is
// called, so the nil database is never reached.
func routePatterns() []string {
	server := &Server{mux: http.NewServeMux()}
	server.routes()
	return server.Patterns()
}

// specPath turns "GET /api/v1/documents/{id}" into the path the document uses,
// which is relative to the /api/v1 server entry. Anything outside that prefix
// — the health checks, the spec itself, the embedded web app — is not part of
// the API being described.
func specPath(pattern string) (string, bool) {
	fields := strings.Fields(pattern)
	path := fields[len(fields)-1]
	const prefix = "/api/v1"
	if !strings.HasPrefix(path, prefix+"/") {
		return "", false
	}
	return normalizePath(strings.TrimPrefix(path, prefix)), true
}

// normalizePath makes two paths comparable when they name their parameters
// differently. OpenAPI path parameter names are the document's own choice —
// {documentId} where the route says {id} is the same endpoint, and comparing
// the literal strings would report every one of them as missing.
func normalizePath(path string) string {
	var out strings.Builder
	depth := 0
	for _, r := range path {
		switch r {
		case '{':
			depth++
			if depth == 1 {
				out.WriteString("{}")
			}
		case '}':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				out.WriteRune(r)
			}
		}
	}
	return out.String()
}

func documentedPaths(t *testing.T) map[string]bool {
	t.Helper()
	paths := map[string]bool{}
	inPaths := false
	for _, line := range strings.Split(string(openAPIDocument), "\n") {
		if strings.HasPrefix(line, "paths:") {
			inPaths = true
			continue
		}
		if inPaths && len(line) > 0 && line[0] != ' ' {
			break
		}
		// A path entry is indented exactly two spaces and starts with a slash.
		if inPaths && strings.HasPrefix(line, "  /") && !strings.HasPrefix(line, "   ") {
			paths[normalizePath(strings.TrimSuffix(strings.TrimSpace(line), ":"))] = true
		}
	}
	if len(paths) == 0 {
		t.Fatal("no paths found in openapi.yaml")
	}
	return paths
}

func unique(values []string) []string {
	out := values[:0]
	var previous string
	for index, value := range values {
		if index > 0 && value == previous {
			continue
		}
		previous = value
		out = append(out, value)
	}
	return out
}
