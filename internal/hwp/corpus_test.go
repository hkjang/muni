package hwp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// TestCorpus reads every real file in a directory and reports what came of
// each. It is opt-in: the files are not in the repository, and the point is to
// run the readers against documents Hangul actually saved rather than ones
// muni built for a test. It fails only on a parse error.
func TestCorpus(t *testing.T) {
	dir := os.Getenv("MUNI_CORPUS")
	if dir == "" {
		t.Skip("set MUNI_CORPUS")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.hwp"))
	sort.Strings(files)
	for _, path := range files {
		body, _ := os.ReadFile(path)
		document, assets, meta, err := Parse(body)
		name := filepath.Base(path)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		counts := map[string]int{}
		var walk func(*richdoc.Node)
		walk = func(n *richdoc.Node) {
			if n == nil {
				return
			}
			counts[n.Type]++
			for _, c := range n.Content {
				walk(c)
			}
		}
		walk(document)
		text := strings.ReplaceAll(document.PlainText(), "\n", "⏎")
		if len([]rune(text)) > 90 {
			text = string([]rune(text)[:90]) + "…"
		}
		t.Logf("%-40s v%s blocks=%d p=%d h=%d tbl=%d img=%d fn=%d assets=%d | %s",
			name, meta.Version, len(document.Content), counts["paragraph"], counts["heading"],
			counts["table"], counts["image"], counts["footnote"], len(assets), text)
	}
}
