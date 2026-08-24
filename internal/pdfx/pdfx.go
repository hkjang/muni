// Package pdfx extracts structured text, headings, lists and images from PDF
// files so they can be imported as muni documents.
package pdfx

import (
	"context"
	"errors"
	"strings"

	"github.com/hkjang/muni/internal/richdoc"
)

// Result is everything an importer needs from a PDF file.
type Result struct {
	Document *richdoc.Node
	Assets   []richdoc.Asset
	Title    string
	Pages    int
}

// ErrNoExtractableText means the file parsed correctly but carries no text
// layer, which is what happens with scans and image-only exports.
var ErrNoExtractableText = errors.New("텍스트 레이어가 없는 PDF입니다. 스캔 문서는 OCR 후 가져와 주세요")

// maxTotalTextRuns bounds the memory a single import can use across all pages.
// Reaching it truncates the document rather than failing, so a long file still
// imports the part that fits.
const maxTotalTextRuns = 400000

// Import converts a PDF file into a muni document. The context bounds the
// work: a crafted file can be expensive to interpret, so parsing stops at the
// first page boundary after the deadline passes.
func Import(ctx context.Context, body []byte) (*Result, error) {
	doc, err := Load(body)
	if err != nil {
		return nil, err
	}
	refs := doc.pages()
	if len(refs) == 0 {
		return nil, errors.New("PDF에서 페이지를 찾지 못했습니다")
	}
	contents := make([]*pageContent, 0, len(refs))
	runs := 0
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			if len(contents) == 0 {
				return nil, errors.New("PDF를 읽는 데 시간이 너무 오래 걸립니다")
			}
			break
		}
		page := doc.renderPage(ctx, ref.dict, ref.rotate, ref.mediaBox)
		contents = append(contents, page)
		runs += len(page.texts)
		if runs >= maxTotalTextRuns {
			break
		}
	}
	document, assets := buildDocument(contents)
	if document.PlainText() == "" && len(assets) == 0 {
		return nil, ErrNoExtractableText
	}
	return &Result{
		Document: document,
		Assets:   assets,
		Title:    strings.TrimSpace(doc.Title()),
		Pages:    len(refs),
	}, nil
}
