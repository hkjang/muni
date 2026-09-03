package hwp

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"io"
)

// Inside the compound file, a .hwp stream is a run of records. Each begins
// with one uint32 holding three things at once: what the record is, how deep
// it sits, and how long it is.
//
//	bits 0-9   tag
//	bits 10-19 level
//	bits 20-31 size
//
// A size of 0xFFF means "too big to fit here", and the real size is the next
// uint32. Reading that as part of the payload is the classic way to lose the
// rest of a stream.
type record struct {
	tag   uint16
	level uint16
	data  []byte
}

const sizeInHeader = 0xFFF

// tags used by the body. The numbers are the format's own.
const (
	tagDocumentProperties = 0x010 // HWPTAG_BEGIN
	tagFaceName           = 0x013
	tagBorderFill         = 0x014
	tagCharShape          = 0x015
	tagParaShape          = 0x019
	tagStyle              = 0x01A
	tagBinData            = 0x012
	tagParaHeader         = 0x042
	tagParaText           = 0x043
	tagParaCharShape      = 0x044
	tagCtrlHeader         = 0x047
	tagListHeader         = 0x048
	tagPageDef            = 0x049
	// The tags below were wrong until real files were read. Counted from
	// HWPTAG_BEGIN (0x10): SHAPE_COMPONENT is +76, TABLE is +77, and the
	// picture's own record — where the id of its stream is written — is +85.
	// Tables worked regardless, because the cells are found through their
	// LIST_HEADERs and the TABLE tag was never consulted. Pictures did not.
	tagShapeComponent = 0x04C
	tagTable          = 0x04D
	tagShapePicture   = 0x055
)

func readRecords(raw []byte) []record {
	out := []record{}
	for offset := 0; offset+4 <= len(raw); {
		header := binary.LittleEndian.Uint32(raw[offset:])
		offset += 4
		tag := uint16(header & 0x3FF)
		level := uint16((header >> 10) & 0x3FF)
		size := int((header >> 20) & 0xFFF)
		if size == sizeInHeader {
			if offset+4 > len(raw) {
				break
			}
			size = int(binary.LittleEndian.Uint32(raw[offset:]))
			offset += 4
		}
		if size < 0 || offset+size > len(raw) {
			// A truncated file still has everything before the truncation.
			break
		}
		out = append(out, record{tag: tag, level: level, data: raw[offset : offset+size]})
		offset += size
	}
	return out
}

// inflate undoes the compression a .hwp applies to its streams.
//
// The bytes are raw deflate with no zlib wrapper, which is why a reader that
// reaches for zlib gets "invalid header" on a perfectly good file.
func inflate(raw []byte) ([]byte, error) {
	reader := flate.NewReader(bytes.NewReader(raw))
	defer reader.Close()
	// A document that decompresses to more than this is not a document.
	return io.ReadAll(io.LimitReader(reader, 256<<20))
}

// A stream's records are a flat list carrying a depth each, and what a record
// belongs to is whatever came before it at one level up. A paragraph's text,
// the shapes applied to it and the controls it holds are all its children; a
// table's cells are children of the control that holds the table, and the
// paragraphs inside a cell are children of the cell.
//
// Reading the list flat — which is all muni did — finds the text and nothing
// about where it sits.
type recordNode struct {
	record
	children []*recordNode
}

// tree groups a flat record list by the depth each record carries, and then
// gives each list its paragraphs.
//
// A LIST_HEADER — a table cell, a text box, a note — does not sit above the
// paragraphs it holds. It sits *before* them, at the same depth, and says in
// its first word how many follow. Grouped by depth alone they become its
// siblings, and every cell in every table reads as empty while its words hang
// off the table instead. Real files showed this; the fixture that let it pass
// had nested the paragraphs one level deeper, the way the reader wished they
// were.
func tree(records []record) []*recordNode {
	roots := treeByDepth(records)
	for _, root := range roots {
		adoptListParagraphs(root)
	}
	return roots
}

// adoptListParagraphs moves the paragraphs that follow each list header into
// it, using the count the header carries. A count of zero, or one that runs
// past the next list, is read as "until the next list".
func adoptListParagraphs(node *recordNode) {
	kept := make([]*recordNode, 0, len(node.children))
	for index := 0; index < len(node.children); index++ {
		child := node.children[index]
		if child.tag != tagListHeader {
			kept = append(kept, child)
			continue
		}
		count := -1
		if len(child.data) >= 4 {
			count = int(binary.LittleEndian.Uint32(child.data))
		}
		taken := 0
		for index+1 < len(node.children) {
			next := node.children[index+1]
			if next.tag == tagListHeader || (count >= 0 && taken >= count && count != 0) {
				break
			}
			if next.tag == tagParaHeader {
				taken++
			}
			child.children = append(child.children, next)
			index++
		}
		kept = append(kept, child)
	}
	node.children = kept
	for _, child := range node.children {
		adoptListParagraphs(child)
	}
}

func treeByDepth(records []record) []*recordNode {
	roots := []*recordNode{}
	// stack[depth] is the node most recently opened at that depth.
	var stack []*recordNode
	for _, item := range records {
		node := &recordNode{record: item}
		depth := int(item.level)
		if depth > len(stack) {
			// A depth that skips a level: treat it as one deeper than the
			// last, rather than dropping the record.
			depth = len(stack)
		}
		if depth == 0 {
			roots = append(roots, node)
			stack = []*recordNode{node}
			continue
		}
		parent := stack[depth-1]
		parent.children = append(parent.children, node)
		stack = append(stack[:depth], node)
	}
	return roots
}

// find returns the first child with a tag, which is how a paragraph's text or
// a control's table is reached.
func (n *recordNode) find(tag uint16) *recordNode {
	for _, child := range n.children {
		if child.tag == tag {
			return child
		}
	}
	return nil
}

// all returns every child with a tag, in order.
func (n *recordNode) all(tag uint16) []*recordNode {
	out := []*recordNode{}
	for _, child := range n.children {
		if child.tag == tag {
			out = append(out, child)
		}
	}
	return out
}
