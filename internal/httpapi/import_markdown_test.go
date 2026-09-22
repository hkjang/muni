package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// tableCellTexts collects the trimmed text of every cell in the first table of
// the document, row by row.
func tableCellTexts(t *testing.T, content json.RawMessage) [][]string {
	t.Helper()
	document := parseImported(t, content)
	var table *richdoc.Node
	var walk func(*richdoc.Node)
	walk = func(current *richdoc.Node) {
		if table != nil {
			return
		}
		if current.Type == "table" {
			table = current
			return
		}
		for _, child := range current.Content {
			walk(child)
		}
	}
	walk(document)
	if table == nil {
		t.Fatalf("no table in document: %s", content)
	}
	rows := make([][]string, 0, len(table.Content))
	for _, row := range table.Content {
		cells := make([]string, 0, len(row.Content))
		for _, cell := range row.Content {
			cells = append(cells, cell.PlainText())
		}
		rows = append(rows, cells)
	}
	return rows
}

// A row that omits the optional trailing delimiter must keep an escaped pipe
// that sits at the very end of its last cell.
func TestMarkdownTableKeepsEscapedPipeWithoutTrailingDelimiter(t *testing.T) {
	content, _, err := markdownDocument("| 좌 | 우 |\n| --- | --- |\n| 가 | 나\\|\n")
	if err != nil {
		t.Fatal(err)
	}
	rows := tableCellTexts(t, content)
	if len(rows) != 2 {
		t.Fatalf("expected header and one body row, got %d: %v", len(rows), rows)
	}
	if len(rows[1]) != 2 {
		t.Fatalf("expected 2 cells in the body row, got %d: %v", len(rows[1]), rows[1])
	}
	if rows[1][1] != "나|" {
		t.Errorf("last cell = %q, want %q", rows[1][1], "나|")
	}
}

// Rows that carry the trailing delimiter, delimiter rows and header rows keep
// splitting exactly as before.
func TestSplitTableRowKeepsExistingSplits(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []string
	}{
		{"both delimiters", "| a | b |", []string{" a ", " b "}},
		{"no trailing delimiter", "| a | b", []string{" a ", " b"}},
		{"no delimiters", "a | b", []string{"a ", " b"}},
		{"escaped pipe inside cell", `| a\| | b |`, []string{" a| ", " b "}},
		{"empty last cell", "| a | |", []string{" a ", " "}},
		{"delimiter row", "|---|---|", []string{"---", "---"}},
		{"aligned delimiter row", "| :--- | ---: |", []string{" :--- ", " ---: "}},
		{"empty line", "", []string{""}},
		{"single pipe", "|", []string{""}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := splitTableRow(testCase.line)
			if len(got) != len(testCase.want) {
				t.Fatalf("splitTableRow(%q) = %q, want %q", testCase.line, got, testCase.want)
			}
			for index := range got {
				if got[index] != testCase.want[index] {
					t.Fatalf("splitTableRow(%q) = %q, want %q", testCase.line, got, testCase.want)
				}
			}
		})
	}
	if !isTableDelimiterRow("|---|---|") || !isTableDelimiterRow("| :--- | ---: |") {
		t.Error("delimiter rows must still be recognised")
	}
	if isTableDelimiterRow("| a | b |") {
		t.Error("body row must not be taken for a delimiter row")
	}
}

// muni always writes the trailing delimiter, so exporting and re-importing a
// table whose cells contain pipes must come back unchanged.
func TestMarkdownTableRoundTripKeepsPipeCells(t *testing.T) {
	original := json.RawMessage(`{"type":"doc","content":[
		{"type":"table","content":[
			{"type":"tableRow","content":[
				{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"좌"}]}]},
				{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"우"}]}]}]},
			{"type":"tableRow","content":[
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"가|나"}]}]},
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"다|"}]}]}]},
			{"type":"tableRow","content":[
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"a\\b"}]}]},
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":""}]}]}]}]}
	]}`)
	exported := renderMarkdown("", original)
	content, _, err := markdownDocument(exported)
	if err != nil {
		t.Fatalf("re-import: %v\n%s", err, exported)
	}
	rows := tableCellTexts(t, content)
	want := [][]string{{"좌", "우"}, {"가|나", "다|"}, {`a\b`, ""}}
	if len(rows) != len(want) {
		t.Fatalf("round trip rows = %v, want %v\nexported markdown:\n%s", rows, want, exported)
	}
	for index := range want {
		if len(rows[index]) != len(want[index]) {
			t.Fatalf("round trip rows = %v, want %v\nexported markdown:\n%s", rows, want, exported)
		}
		for cell := range want[index] {
			if rows[index][cell] != want[index][cell] {
				t.Fatalf("round trip rows = %v, want %v\nexported markdown:\n%s", rows, want, exported)
			}
		}
	}
}
