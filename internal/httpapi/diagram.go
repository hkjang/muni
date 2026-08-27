package httpapi

import (
	"io/fs"
	"sync"

	"github.com/hkjang/muni/webui"
)

// A diagram is drawn by the browser, which means the browser needs the library
// that draws it.
//
// The editor imports mermaid as a module and vite splits it out, so a document
// without a diagram never loads it. An exported HTML file and the page the PDF
// is printed from cannot import a module chunk — each needs one self-contained
// script — so the browser build is copied into the embedded assets at build
// time and handed to whichever export has something to draw.
//
// muni is offline. Nothing here reaches for a CDN, and an export made on a
// machine with no network draws exactly the same as one made anywhere else.

var (
	diagramOnce   sync.Once
	diagramScript []byte
)

// drawingLibrary returns the bytes of the drawing library, or nothing if this
// build does not carry it.
//
// A build without it is not an error: the export still carries the text of
// every diagram, which is what a diagram says. Refusing to export at all
// because a picture cannot be drawn would be the worse trade.
func drawingLibrary() []byte {
	diagramOnce.Do(func() {
		dist, err := fs.Sub(webui.Dist, "dist")
		if err != nil {
			return
		}
		if raw, err := fs.ReadFile(dist, "mermaid.min.js"); err == nil {
			diagramScript = raw
		}
	})
	return diagramScript
}

// diagramBootScript is what starts the drawing once the page is laid out.
//
// Rendering is synchronous from the caller's point of view and sets a flag the
// PDF printer waits for; without it Chromium prints the page while the
// diagrams are still being drawn, and the picture is missing from the file
// rather than wrong in it.
const diagramBootScript = `
window.muniDiagramsReady = false;
(function () {
  function start() {
    if (!window.mermaid) { window.muniDiagramsReady = true; return; }
    try {
      window.mermaid.initialize({
        startOnLoad: false,
        suppressErrorRendering: true,
        securityLevel: 'strict',
        fontFamily: "'Noto Sans KR','Malgun Gothic',sans-serif"
      });
      window.mermaid.run({ querySelector: 'pre.mermaid' })
        .catch(function () {})
        .finally(function () { window.muniDiagramsReady = true; });
    } catch (error) {
      window.muniDiagramsReady = true;
    }
  }
  if (document.readyState === 'complete' || document.readyState === 'interactive') {
    start();
  } else {
    document.addEventListener('DOMContentLoaded', start);
  }
})();
`

// diagramStyle keeps a drawn diagram inside the page it is printed on.
const diagramStyle = `pre.mermaid{background:none;border:none;padding:0;text-align:center;` +
	`page-break-inside:avoid}pre.mermaid svg{max-width:100%;height:auto}`
