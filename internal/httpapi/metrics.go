package httpapi

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Metrics for a Prometheus scrape.
//
// /healthz and /readyz answer whether muni is alive, which is the only
// question an operator could ask it. When something is slow there was nothing
// to look at but the log.
//
// The exposition format is written out rather than pulled in as a library:
// it is a few lines of text, and this is a project that ships as one offline
// image where every dependency is weight in that image.
//
// Cardinality is kept deliberately low. A label per document or per user is
// how a metrics endpoint turns into the thing that fills the disk.

// metricsCacheTTL is how long the counts that come from the database are
// reused. A scrape every fifteen seconds should not be a scan every fifteen
// seconds, and none of these numbers change meaningfully faster.
const metricsCacheTTL = 30 * time.Second

// durationBuckets are the boundaries for request latency, in seconds. They run
// from "instant" to "somebody is waiting", which is the range that tells an
// operator anything.
var durationBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

type metrics struct {
	mu sync.Mutex

	// requests counts by method and status class, not by path: a label per
	// route is manageable but a label per document id is not, and paths here
	// contain ids.
	requests map[requestKey]int64
	// buckets[i] counts requests that finished within durationBuckets[i].
	buckets  []int64
	overflow int64
	sum      float64
	count    int64

	cached     map[string]float64
	cachedAt   time.Time
	cachedOnce sync.Mutex
}

type requestKey struct {
	method string
	status string
}

func newMetrics() *metrics {
	return &metrics{
		requests: map[requestKey]int64{},
		buckets:  make([]int64, len(durationBuckets)),
	}
}

// observe records one finished request.
func (m *metrics) observe(method string, status int, elapsed time.Duration) {
	if m == nil {
		return
	}
	seconds := elapsed.Seconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[requestKey{method: method, status: statusClass(status)}]++
	m.count++
	m.sum += seconds
	placed := false
	for index, boundary := range durationBuckets {
		if seconds <= boundary {
			m.buckets[index]++
			placed = true
			break
		}
	}
	if !placed {
		m.overflow++
	}
}

// statusClass groups statuses as 2xx, 4xx and so on. The exact code matters
// when reading a log; what matters on a graph is whether it worked.
func statusClass(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	case status >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}

// statusRecorder remembers what was written, since ResponseWriter does not.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(body []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(body)
}

// Unwrap is what http.ResponseController follows to the real writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Flush and Hijack are forwarded because the code that needs them asks for
// them by type assertion, and a wrapper that does not carry them silently
// turns streaming answers into buffered ones and stops the collaboration
// socket from being upgraded at all.
func (s *statusRecorder) Flush() {
	if flusher, ok := s.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("이 응답은 연결을 넘겨받을 수 없습니다")
	}
	return hijacker.Hijack()
}

func (s *Server) serveMetrics(w http.ResponseWriter, r *http.Request) {
	counts := s.metricCounts(r.Context())

	var out strings.Builder
	write := func(name, kind, help string, value float64, labels string) {
		fmt.Fprintf(&out, "# HELP %s %s\n# TYPE %s %s\n%s%s %s\n",
			name, help, name, kind, name, labels, formatValue(value))
	}

	fmt.Fprintf(&out, "# HELP muni_build_info The running version.\n# TYPE muni_build_info gauge\n")
	fmt.Fprintf(&out, "muni_build_info{version=%q,commit=%q} 1\n",
		escapeLabel(s.info.Version), escapeLabel(s.info.Commit))

	s.metrics.writeHTTP(&out)

	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		write(name, "gauge", metricHelp[name], counts[name], "")
	}

	rooms, connections := s.hub.Size()
	write("muni_collab_documents_open", "gauge", "Documents with at least one editor connected.", float64(rooms), "")
	write("muni_collab_connections", "gauge", "Open collaboration connections.", float64(connections), "")

	write("muni_pdf_renders_in_progress", "gauge", "Headless browsers rendering a PDF right now.", float64(len(pdfSlots)), "")
	write("muni_pdf_renders_limit", "gauge", "How many may run at once.", float64(cap(pdfSlots)), "")

	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	write("muni_goroutines", "gauge", "Goroutines running.", float64(runtime.NumGoroutine()), "")
	write("muni_memory_bytes", "gauge", "Heap in use.", float64(memory.HeapInuse), "")

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(out.String()))
}

// writeHTTP renders the request counter and the latency histogram.
func (m *metrics) writeHTTP(out *strings.Builder) {
	m.mu.Lock()
	requests := make([]struct {
		key   requestKey
		value int64
	}, 0, len(m.requests))
	for key, value := range m.requests {
		requests = append(requests, struct {
			key   requestKey
			value int64
		}{key, value})
	}
	buckets := append([]int64(nil), m.buckets...)
	overflow, sum, count := m.overflow, m.sum, m.count
	m.mu.Unlock()

	sort.Slice(requests, func(i, j int) bool {
		if requests[i].key.method != requests[j].key.method {
			return requests[i].key.method < requests[j].key.method
		}
		return requests[i].key.status < requests[j].key.status
	})

	out.WriteString("# HELP muni_http_requests_total Requests served.\n# TYPE muni_http_requests_total counter\n")
	for _, entry := range requests {
		fmt.Fprintf(out, "muni_http_requests_total{method=%q,status=%q} %d\n",
			escapeLabel(entry.key.method), entry.key.status, entry.value)
	}

	out.WriteString("# HELP muni_http_request_duration_seconds How long requests took.\n# TYPE muni_http_request_duration_seconds histogram\n")
	running := int64(0)
	for index, boundary := range durationBuckets {
		running += buckets[index]
		fmt.Fprintf(out, "muni_http_request_duration_seconds_bucket{le=%q} %d\n",
			strconv.FormatFloat(boundary, 'g', -1, 64), running)
	}
	running += overflow
	fmt.Fprintf(out, "muni_http_request_duration_seconds_bucket{le=\"+Inf\"} %d\n", running)
	fmt.Fprintf(out, "muni_http_request_duration_seconds_sum %s\n", formatValue(sum))
	fmt.Fprintf(out, "muni_http_request_duration_seconds_count %d\n", count)
}

var metricHelp = map[string]string{
	"muni_users":                "Accounts that exist.",
	"muni_users_active":         "Accounts that are not suspended.",
	"muni_documents":            "Documents outside the trash.",
	"muni_documents_trashed":    "Documents in the trash.",
	"muni_workspaces":           "Workspaces in use.",
	"muni_approvals_pending":    "Documents waiting for a decision.",
	"muni_sessions_open":        "Sessions that have not expired.",
	"muni_attachment_bytes":     "Bytes held in attachments.",
	"muni_notifications_unsent": "Notifications waiting to be emailed.",
	"muni_ai_calls_last_24h":    "AI calls in the past day.",
	"muni_ai_failures_last_24h": "AI calls in the past day that failed.",
}

// metricCounts reads the numbers that come from the database, reusing the last
// answer for a short while so a scrape is not a scan.
func (s *Server) metricCounts(ctx context.Context) map[string]float64 {
	s.metrics.cachedOnce.Lock()
	defer s.metrics.cachedOnce.Unlock()
	if s.metrics.cached != nil && time.Since(s.metrics.cachedAt) < metricsCacheTTL {
		return s.metrics.cached
	}

	counts := map[string]float64{}
	scan := func(name, query string) {
		var value float64
		if s.db.QueryRow(ctx, query).Scan(&value) == nil {
			counts[name] = value
		}
	}
	scan("muni_users", `SELECT count(*) FROM users`)
	scan("muni_users_active", `SELECT count(*) FROM users WHERE status='ACTIVE'`)
	scan("muni_documents", `SELECT count(*) FROM documents WHERE deleted_at IS NULL`)
	scan("muni_documents_trashed", `SELECT count(*) FROM documents WHERE deleted_at IS NOT NULL`)
	scan("muni_workspaces", `SELECT count(*) FROM workspaces WHERE deleted_at IS NULL`)
	scan("muni_approvals_pending", `SELECT count(*) FROM approval_requests WHERE status='PENDING'`)
	scan("muni_sessions_open", `SELECT count(*) FROM sessions WHERE expires_at > now()`)
	scan("muni_attachment_bytes", `SELECT coalesce(sum(size_bytes),0) FROM attachments`)
	scan("muni_notifications_unsent", `SELECT count(*) FROM notifications WHERE emailed_at IS NULL`)
	scan("muni_ai_calls_last_24h", `SELECT count(*) FROM ai_actions WHERE created_at > now() - interval '24 hours'`)
	scan("muni_ai_failures_last_24h",
		`SELECT count(*) FROM ai_actions WHERE created_at > now() - interval '24 hours' AND status <> 'COMPLETED'`)

	s.metrics.cached = counts
	s.metrics.cachedAt = time.Now()
	return counts
}

// formatValue writes a number the way the exposition format expects: no
// exponent for ordinary sizes, and no trailing zeros.
func formatValue(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// escapeLabel makes a value safe to put between quotes in a label.
func escapeLabel(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return replacer.Replace(value)
}
