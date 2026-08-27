package httpapi

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// PDF rendering through Chromium's DevTools protocol.
//
// The command line can print a page and nothing else: its only choice about
// headers and footers is Chromium's own, which puts the source URL and the
// date on every page — and the source here is a temporary file path. A report
// that is printed and filed needs page numbers, and this is the only way
// Chromium offers to put them there. CSS cannot: page counters live in @page
// margin boxes, which Chromium does not implement.
//
// The command line stays as the fallback. A protocol conversation has more
// ways to fail than running a program does, and an export that stops working
// is worse than one without page numbers.

// devtoolsTimeout bounds the whole conversation, including the browser start.
const devtoolsTimeout = 90 * time.Second

type devtoolsMessage struct {
	ID     int             `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// printToPDFWithDevtools renders the page and returns the PDF.
func printToPDFWithDevtools(parent context.Context, binary, tempDir, htmlPath string, furniture pdfFurniture) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, devtoolsTimeout)
	defer cancel()

	profile := filepath.Join(tempDir, "profile")
	command := exec.CommandContext(ctx, binary,
		"--headless=new", "--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage",
		// Port zero lets the operating system choose; Chromium writes the one
		// it got into the profile directory.
		"--remote-debugging-port=0",
		"--user-data-dir="+profile,
		"about:blank")
	command.Env = append(os.Environ(),
		"HOME="+tempDir,
		"XDG_CONFIG_HOME="+filepath.Join(tempDir, "config"),
		"XDG_CACHE_HOME="+filepath.Join(tempDir, "cache"),
	)
	// Chromium announces where to connect on stderr. The port file it also
	// writes lands wherever the browser's own sandboxing decides to put the
	// profile, which is not always where it was asked to; the announcement is
	// not subject to that.
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("Chromium을 시작하지 못했습니다: %w", err)
	}
	defer func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
	}()

	browserURL, err := waitForDevtools(ctx, stderr)
	if err != nil {
		return nil, err
	}
	endpoint, err := devtoolsPage(ctx, browserURL)
	if err != nil {
		return nil, err
	}
	connection, _, err := websocket.DefaultDialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("Chromium에 연결하지 못했습니다: %w", err)
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetReadDeadline(deadline)
		_ = connection.SetWriteDeadline(deadline)
	}

	session := &devtoolsSession{connection: connection}
	if _, err := session.call("Page.enable", nil); err != nil {
		return nil, err
	}
	if _, err := session.call("Page.navigate", map[string]any{"url": "file://" + htmlPath}); err != nil {
		return nil, err
	}
	if err := session.waitForLoad(); err != nil {
		return nil, err
	}
	if furniture.Draws {
		// The page draws its diagrams after it loads. Printing before they are
		// finished leaves the picture out of the file rather than wrong in it,
		// and nothing about the PDF says anything went missing.
		session.waitForDiagrams()
	}

	paperWidth, paperHeight := furniture.paper()
	result, err := session.call("Page.printToPDF", map[string]any{
		"printBackground":     true,
		"displayHeaderFooter": true,
		"headerTemplate":      furniture.headerTemplate(),
		"footerTemplate":      furniture.footerTemplate(),
		// A4 in inches, with room at the bottom for the footer.
		"paperWidth":   paperWidth,
		"paperHeight":  paperHeight,
		"marginTop":    0.79,
		"marginBottom": 0.79,
		"marginLeft":   0.79,
		"marginRight":  0.79,
	})
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(result, &payload); err != nil || payload.Data == "" {
		return nil, errors.New("Chromium이 PDF를 돌려주지 않았습니다")
	}
	decoded, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		return nil, fmt.Errorf("PDF를 읽지 못했습니다: %w", err)
	}
	return decoded, nil
}

// pdfFurniture is what prints on every page around the document itself.
type pdfFurniture struct {
	Title string
	// Draws says the page has diagrams to draw, so the printer waits for it
	// to finish before taking the picture.
	Draws bool
	// Header is the document's own header line — where a Korean office
	// document carries 대외비 and the department. The header band used to be
	// switched on and left empty, so the space was reserved and nothing was
	// printed in it.
	Header string
	// Footer replaces the title on the left of the bottom band when the
	// author set one. The page numbers stay on the right either way.
	Footer string
	// Landscape turns the paper. A wide table printed on a portrait page is
	// either cut off or shrunk past reading.
	Landscape bool
}

// paper returns the sheet in inches, turned if the document asks for it.
func (f pdfFurniture) paper() (width, height float64) {
	const a4Width, a4Height = 8.27, 11.69
	if f.Landscape {
		return a4Height, a4Width
	}
	return a4Width, a4Height
}

// bandStyle is shared by both templates. Everything is inlined because
// Chromium renders each band in a document of its own that shares no
// stylesheet with the page.
const bandStyle = `width:100%;font-size:8pt;color:#666;` +
	`font-family:'Noto Sans CJK KR','Noto Sans KR',sans-serif;` +
	`padding:0 12mm;display:flex;justify-content:space-between;align-items:center;`

const ellipsis = `overflow:hidden;white-space:nowrap;text-overflow:ellipsis;`

// headerTemplate is the band at the top. With nothing to put there it is an
// empty span rather than an empty band, so the page keeps its full height.
func (f pdfFurniture) headerTemplate() string {
	text := strings.TrimSpace(f.Header)
	if text == "" {
		return `<span></span>`
	}
	return `<div style="` + bandStyle + `justify-content:flex-end;">` +
		`<span style="` + ellipsis + `max-width:100%;">` +
		html.EscapeString(truncate(text, 120)) + `</span></div>`
}

// footerTemplate builds the band Chromium renders at the bottom of each page.
//
// The placeholder classes are Chromium's own: it replaces their contents with
// the numbers as it lays each page out.
func (f pdfFurniture) footerTemplate() string {
	left := strings.TrimSpace(f.Footer)
	if left == "" {
		left = strings.TrimSpace(f.Title)
	}
	return `<div style="` + bandStyle + `">` +
		`<span style="` + ellipsis + `max-width:70%;">` + html.EscapeString(truncate(left, 80)) + `</span>` +
		`<span><span class="pageNumber"></span> / <span class="totalPages"></span></span>` +
		`</div>`
}

// waitForDiagrams waits for the page to say it has finished drawing.
//
// The page sets a flag when the drawing library is done, or immediately if it
// is not there at all. The bound is what stops one unparseable diagram from
// holding a worker for the whole request timeout — the page is printed as it
// stands, which is the text of the diagram rather than nothing.
func (s *devtoolsSession) waitForDiagrams() {
	const attempts = 100
	for attempt := 0; attempt < attempts; attempt++ {
		raw, err := s.call("Runtime.evaluate", map[string]any{
			"expression":    "window.muniDiagramsReady === true",
			"returnByValue": true,
		})
		if err == nil {
			var answer struct {
				Result struct {
					Value bool `json:"value"`
				} `json:"result"`
			}
			if json.Unmarshal(raw, &answer) == nil && answer.Result.Value {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitForDevtools reads the endpoint Chromium prints when it is ready.
func waitForDevtools(ctx context.Context, stderr io.Reader) (string, error) {
	type found struct {
		url string
		err error
	}
	results := make(chan found, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if index := strings.Index(line, "ws://"); index >= 0 {
				results <- found{url: strings.TrimSpace(line[index:])}
				return
			}
		}
		// Reading to the end without an endpoint means the browser exited.
		results <- found{err: errors.New("Chromium이 디버그 주소를 알려주지 않았습니다")}
	}()

	select {
	case result := <-results:
		return result.url, result.err
	case <-ctx.Done():
		return "", errors.New("Chromium이 제때 준비되지 않았습니다")
	}
}

// devtoolsPage opens a page to print in and returns its socket.
//
// The browser endpoint cannot print; printing belongs to a page target, and
// asking the browser's HTTP interface for one is the shortest way to get a
// socket that can.
func devtoolsPage(ctx context.Context, browserURL string) (string, error) {
	parsed, err := url.Parse(browserURL)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("디버그 주소를 이해하지 못했습니다: %q", browserURL)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut,
		"http://"+parsed.Host+"/json/new?about:blank", nil)
	if err != nil {
		return "", err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("Chromium 디버그 포트에 연결하지 못했습니다: %w", err)
	}
	defer response.Body.Close()
	var target struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(response.Body).Decode(&target); err != nil || target.WebSocketDebuggerURL == "" {
		return "", errors.New("Chromium이 인쇄할 페이지를 열지 못했습니다")
	}
	return target.WebSocketDebuggerURL, nil
}

// devtoolsSession is one conversation with one page.
type devtoolsSession struct {
	connection *websocket.Conn
	nextID     int
}

func (s *devtoolsSession) call(method string, params map[string]any) (json.RawMessage, error) {
	s.nextID++
	id := s.nextID
	encoded, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if err := s.connection.WriteMessage(websocket.TextMessage, encoded); err != nil {
		return nil, fmt.Errorf("%s 요청을 보내지 못했습니다: %w", method, err)
	}
	// Events arrive on the same socket, so replies are matched by id rather
	// than assumed to be next.
	for {
		var message devtoolsMessage
		if err := s.connection.ReadJSON(&message); err != nil {
			return nil, fmt.Errorf("%s 응답을 읽지 못했습니다: %w", method, err)
		}
		if message.ID != id {
			continue
		}
		if message.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, message.Error.Message)
		}
		return message.Result, nil
	}
}

// waitForLoad waits until the page says it has finished loading, so a document
// is not printed before its fonts and images are there.
func (s *devtoolsSession) waitForLoad() error {
	for {
		var message devtoolsMessage
		if err := s.connection.ReadJSON(&message); err != nil {
			return fmt.Errorf("페이지가 준비되기를 기다리지 못했습니다: %w", err)
		}
		if message.Method == "Page.loadEventFired" {
			return nil
		}
	}
}
