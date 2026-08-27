package docx

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/hkjang/muni/internal/richdoc"
)

const maxImportedImageBytes = 24 << 20

type relTarget struct {
	target   string
	kind     string
	external bool
}

type styleInfo struct {
	name    string
	basedOn string
	kind    string
	numID   string
	level   int
	// runProperties is the style's own w:rPr. A run that names a character
	// style carries none of the formatting itself — "Strong" is bold because
	// the style says so, and a reader that only looks at the run sees plain
	// text.
	runProperties *xnode
	// paragraphProperties is the style's own w:pPr, which is where an office
	// template keeps the indentation and the line spacing of its body text.
	paragraphProperties *xnode
	// firstRowFormatting says the style draws the first row differently from
	// the rest — a w:tblStylePr of type firstRow. That, and not the cells, is
	// where a Word table keeps its header.
	firstRowFormatting bool
}

type importer struct {
	rels      map[string]relTarget
	media     map[string][]byte
	styles    map[string]styleInfo
	footnotes map[string][]*richdoc.Node
	listKinds map[string]string // "numId:ilvl" -> bullet|ordered
	assets    []richdoc.Asset
	assetByID map[string]string
}

// Parse converts a .docx package into a document tree plus the images it
// embedded. Image nodes point at Asset.Placeholder so the caller can store the
// bytes wherever it keeps attachments and rewrite the source.
func Parse(body []byte) (*richdoc.Node, []richdoc.Asset, Meta, error) {
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, nil, Meta{}, fmt.Errorf("DOCX 압축을 열지 못했습니다: %w", err)
	}
	files := map[string]*zip.File{}
	for _, file := range archive.File {
		files[strings.TrimPrefix(file.Name, "/")] = file
	}
	documentPart := files["word/document.xml"]
	if documentPart == nil {
		// Some producers name the main part differently; fall back to the rels.
		for name, file := range files {
			if strings.HasPrefix(name, "word/") && strings.HasSuffix(name, "document.xml") {
				documentPart = file
				break
			}
		}
	}
	if documentPart == nil {
		return nil, nil, Meta{}, errors.New("word/document.xml을 찾을 수 없습니다")
	}

	imp := &importer{
		rels:      map[string]relTarget{},
		media:     map[string][]byte{},
		styles:    map[string]styleInfo{},
		footnotes: map[string][]*richdoc.Node{},
		listKinds: map[string]string{},
		assetByID: map[string]string{},
	}
	imp.loadRelationships(files["word/_rels/document.xml.rels"])
	imp.loadStyles(files["word/styles.xml"])
	imp.loadNumbering(files["word/numbering.xml"])
	imp.loadFootnotes(files["word/footnotes.xml"])
	imp.loadEndnotes(files["word/endnotes.xml"])
	imp.loadMedia(files)

	root, err := readXML(documentPart)
	if err != nil {
		return nil, nil, Meta{}, err
	}
	bodyNode := root.child("w", "body")
	if bodyNode == nil {
		bodyNode = root.descendant("w", "body")
	}
	if bodyNode == nil {
		return nil, nil, Meta{}, errors.New("DOCX 본문을 읽지 못했습니다")
	}
	blocks := imp.blocks(bodyNode.Children)
	document := richdoc.Doc(groupBlocks(blocks)...)
	if len(document.Content) == 0 {
		document.Content = []*richdoc.Node{richdoc.Paragraph()}
	}
	// Word keeps a picture inside the run that holds it; the editor gives it
	// a line of its own.
	richdoc.LiftImages(document)
	return document, imp.assets, imp.pageFurniture(files, bodyNode), nil
}

func readXML(file *zip.File) (*xnode, error) {
	if file == nil {
		return nil, nil
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return parseXML(io.LimitReader(reader, 64<<20))
}

func (imp *importer) loadRelationships(file *zip.File) {
	root, err := readXML(file)
	if err != nil || root == nil {
		return
	}
	for _, child := range root.Children {
		if child.Local != "Relationship" {
			continue
		}
		id := child.attr("Id")
		if id == "" {
			continue
		}
		kind := child.attr("Type")
		imp.rels[id] = relTarget{
			target:   child.attr("Target"),
			kind:     kind[strings.LastIndex(kind, "/")+1:],
			external: strings.EqualFold(child.attr("TargetMode"), "External"),
		}
	}
}

func (imp *importer) loadStyles(file *zip.File) {
	root, err := readXML(file)
	if err != nil || root == nil {
		return
	}
	for _, child := range root.Children {
		if !child.is("w", "style") {
			continue
		}
		id := child.attr("w:styleId")
		if id == "" {
			continue
		}
		info := styleInfo{
			name:                child.child("w", "name").val(),
			basedOn:             child.child("w", "basedOn").val(),
			kind:                child.attr("w:type"),
			runProperties:       child.child("w", "rPr"),
			paragraphProperties: child.child("w", "pPr"),
		}
		for _, conditional := range child.Children {
			if conditional.is("w", "tblStylePr") && conditional.attr("w:type") == "firstRow" {
				info.firstRowFormatting = len(conditional.Children) > 0
			}
		}
		// Word attaches list numbering to the style itself for the built-in
		// "List Bullet" and "List Number" families.
		if numbering := child.child("w", "pPr").child("w", "numPr"); numbering != nil {
			info.numID = numbering.child("w", "numId").val()
			info.level, _ = strconv.Atoi(numbering.child("w", "ilvl").val())
		}
		imp.styles[id] = info
	}
}

func (imp *importer) loadNumbering(file *zip.File) {
	root, err := readXML(file)
	if err != nil || root == nil {
		return
	}
	abstract := map[string]map[int]string{}
	for _, child := range root.Children {
		if !child.is("w", "abstractNum") {
			continue
		}
		id := child.attr("w:abstractNumId")
		levels := map[int]string{}
		for _, level := range child.Children {
			if !level.is("w", "lvl") {
				continue
			}
			index, _ := strconv.Atoi(level.attr("w:ilvl"))
			format := level.child("w", "numFmt").val()
			text := level.child("w", "lvlText").val()
			levels[index] = classifyListFormat(format, text)
		}
		abstract[id] = levels
	}
	for _, child := range root.Children {
		if !child.is("w", "num") {
			continue
		}
		numID := child.attr("w:numId")
		abstractID := child.child("w", "abstractNumId").val()
		levels := abstract[abstractID]
		for index, kind := range levels {
			imp.listKinds[numID+":"+strconv.Itoa(index)] = kind
		}
		// Level overrides can change the format for a single list instance.
		for _, override := range child.Children {
			if !override.is("w", "lvlOverride") {
				continue
			}
			index, _ := strconv.Atoi(override.attr("w:ilvl"))
			if level := override.child("w", "lvl"); level != nil {
				imp.listKinds[numID+":"+strconv.Itoa(index)] = classifyListFormat(level.child("w", "numFmt").val(), level.child("w", "lvlText").val())
			}
		}
	}
}

func classifyListFormat(format, text string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "bullet", "none", "":
		if isCheckboxGlyph(text) {
			return "task"
		}
		return "bullet"
	default:
		return "ordered"
	}
}

func isCheckboxGlyph(text string) bool {
	trimmed := strings.TrimSpace(text)
	switch trimmed {
	case "☐", "☑", "☒", "□", "", "", "":
		return true
	}
	return false
}

func (imp *importer) loadMedia(files map[string]*zip.File) {
	for name, file := range files {
		if !strings.HasPrefix(name, "word/media/") && !strings.HasPrefix(name, "word/embeddings/") {
			continue
		}
		if file.UncompressedSize64 > maxImportedImageBytes {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(reader, maxImportedImageBytes))
		reader.Close()
		if err != nil || len(data) == 0 {
			continue
		}
		imp.media[name] = data
	}
}

func (imp *importer) mediaFor(relID string) ([]byte, string, bool) {
	rel, ok := imp.rels[relID]
	if !ok || rel.external {
		return nil, "", false
	}
	target := rel.target
	candidates := []string{
		"word/" + strings.TrimPrefix(target, "./"),
		strings.TrimPrefix(target, "/"),
		"word/media/" + path.Base(target),
	}
	for _, candidate := range candidates {
		if data, ok := imp.media[path.Clean(candidate)]; ok {
			return data, path.Base(candidate), true
		}
	}
	return nil, "", false
}

// styleKey normalises a paragraph style into the semantic name the converter
// switches on, tolerating localised style names such as "제목 1".
func (imp *importer) styleKey(styleID string) string {
	seen := map[string]bool{}
	current := styleID
	for current != "" && !seen[current] {
		seen[current] = true
		info := imp.styles[current]
		if key := normalizeStyleName(current); key != "" {
			return key
		}
		if key := normalizeStyleName(info.name); key != "" {
			return key
		}
		current = info.basedOn
	}
	return ""
}

// styleNumbering walks the basedOn chain looking for numbering attached to a
// paragraph style rather than to the paragraph itself.
func (imp *importer) styleNumbering(styleID string) (string, int, bool) {
	seen := map[string]bool{}
	current := styleID
	for current != "" && !seen[current] {
		seen[current] = true
		info := imp.styles[current]
		if info.numID != "" && info.numID != "0" {
			return info.numID, info.level, true
		}
		current = info.basedOn
	}
	return "", 0, false
}

func normalizeStyleName(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, "_", "")
	switch normalized {
	case "title", "제목", "책제목":
		return "title"
	case "subtitle", "부제":
		return "subtitle"
	case "quote", "intensequote", "blockquote", "인용", "인용구":
		return "quote"
	case "codeblock", "code", "sourcecode", "htmlpreformatted", "preformattedtext", "코드":
		return "code"
	case "listparagraph", "목록단락":
		return "list"
	case "caption", "캡션":
		return "caption"
	}
	compact := strings.ReplaceAll(normalized, " ", "")
	switch compact {
	case "listbullet", "목록글머리기호", "listcontinue":
		return "listbullet1"
	case "listnumber", "목록번호":
		return "listnumber1"
	}
	for level := 2; level <= 9; level++ {
		digit := strconv.Itoa(level)
		if compact == "listbullet"+digit || compact == "목록글머리기호"+digit {
			return "listbullet" + digit
		}
		if compact == "listnumber"+digit || compact == "목록번호"+digit {
			return "listnumber" + digit
		}
	}
	for level := 1; level <= 9; level++ {
		digit := strconv.Itoa(level)
		if compact == "heading"+digit || compact == "제목"+digit || compact == "머리글"+digit || compact == "h"+digit {
			return "heading" + digit
		}
	}
	return ""
}

// Meta is what a .docx says around the document rather than inside it.
//
// A Korean office document carries its classification in the page header —
// 대외비, 부서명, 문서번호 — and until now importing one dropped those parts
// entirely and silently. Losing "대외비" is not a formatting loss.
type Meta struct {
	Header string
	Footer string
	// Landscape is set when the document's section is turned sideways, which
	// is how a wide table gets printed. muni holds one orientation for the
	// whole document; a file that changes it partway keeps the first.
	Landscape bool
}

// furnitureText reads a header or footer as the words it carries, leaving out
// the fields and the punctuation that only held them together.
//
// muni's model has one string per header and footer — there is nowhere to put
// a page counter. Reading a field's result would take whatever number was
// stored the last time the file was written, so "내부 열람용" would come back as
// "내부 열람용1 / 1", a little longer after every round trip.
//
// Dropping the result is not enough on its own. A footer laid out the way muni
// lays it out is "내부 열람용 [PAGE] / [NUMPAGES]", and the slash between the two
// fields is an ordinary run; a Word header is often "[STYLEREF] · 대외비", where
// the middle dot is one too. Both are punctuation that exists to separate a
// field from something, so a piece of text that is only separators and sits
// next to a field goes with the field. A middle dot the author typed, with
// words on both sides, stays.
func furnitureText(root *xnode) string {
	pieces := []furniturePiece{}
	addText := func(value string) {
		if len(pieces) > 0 && !pieces[len(pieces)-1].field {
			pieces[len(pieces)-1].text += value
			return
		}
		pieces = append(pieces, furniturePiece{text: value})
	}
	fields, inResult := 0, false
	var walk func(*xnode)
	walk = func(node *xnode) {
		if node == nil {
			return
		}
		switch {
		case node.is("w", "fldChar"):
			switch strings.ToLower(node.attr("w:fldCharType")) {
			case "begin":
				if fields == 0 {
					pieces = append(pieces, furniturePiece{field: true})
				}
				fields++
			case "separate":
				inResult = true
			case "end":
				if fields > 0 {
					fields--
				}
				if fields == 0 {
					inResult = false
				}
			}
			return
		case node.is("w", "instrText"):
			return
		case node.is("w", "fldSimple"):
			pieces = append(pieces, furniturePiece{field: true})
			return
		case !inResult && (node.is("w", "t") || node.is("w", "delText")):
			addText(node.Text)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)

	var out strings.Builder
	for index, current := range pieces {
		if current.field {
			continue
		}
		text := current.text
		if index > 0 && pieces[index-1].field {
			text = strings.TrimLeftFunc(text, isFieldJoiner)
		}
		if index+1 < len(pieces) && pieces[index+1].field {
			text = strings.TrimRightFunc(text, isFieldJoiner)
		}
		out.WriteString(text)
	}
	return out.String()
}

// isFieldJoiner reports whether a rune is the sort of punctuation that sits
// between a field and its neighbour rather than saying anything itself. Only
// the marks that join: a comma or a full stop belongs to whoever typed it.
func isFieldJoiner(r rune) bool {
	switch r {
	case ' ', '\t', '\u00a0', '/', '-', '~', '·', '|', '(', ')', '[', ']':
		return true
	}
	return false
}

// furniturePiece is a run of a header or footer: either its words, or the
// place a field stood.
type furniturePiece struct {
	text  string
	field bool
}

// pageFurniture pulls the default header and footer text out of the package.
//
// Word can hold three of each (first page, even pages, odd pages). muni has
// one, so the default is taken and the others are left: showing the first-page
// header on every page would be a different document.
func (imp *importer) pageFurniture(files map[string]*zip.File, body *xnode) Meta {
	section := body.child("w", "sectPr")
	if section == nil {
		section = body.descendant("w", "sectPr")
	}
	if section == nil {
		return Meta{}
	}
	pick := func(local string) string {
		var fallback string
		for _, child := range section.Children {
			if child.Local != local {
				continue
			}
			id := child.attr("r:id")
			if id == "" {
				id = child.attr("id")
			}
			rel, ok := imp.rels[id]
			if !ok || rel.external {
				continue
			}
			name := "word/" + strings.TrimPrefix(rel.target, "/")
			file := files[name]
			if file == nil {
				file = files[strings.TrimPrefix(rel.target, "/")]
			}
			if file == nil {
				continue
			}
			root, err := readXML(file)
			if err != nil || root == nil {
				continue
			}
			text := collapseSpaces(furnitureText(root))
			if text == "" {
				continue
			}
			if child.attr("w:type") == "default" {
				return text
			}
			if fallback == "" {
				fallback = text
			}
		}
		return fallback
	}
	landscape := false
	if size := section.child("w", "pgSz"); size != nil {
		// w:orient is what Word writes and reads. Some producers only swap the
		// numbers, so a page wider than it is tall counts too.
		if strings.EqualFold(size.attr("w:orient"), "landscape") {
			landscape = true
		} else {
			width, wErr := strconv.Atoi(size.attr("w:w"))
			height, hErr := strconv.Atoi(size.attr("w:h"))
			landscape = wErr == nil && hErr == nil && width > height
		}
	}
	return Meta{
		Header:    truncateRunes(pick("headerReference"), 200),
		Footer:    truncateRunes(pick("footerReference"), 200),
		Landscape: landscape,
	}
}

// collapseSpaces turns the runs and tabs a header is built from into one line.
// A header laid out as "기획조정실 <tab> 대외비" reads as two columns on the
// page and as one sentence anywhere else.
func collapseSpaces(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

// loadFootnotes reads the notes so the body walk can attach each one where its
// reference sits.
//
// Ids 0 and 1 are the separator lines Word draws above the notes, not notes.
// The reference mark at the head of a note is dropped: muni numbers notes by
// their order in the document, so a number carried in from the file would be
// the old document's number.
// loadFootnotes reads word/footnotes.xml, and loadEndnotes reads
// word/endnotes.xml into the same place.
//
// muni has one kind of note. An endnote and a footnote differ only in where
// they are printed, and muni already prints its notes at the end of a PDF
// because the browser it prints through has no way to put them on the page
// they belong to. Reading 미주 as 각주 keeps the words and the place in the
// sentence they were attached to; dropping them keeps neither.
func (imp *importer) loadFootnotes(file *zip.File) {
	imp.loadNotes(file, "footnote")
}

func (imp *importer) loadEndnotes(file *zip.File) {
	imp.loadNotes(file, "endnote")
}

func (imp *importer) loadNotes(file *zip.File, local string) {
	root, err := readXML(file)
	if err != nil || root == nil {
		return
	}
	for _, child := range root.Children {
		if child.Local != local {
			continue
		}
		switch child.attr("w:type") {
		case "separator", "continuationSeparator", "continuationNotice":
			continue
		}
		// Only the type says what a separator is. Word numbers its separators
		// -1 and 0 and its first real note 1; muni's own writer numbers them
		// 0 and 1 and starts at 2. A guard on the number was tuned to muni's
		// output and threw away the first footnote of every Word file.
		id := child.attr("w:id")
		if id == "" {
			continue
		}
		content := noteContent(child)
		if len(content) == 0 {
			continue
		}
		// The two files number themselves separately, so a footnote 2 and an
		// endnote 2 would collide.
		imp.footnotes[local+":"+id] = content
	}
}

// noteContent reads a note as the lines it was written in, joined by a space.
//
// A note is often more than one paragraph — a citation and then a remark about
// it — and reading the whole part as one string ran them together:
// "첫째 줄입니다둘째 줄입니다". The words were all there and the sentence was gone.
//
// The join is a space rather than a hardBreak because muni's note holds text
// and nothing else: `content: "text*"`. A break inside one is a shape the
// schema cannot represent, and ProseMirror does not complain when the document
// is loaded — it throws on the first edit that touches the note, which is
// worse than not loading at all.
func noteContent(note *xnode) []*richdoc.Node {
	lines := []string{}
	for _, child := range note.Children {
		if !child.is("w", "p") {
			continue
		}
		if text := collapseSpaces(child.allText()); text != "" {
			lines = append(lines, text)
		}
	}
	if len(lines) == 0 {
		// A note Word wrapped in a content control, or one holding a table,
		// has no paragraph of its own to read. Its words are still words.
		if text := collapseSpaces(note.allText()); text != "" {
			lines = append(lines, text)
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return []*richdoc.Node{richdoc.Text(strings.Join(lines, " "))}
}
