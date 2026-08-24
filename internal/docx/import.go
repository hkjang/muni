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
}

type importer struct {
	rels      map[string]relTarget
	media     map[string][]byte
	styles    map[string]styleInfo
	listKinds map[string]string // "numId:ilvl" -> bullet|ordered
	assets    []richdoc.Asset
	assetByID map[string]string
}

// Parse converts a .docx package into a document tree plus the images it
// embedded. Image nodes point at Asset.Placeholder so the caller can store the
// bytes wherever it keeps attachments and rewrite the source.
func Parse(body []byte) (*richdoc.Node, []richdoc.Asset, error) {
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, nil, fmt.Errorf("DOCX 압축을 열지 못했습니다: %w", err)
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
		return nil, nil, errors.New("word/document.xml을 찾을 수 없습니다")
	}

	imp := &importer{
		rels:      map[string]relTarget{},
		media:     map[string][]byte{},
		styles:    map[string]styleInfo{},
		listKinds: map[string]string{},
		assetByID: map[string]string{},
	}
	imp.loadRelationships(files["word/_rels/document.xml.rels"])
	imp.loadStyles(files["word/styles.xml"])
	imp.loadNumbering(files["word/numbering.xml"])
	imp.loadMedia(files)

	root, err := readXML(documentPart)
	if err != nil {
		return nil, nil, err
	}
	bodyNode := root.child("w", "body")
	if bodyNode == nil {
		bodyNode = root.descendant("w", "body")
	}
	if bodyNode == nil {
		return nil, nil, errors.New("DOCX 본문을 읽지 못했습니다")
	}
	blocks := imp.blocks(bodyNode.Children)
	document := richdoc.Doc(groupBlocks(blocks)...)
	if len(document.Content) == 0 {
		document.Content = []*richdoc.Node{richdoc.Paragraph()}
	}
	return document, imp.assets, nil
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
			name:    child.child("w", "name").val(),
			basedOn: child.child("w", "basedOn").val(),
			kind:    child.attr("w:type"),
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
