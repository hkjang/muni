package pdfx

import (
	"bytes"
	"errors"
	"strconv"
)

type Document struct {
	data      []byte
	offsets   map[int]int    // object number -> byte offset of "N G obj"
	cache     map[int]Object // resolved top-level objects
	loading   map[int]bool
	trailer   Dict
	decryptor *decryptor
	inObjStm  map[int]bool
}

// Load indexes a PDF by scanning for object headers rather than trusting the
// cross-reference table, which is frequently damaged in files that still open
// fine in a viewer.
func Load(data []byte) (*Document, error) {
	if !bytes.Contains(first(data, 1024), []byte("%PDF-")) && !bytes.Contains(data, []byte(" obj")) {
		return nil, errors.New("PDF 파일이 아닙니다")
	}
	doc := &Document{
		data:     data,
		offsets:  map[int]int{},
		cache:    map[int]Object{},
		loading:  map[int]bool{},
		trailer:  Dict{},
		inObjStm: map[int]bool{},
	}
	doc.scanObjects()
	doc.collectTrailers()
	if err := doc.setupDecryption(); err != nil {
		return nil, err
	}
	doc.expandObjectStreams()
	return doc, nil
}

func first(data []byte, count int) []byte {
	if len(data) < count {
		return data
	}
	return data[:count]
}

// scanObjects records every "<num> <gen> obj" header in the file. Later
// definitions win so incremental updates resolve to their newest revision.
func (d *Document) scanObjects() {
	data := d.data
	for index := 0; index+3 < len(data); {
		position := bytes.Index(data[index:], []byte("obj"))
		if position < 0 {
			return
		}
		absolute := index + position
		index = absolute + 3
		if absolute+3 < len(data) && !isWhitespace(data[absolute+3]) && !isDelimiter(data[absolute+3]) {
			continue
		}
		cursor := absolute - 1
		for cursor >= 0 && isWhitespace(data[cursor]) {
			cursor--
		}
		generationEnd := cursor + 1
		for cursor >= 0 && data[cursor] >= '0' && data[cursor] <= '9' {
			cursor--
		}
		generationStart := cursor + 1
		if generationStart == generationEnd {
			continue
		}
		for cursor >= 0 && isWhitespace(data[cursor]) {
			cursor--
		}
		numberEnd := cursor + 1
		if numberEnd == generationStart {
			continue
		}
		for cursor >= 0 && data[cursor] >= '0' && data[cursor] <= '9' {
			cursor--
		}
		numberStart := cursor + 1
		if numberStart == numberEnd {
			continue
		}
		if cursor >= 0 && !isWhitespace(data[cursor]) && !isDelimiter(data[cursor]) {
			continue
		}
		number, err := strconv.Atoi(string(data[numberStart:numberEnd]))
		if err != nil || number <= 0 {
			continue
		}
		d.offsets[number] = numberStart
	}
}

func (d *Document) collectTrailers() {
	for index := 0; ; {
		position := bytes.Index(d.data[index:], []byte("trailer"))
		if position < 0 {
			break
		}
		absolute := index + position + len("trailer")
		index = absolute
		parser := newParser(d.data, absolute)
		if value, err := parser.object(0); err == nil {
			if dict, ok := value.(Dict); ok {
				d.mergeTrailer(dict)
			}
		}
	}
	if d.trailer.get("Root") == nil || d.trailer.get("Encrypt") != nil {
		// Cross-reference streams carry the trailer for PDF 1.5+ files.
		for number := range d.offsets {
			object := d.loadAt(number)
			stream, ok := object.(*Stream)
			if !ok {
				continue
			}
			if name, _ := stream.Dict.get("Type").(Name); name == "XRef" {
				d.mergeTrailer(stream.Dict)
			}
		}
	}
}

func (d *Document) mergeTrailer(dict Dict) {
	for _, key := range []Name{"Root", "Encrypt", "Info", "ID", "Size"} {
		if value, ok := dict[key]; ok {
			if _, exists := d.trailer[key]; !exists || key == "Root" {
				d.trailer[key] = value
			}
		}
	}
}

func (d *Document) expandObjectStreams() {
	numbers := make([]int, 0, len(d.offsets))
	for number := range d.offsets {
		numbers = append(numbers, number)
	}
	for _, number := range numbers {
		stream, ok := d.loadAt(number).(*Stream)
		if !ok {
			continue
		}
		if name, _ := stream.Dict.get("Type").(Name); name != "ObjStm" {
			continue
		}
		d.expandObjectStream(stream)
	}
}

func (d *Document) expandObjectStream(stream *Stream) {
	content, err := d.StreamData(stream)
	if err != nil || len(content) == 0 {
		return
	}
	count, _ := toInt(d.resolve(stream.Dict.get("N")))
	firstOffset, _ := toInt(d.resolve(stream.Dict.get("First")))
	if count <= 0 || firstOffset <= 0 || firstOffset > len(content) {
		return
	}
	header := newParser(content, 0)
	type entry struct{ number, offset int }
	entries := make([]entry, 0, count)
	for index := 0; index < count; index++ {
		numberToken := header.read()
		offsetToken := header.read()
		if numberToken.kind != tokenNumber || offsetToken.kind != tokenNumber {
			break
		}
		entries = append(entries, entry{number: int(numberToken.integer), offset: int(offsetToken.integer)})
	}
	for _, item := range entries {
		start := firstOffset + item.offset
		if start < 0 || start >= len(content) {
			continue
		}
		if _, exists := d.cache[item.number]; exists && !d.inObjStm[item.number] {
			continue
		}
		if _, exists := d.offsets[item.number]; exists && !d.inObjStm[item.number] {
			continue
		}
		parser := newParser(content, start)
		value, err := parser.object(0)
		if err != nil {
			continue
		}
		d.cache[item.number] = value
		d.inObjStm[item.number] = true
	}
}

// loadAt parses the top-level object with the given number, memoising results.
func (d *Document) loadAt(number int) Object {
	if value, ok := d.cache[number]; ok {
		return value
	}
	offset, ok := d.offsets[number]
	if !ok || d.loading[number] {
		return nil
	}
	d.loading[number] = true
	defer delete(d.loading, number)

	parser := newParser(d.data, offset)
	numberToken := parser.read()
	generationToken := parser.read()
	keyword := parser.read()
	if numberToken.kind != tokenNumber || generationToken.kind != tokenNumber || keyword.kind != tokenKeyword || keyword.text != "obj" {
		d.cache[number] = nil
		return nil
	}
	value, err := parser.object(0)
	if err != nil {
		d.cache[number] = nil
		return nil
	}
	reference := Ref{Number: number, Generation: int(generationToken.integer)}
	if dict, ok := value.(Dict); ok {
		if stream, consumed := d.readStream(parser, dict, reference); consumed {
			d.cache[number] = stream
			return stream
		}
	}
	value = d.decryptObject(value, reference)
	d.cache[number] = value
	return value
}

// readStream consumes the "stream ... endstream" body that may follow a dict,
// preferring the declared /Length but falling back to the endstream marker.
func (d *Document) readStream(parser *parser, dict Dict, reference Ref) (*Stream, bool) {
	next := parser.read()
	if next.kind != tokenKeyword || next.text != "stream" {
		parser.unread(next)
		return nil, false
	}
	position := parser.pos
	if position < len(d.data) && d.data[position] == '\r' {
		position++
	}
	if position < len(d.data) && d.data[position] == '\n' {
		position++
	}
	length := -1
	if value, ok := toInt(d.resolve(dict.get("Length"))); ok {
		length = value
	}
	end := -1
	if length >= 0 && position+length <= len(d.data) {
		candidate := position + length
		trailing := d.data[candidate:min(candidate+20, len(d.data))]
		trimmed := bytes.TrimLeft(trailing, "\r\n \t")
		if bytes.HasPrefix(trimmed, []byte("endstream")) {
			end = candidate
		}
	}
	if end < 0 {
		marker := bytes.Index(d.data[position:], []byte("endstream"))
		if marker < 0 {
			end = len(d.data)
		} else {
			end = position + marker
			for end > position && (d.data[end-1] == '\n' || d.data[end-1] == '\r') {
				end--
			}
		}
	}
	raw := d.data[position:end]
	if d.decryptor != nil && !d.decryptor.identityStreams {
		if name, _ := dict.get("Type").(Name); name != "XRef" {
			raw = d.decryptor.decrypt(raw, reference, false)
		}
	}
	return &Stream{Dict: d.decryptObject(dict, reference).(Dict), Raw: raw, ref: reference}, true
}

func (d *Document) resolve(value Object) Object {
	for depth := 0; depth < 32; depth++ {
		reference, ok := value.(Ref)
		if !ok {
			return value
		}
		value = d.loadAt(reference.Number)
	}
	return nil
}

func (d *Document) dict(value Object) Dict {
	dict, _ := d.resolve(value).(Dict)
	if dict == nil {
		if stream, ok := d.resolve(value).(*Stream); ok {
			return stream.Dict
		}
	}
	return dict
}

func (d *Document) array(value Object) Array {
	array, _ := d.resolve(value).(Array)
	return array
}

func (d *Document) name(value Object) string {
	name, _ := d.resolve(value).(Name)
	return string(name)
}

func (d *Document) number(value Object, fallback float64) float64 {
	if number, ok := toFloat(d.resolve(value)); ok {
		return number
	}
	return fallback
}

// StreamData returns the fully decoded contents of a stream.
func (d *Document) StreamData(stream *Stream) ([]byte, error) {
	if stream == nil {
		return nil, errors.New("빈 스트림")
	}
	return decodeStream(d, stream)
}

func (d *Document) catalog() Dict {
	if root := d.dict(d.trailer.get("Root")); root != nil {
		return root
	}
	for number := range d.offsets {
		if dict, ok := d.loadAt(number).(Dict); ok {
			if name, _ := dict.get("Type").(Name); name == "Catalog" {
				return dict
			}
		}
	}
	for number := range d.cache {
		if dict, ok := d.cache[number].(Dict); ok {
			if name, _ := dict.get("Type").(Name); name == "Catalog" {
				return dict
			}
		}
	}
	return nil
}
