// Package hwpx reads .hwpx files — the XML format Hangul Office writes, and
// the one a Korean office is most likely to hand muni.
//
// The shape is familiar from .docx: a zip holding XML parts, with the text in
// Contents/section*.xml, the formatting it refers to by id in
// Contents/header.xml, and the pictures in BinData. What differs is that HWPX
// keeps a run's formatting nowhere near the run — a run names a charPr id, and
// what that id means lives in the header. muni has been through what happens
// when an importer reads only where the formatting is applied, so this one
// reads the header first.
package hwpx

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/hkjang/muni/internal/richdoc"
)

// Meta is what a section says about the page rather than about the text.
type Meta struct {
	Landscape bool
}

const maxPartBytes = 64 << 20

type importer struct {
	// charShapes and paraShapes are what the header said, by the id a run or a
	// paragraph refers to.
	charShapes map[string]charShape
	paraShapes map[string]paraShape
	styles     map[string]styleInfo
	// binary is the bytes of each BinData part, by the id a picture refers to.
	binary    map[string][]byte
	assets    []richdoc.Asset
	assetByID map[string]string
}

type charShape struct {
	bold      bool
	italic    bool
	underline bool
	strike    bool
	color     string
	sizePoint string
	family    string
}

type paraShape struct {
	align    string
	indent   int
	firstLin bool
	lineRate string
}

type styleInfo struct {
	name         string
	englishName  string
	paraShapeID  string
	charShapeID  string
	headingLevel int
}

// Parse reads a .hwpx into muni's document model.
func Parse(body []byte) (*richdoc.Node, []richdoc.Asset, Meta, error) {
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, nil, Meta{}, fmt.Errorf("HWPX 압축을 열지 못했습니다: %w", err)
	}
	files := map[string]*zip.File{}
	for _, file := range archive.File {
		files[strings.TrimPrefix(file.Name, "/")] = file
	}

	imp := &importer{
		charShapes: map[string]charShape{},
		paraShapes: map[string]paraShape{},
		styles:     map[string]styleInfo{},
		binary:     map[string][]byte{},
		assetByID:  map[string]string{},
	}
	imp.loadHeader(files)
	imp.loadBinData(files)

	sections := sectionNames(files)
	if len(sections) == 0 {
		return nil, nil, Meta{}, errors.New("HWPX 본문(Contents/section0.xml)을 찾을 수 없습니다")
	}

	document := richdoc.Doc()
	meta := Meta{}
	for _, name := range sections {
		root, err := readPart(files[name])
		if err != nil {
			continue
		}
		if imp.sectionIsLandscape(root) {
			meta.Landscape = true
		}
		document.Content = append(document.Content, imp.blocks(root)...)
	}
	if len(document.Content) == 0 {
		document.Content = []*richdoc.Node{richdoc.Paragraph()}
	}
	// A picture inside a paragraph is a shape the editor refuses; the same lift
	// the other importers do.
	richdoc.LiftImages(document)
	return document, imp.assets, meta, nil
}

// sectionNames returns the body parts in the order they are read.
//
// The spine in Contents/content.hpf names them, but the names themselves carry
// the order and a file that has lost its manifest still has its text.
func sectionNames(files map[string]*zip.File) []string {
	var out []string
	for name := range files {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "contents/section") && strings.HasSuffix(lower, ".xml") {
			out = append(out, name)
		}
	}
	sort.Slice(out, func(a, b int) bool {
		return sectionIndex(out[a]) < sectionIndex(out[b])
	})
	return out
}

func sectionIndex(name string) int {
	base := strings.ToLower(path.Base(name))
	base = strings.TrimSuffix(strings.TrimPrefix(base, "section"), ".xml")
	value, err := strconv.Atoi(base)
	if err != nil {
		return 1 << 30
	}
	return value
}

func readPart(file *zip.File) (*node, error) {
	if file == nil {
		return nil, errors.New("part missing")
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return parse(io.LimitReader(reader, maxPartBytes))
}

func readBytes(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, maxPartBytes))
}
