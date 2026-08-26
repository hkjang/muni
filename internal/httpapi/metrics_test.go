package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The request middleware wraps the ResponseWriter to remember the status.
// Streaming answers and the collaboration socket reach for Flusher and
// Hijacker by type assertion, so a wrapper that does not carry them would turn
// the AI stream into a buffered response and stop the socket from upgrading —
// with nothing failing loudly enough to notice.

func TestTheWrappedWriterStillStreams(t *testing.T) {
	recorder := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	if _, ok := any(recorder).(http.Flusher); !ok {
		t.Fatal("a streaming answer needs the writer to flush")
	}
	if _, ok := any(recorder).(http.Hijacker); !ok {
		t.Fatal("the collaboration socket needs to hijack the connection")
	}
}

func TestTheWrapperReachesTheWriterUnderneath(t *testing.T) {
	underlying := httptest.NewRecorder()
	recorder := &statusRecorder{ResponseWriter: underlying}
	if recorder.Unwrap() != http.ResponseWriter(underlying) {
		t.Fatal("http.ResponseController follows Unwrap to the real writer")
	}
}

func TestAWriteWithNoHeaderCountsAsSuccess(t *testing.T) {
	// A handler that writes a body without calling WriteHeader has answered
	// 200, and the metric has to say so rather than nothing.
	recorder := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	_, _ = recorder.Write([]byte("body"))
	if recorder.status != http.StatusOK {
		t.Fatalf("status = %d", recorder.status)
	}
}

func TestStatusesAreGroupedNotEnumerated(t *testing.T) {
	// A label per status code is manageable; the point is that a graph wants
	// to know whether it worked.
	cases := map[int]string{200: "2xx", 204: "2xx", 302: "3xx", 404: "4xx", 500: "5xx"}
	for status, want := range cases {
		if got := statusClass(status); got != want {
			t.Fatalf("statusClass(%d) = %q, want %q", status, got, want)
		}
	}
}

func TestLatencyLandsInTheRightBucket(t *testing.T) {
	m := newMetrics()
	m.observe("GET", 200, 30*time.Millisecond)
	m.observe("GET", 200, 2*time.Second)
	// Longer than every boundary, so it only appears in +Inf.
	m.observe("POST", 500, time.Minute)

	var out strings.Builder
	m.writeHTTP(&out)
	body := out.String()

	if !strings.Contains(body, `muni_http_requests_total{method="GET",status="2xx"} 2`) {
		t.Fatalf("request counts missing: %s", body)
	}
	if !strings.Contains(body, `muni_http_requests_total{method="POST",status="5xx"} 1`) {
		t.Fatalf("failure count missing: %s", body)
	}
	// The buckets are cumulative: everything at or under 0.05s is one request.
	if !strings.Contains(body, `muni_http_request_duration_seconds_bucket{le="0.05"} 1`) {
		t.Fatalf("cumulative buckets wrong: %s", body)
	}
	if !strings.Contains(body, `muni_http_request_duration_seconds_bucket{le="+Inf"} 3`) {
		t.Fatalf("the slowest request must still be counted: %s", body)
	}
	if !strings.Contains(body, "muni_http_request_duration_seconds_count 3") {
		t.Fatalf("count missing: %s", body)
	}
}

func TestALabelCannotBreakOutOfItsQuotes(t *testing.T) {
	if got := escapeLabel(`a"b\c`); got != `a\"b\\c` {
		t.Fatalf("escapeLabel = %q", got)
	}
	if strings.Contains(escapeLabel("a\nb"), "\n") {
		t.Fatal("a newline in a label would end the line early")
	}
}

func TestValuesAreWrittenWithoutAnExponent(t *testing.T) {
	// 3.7e+08 is valid in the format but unreadable on a dashboard.
	if got := formatValue(370000000); strings.Contains(got, "e") {
		t.Fatalf("value = %q", got)
	}
}
