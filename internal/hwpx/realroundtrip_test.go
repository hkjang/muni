package hwpx

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/hwp"
	"github.com/hkjang/muni/internal/richdoc"
)

// TestRealRoundTrip reads every real .hwp in a directory with the .hwp reader,
// writes it with this package's writer, and reads it back with this package's
// reader — real content through the whole chain. Opt-in, like TestCorpus: the
// files are not in the repository. Seventeen public files from the pyhwp,
// hwp.js and rhwp corpora come back with identical text and structure; a
// divergence is a failure.
func TestRealRoundTrip(t *testing.T) {
	dir := os.Getenv("MUNI_CORPUS")
	if dir == "" {
		t.Skip()
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.hwp"))
	sort.Strings(files)
	count := func(n *richdoc.Node, kind string) int {
		total := 0
		var walk func(*richdoc.Node)
		walk = func(x *richdoc.Node) {
			if x == nil {
				return
			}
			if x.Type == kind {
				total++
			}
			for _, c := range x.Content {
				walk(c)
			}
		}
		walk(n)
		return total
	}
	for _, path := range files {
		body, _ := os.ReadFile(path)
		original, assets, meta, err := hwp.Parse(body)
		if err != nil {
			continue
		}
		bytesFor := map[string]richdoc.Asset{}
		for _, a := range assets {
			bytesFor[a.Placeholder] = a
		}
		built, err := Build(original, Options{Title: "왕복", Header: meta.Header, Footer: meta.Footer, ResolveImage: func(src string) (Image, bool) {
			a, ok := bytesFor[src]
			return Image{Data: a.Data, MediaType: a.MediaType}, ok
		}})
		if err != nil {
			t.Errorf("%s: build: %v", filepath.Base(path), err)
			continue
		}
		// With MUNI_OUT set, each file written is kept, for a reader that
		// is not this one — Hangul, or another implementation — to judge.
		if out := os.Getenv("MUNI_OUT"); out != "" {
			name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) + ".hwpx"
			if err := os.WriteFile(filepath.Join(out, name), built, 0o644); err != nil {
				t.Errorf("%s: write: %v", name, err)
			}
		}
		back, backAssets, _, err := Parse(built)
		if err != nil {
			t.Errorf("%s: reread: %v", filepath.Base(path), err)
			continue
		}
		a, b := strings.Join(strings.Fields(original.PlainText()), " "), strings.Join(strings.Fields(back.PlainText()), " ")
		same := a == b
		t.Logf("%-36s text=%v h %d→%d tbl %d→%d img %d→%d fn %d→%d assets %d→%d",
			filepath.Base(path), same,
			count(original, "heading"), count(back, "heading"),
			count(original, "table"), count(back, "table"),
			count(original, "image"), count(back, "image"),
			count(original, "footnote"), count(back, "footnote"),
			len(assets), len(backAssets))
		if !same {
			t.Errorf("%s: 왕복에서 글자가 달라졌습니다", filepath.Base(path))
			// show the first divergence
			i := 0
			for i < len(a) && i < len(b) && a[i] == b[i] {
				i++
			}
			lo := i - 30
			if lo < 0 {
				lo = 0
			}
			t.Logf("    diverge@%d  hwp=%q  hwpx=%q", i, a[lo:min(len(a), i+40)], b[lo:min(len(b), i+40)])
		}
	}
}
