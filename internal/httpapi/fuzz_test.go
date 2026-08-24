package httpapi

import "testing"

// FuzzMarkdownDocument and FuzzHTMLDocument guard the text importers, which
// parse uploaded files directly.
func FuzzMarkdownDocument(f *testing.F) {
	f.Add(richMarkdown)
	f.Add("- [x] a\n  - b\n    1. c\n")
	f.Add("| a | b |\n| --- | --- |\n| 1 | 2 |\n")
	f.Add("***bold italic*** and `code` and ![i](data:image/png;base64,AA)")
	f.Add("> quote\n> more\n\n```go\nx\n```\n")

	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > 1<<18 {
			t.Skip()
		}
		content, _, err := markdownDocument(body)
		if err != nil {
			return
		}
		if !validDocumentJSON(content) {
			t.Fatalf("markdown import produced an invalid document: %s", content)
		}
	})
}

func FuzzHTMLDocument(f *testing.F) {
	f.Add(richHTML)
	f.Add("<table><tr><td colspan=2 rowspan=3>x</td></tr></table>")
	f.Add("<ul><li><ul><li><ol><li>deep</li></ol></li></ul></li></ul>")
	f.Add("<p style=\"color:red;font-size:9pt\">styled</p>")

	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > 1<<18 {
			t.Skip()
		}
		content, _, err := htmlDocument([]byte(body))
		if err != nil {
			return
		}
		if !validDocumentJSON(content) {
			t.Fatalf("HTML import produced an invalid document: %s", content)
		}
	})
}
