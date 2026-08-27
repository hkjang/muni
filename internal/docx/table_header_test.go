package docx

import (
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// Word almost never marks a header row on the row. w:tblHeader means "repeat
// this at the top of every page", which most tables leave off; a header row is
// the table style's firstRow formatting, switched on by w:tblLook. Reading the
// cells finds nothing either, because the bold and the shading live in the
// style rather than on them — so muni imported every Word table as a table
// with no header at all.

const tableStyles = `<w:style w:type="table" w:styleId="Shaded"><w:name w:val="Light Shading"/>` +
	`<w:tblStylePr w:type="firstRow"><w:rPr><w:b/></w:rPr>` +
	`<w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="D9E2F3"/></w:tcPr></w:tblStylePr></w:style>` +
	`<w:style w:type="table" w:styleId="Grid"><w:name w:val="Table Grid"/></w:style>` +
	`<w:style w:type="table" w:styleId="ShadedChild"><w:name w:val="우리표"/><w:basedOn w:val="Shaded"/></w:style>`

func wordTable(styleID, look string, rows ...[]string) string {
	out := `<w:tbl><w:tblPr>`
	if styleID != "" {
		out += `<w:tblStyle w:val="` + styleID + `"/>`
	}
	out += look + `</w:tblPr>`
	for _, row := range rows {
		out += `<w:tr>`
		for _, cell := range row {
			out += `<w:tc><w:tcPr/><w:p><w:r><w:t>` + escapeXML(cell) + `</w:t></w:r></w:p></w:tc>`
		}
		out += `</w:tr>`
	}
	return out + `</w:tbl>`
}

func cellTypesOf(t *testing.T, document *richdoc.Node) []string {
	t.Helper()
	var types []string
	var walk func(*richdoc.Node)
	walk = func(node *richdoc.Node) {
		if node == nil {
			return
		}
		if node.Type == "tableCell" || node.Type == "tableHeader" {
			types = append(types, node.Type)
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(document)
	return types
}

func importTable(t *testing.T, body string) []string {
	t.Helper()
	document, _, _, err := Parse(wordPackageWithStyles(t, body, tableStyles))
	if err != nil {
		t.Fatal(err)
	}
	return cellTypesOf(t, document)
}

func TestAStyledFirstRowIsAHeader(t *testing.T) {
	body := wordTable("Shaded", `<w:tblLook w:val="04A0" w:firstRow="1" w:lastRow="0"/>`,
		[]string{"머리하나", "머리둘"}, []string{"값하나", "값둘"})
	got := importTable(t, body)
	want := []string{"tableHeader", "tableHeader", "tableCell", "tableCell"}
	for index, cell := range want {
		if index >= len(got) || got[index] != cell {
			t.Fatalf("칸 = %v, %v 를 기대했습니다", got, want)
		}
	}
}

// Table Grid draws every row alike. w:tblLook is on there too — Word writes it
// on nearly every table — so believing it alone would invent a header the
// document does not have.
func TestAUniformStyleGetsNoHeader(t *testing.T) {
	body := wordTable("Grid", `<w:tblLook w:val="04A0" w:firstRow="1"/>`,
		[]string{"칸하나", "칸둘"}, []string{"값하나", "값둘"})
	for _, cell := range importTable(t, body) {
		if cell == "tableHeader" {
			t.Fatalf("머리글을 지어냈습니다: %v", importTable(t, body))
		}
	}
}

// A house style built on a shaded one is still shaded.
func TestAStyleBuiltOnAShadedOneKeepsTheHeader(t *testing.T) {
	body := wordTable("ShadedChild", `<w:tblLook w:firstRow="1"/>`,
		[]string{"머리하나"}, []string{"값하나"})
	if got := importTable(t, body); len(got) == 0 || got[0] != "tableHeader" {
		t.Fatalf("칸 = %v", got)
	}
}

// Turning the look off is the writer saying the first row is ordinary.
func TestFirstRowOffMeansNoHeader(t *testing.T) {
	body := wordTable("Shaded", `<w:tblLook w:val="0000" w:firstRow="0"/>`,
		[]string{"칸하나"}, []string{"값하나"})
	for _, cell := range importTable(t, body) {
		if cell == "tableHeader" {
			t.Fatal("firstRow=0 인데 머리글이 되었습니다")
		}
	}
}

// Older files carry the look only as a hex bitmask; 0x0020 is the first-row bit.
func TestTheOldBitmaskIsRead(t *testing.T) {
	body := wordTable("Shaded", `<w:tblLook w:val="04A0"/>`, []string{"머리하나"}, []string{"값하나"})
	if got := importTable(t, body); len(got) == 0 || got[0] != "tableHeader" {
		t.Fatalf("칸 = %v", got)
	}
	plain := wordTable("Shaded", `<w:tblLook w:val="0480"/>`, []string{"칸하나"}, []string{"값하나"})
	for _, cell := range importTable(t, plain) {
		if cell == "tableHeader" {
			t.Fatal("첫 행 비트가 없는데 머리글이 되었습니다")
		}
	}
}

// The printing instruction still means what it always meant.
func TestARepeatingRowIsStillAHeader(t *testing.T) {
	body := `<w:tbl><w:tblPr/>` +
		`<w:tr><w:trPr><w:tblHeader/></w:trPr><w:tc><w:tcPr/><w:p><w:r><w:t>머리하나</w:t></w:r></w:p></w:tc></w:tr>` +
		`<w:tr><w:tc><w:tcPr/><w:p><w:r><w:t>값하나</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`
	if got := importTable(t, body); len(got) == 0 || got[0] != "tableHeader" {
		t.Fatalf("칸 = %v", got)
	}
}
