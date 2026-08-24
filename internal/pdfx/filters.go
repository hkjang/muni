package pdfx

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"errors"
	"io"
)

const maxDecodedStream = 64 << 20

func decodeStream(doc *Document, stream *Stream) ([]byte, error) {
	data := stream.Raw
	filters := namesOf(doc.resolve(stream.Dict.get("Filter", "F")))
	parms := parmsOf(doc, stream.Dict.get("DecodeParms", "DP"), len(filters))
	for index, filter := range filters {
		var err error
		var parm Dict
		if index < len(parms) {
			parm = parms[index]
		}
		switch filter {
		case "FlateDecode", "Fl":
			data, err = flateDecode(data)
		case "LZWDecode", "LZW":
			early := 1
			if value, ok := toInt(doc.resolve(parm.get("EarlyChange"))); ok {
				early = value
			}
			data, err = lzwDecode(data, early == 1)
		case "ASCIIHexDecode", "AHx":
			data, err = asciiHexDecode(data)
		case "ASCII85Decode", "A85":
			data, err = ascii85Decode(data)
		case "RunLengthDecode", "RL":
			data, err = runLengthDecode(data)
		case "DCTDecode", "DCT", "JPXDecode", "JBIG2Decode", "CCITTFaxDecode", "CCF":
			// Image codecs: hand the payload back untouched for the caller.
			return data, nil
		case "Crypt":
			// Identity crypt filter; already handled during object loading.
		default:
			return data, nil
		}
		if err != nil {
			return nil, err
		}
		if parm != nil {
			data, err = applyPredictor(doc, data, parm)
			if err != nil {
				return nil, err
			}
		}
		if len(data) > maxDecodedStream {
			return nil, errors.New("PDF 스트림이 너무 큽니다")
		}
	}
	return data, nil
}

func namesOf(value Object) []string {
	switch typed := value.(type) {
	case Name:
		return []string{string(typed)}
	case Array:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if name, ok := item.(Name); ok {
				out = append(out, string(name))
			}
		}
		return out
	}
	return nil
}

func parmsOf(doc *Document, value Object, count int) []Dict {
	out := make([]Dict, count)
	switch typed := doc.resolve(value).(type) {
	case Dict:
		if count > 0 {
			out[0] = typed
		}
	case Array:
		for index := 0; index < count && index < len(typed); index++ {
			if dict, ok := doc.resolve(typed[index]).(Dict); ok {
				out[index] = dict
			}
		}
	}
	return out
}

func flateDecode(data []byte) ([]byte, error) {
	for len(data) > 0 && isWhitespace(data[0]) {
		data = data[1:]
	}
	if len(data) == 0 {
		return nil, nil
	}
	if reader, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
		out, readErr := readAllTolerant(reader)
		if len(out) > 0 || readErr == nil {
			return out, nil
		}
	}
	// Some producers omit the zlib wrapper or corrupt the first byte.
	out, err := readAllTolerant(flate.NewReader(bytes.NewReader(data)))
	if len(out) > 0 {
		return out, nil
	}
	if len(data) > 1 {
		if out, err2 := readAllTolerant(flate.NewReader(bytes.NewReader(data[1:]))); len(out) > 0 {
			return out, nil
		} else if err2 != nil && err == nil {
			err = err2
		}
	}
	if err == nil {
		return out, nil
	}
	return nil, err
}

// readAllTolerant keeps whatever was decoded before a truncation error, which
// is common in PDFs written by streaming generators.
func readAllTolerant(reader io.Reader) ([]byte, error) {
	var buffer bytes.Buffer
	_, err := io.Copy(&buffer, io.LimitReader(reader, maxDecodedStream))
	if err != nil && buffer.Len() > 0 {
		return buffer.Bytes(), nil
	}
	return buffer.Bytes(), err
}

func asciiHexDecode(data []byte) ([]byte, error) {
	out := make([]byte, 0, len(data)/2)
	var high byte
	haveHigh := false
	for _, c := range data {
		if c == '>' {
			break
		}
		value, ok := hexValue(c)
		if !ok {
			continue
		}
		if haveHigh {
			out = append(out, high<<4|value)
			haveHigh = false
		} else {
			high = value
			haveHigh = true
		}
	}
	if haveHigh {
		out = append(out, high<<4)
	}
	return out, nil
}

func ascii85Decode(data []byte) ([]byte, error) {
	data = bytes.TrimPrefix(bytes.TrimLeft(data, " \r\n\t"), []byte("<~"))
	out := make([]byte, 0, len(data)*4/5)
	var group [5]byte
	count := 0
	for index := 0; index < len(data); index++ {
		c := data[index]
		if isWhitespace(c) {
			continue
		}
		if c == '~' {
			break
		}
		if c == 'z' && count == 0 {
			out = append(out, 0, 0, 0, 0)
			continue
		}
		if c < '!' || c > 'u' {
			continue
		}
		group[count] = c - '!'
		count++
		if count == 5 {
			value := uint32(0)
			for _, digit := range group {
				value = value*85 + uint32(digit)
			}
			out = append(out, byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
			count = 0
		}
	}
	if count > 0 {
		for index := count; index < 5; index++ {
			group[index] = 84
		}
		value := uint32(0)
		for _, digit := range group {
			value = value*85 + uint32(digit)
		}
		full := []byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}
		out = append(out, full[:count-1]...)
	}
	return out, nil
}

func runLengthDecode(data []byte) ([]byte, error) {
	out := make([]byte, 0, len(data)*2)
	for index := 0; index < len(data); {
		length := int(data[index])
		index++
		switch {
		case length == 128:
			return out, nil
		case length < 128:
			end := index + length + 1
			if end > len(data) {
				end = len(data)
			}
			out = append(out, data[index:end]...)
			index = end
		default:
			if index >= len(data) {
				return out, nil
			}
			for count := 0; count < 257-length; count++ {
				out = append(out, data[index])
			}
			index++
		}
	}
	return out, nil
}

// lzwDecode implements the PDF/TIFF variant of LZW, including the early-change
// behaviour the standard library's decoder does not provide.
func lzwDecode(data []byte, earlyChange bool) ([]byte, error) {
	const (
		clearCode = 256
		eodCode   = 257
	)
	table := make([][]byte, 258, 4096)
	reset := func() {
		table = table[:258]
		for index := 0; index < 256; index++ {
			table[index] = []byte{byte(index)}
		}
	}
	reset()
	width := 9
	var out []byte
	var previous []byte
	bitPos := 0
	total := len(data) * 8
	early := 0
	if earlyChange {
		early = 1
	}
	for bitPos+width <= total {
		code := 0
		for count := 0; count < width; count++ {
			byteIndex := (bitPos + count) / 8
			bitIndex := 7 - uint((bitPos+count)%8)
			code = code<<1 | int((data[byteIndex]>>bitIndex)&1)
		}
		bitPos += width
		switch {
		case code == clearCode:
			reset()
			width = 9
			previous = nil
			continue
		case code == eodCode:
			return out, nil
		}
		var entry []byte
		switch {
		case code < len(table) && table[code] != nil:
			entry = table[code]
		case previous != nil:
			entry = append(append([]byte{}, previous...), previous[0])
		default:
			return out, nil
		}
		out = append(out, entry...)
		if len(out) > maxDecodedStream {
			return out, errors.New("PDF LZW 스트림이 너무 큽니다")
		}
		if previous != nil && len(table) < 4096 {
			table = append(table, append(append([]byte{}, previous...), entry[0]))
		}
		previous = entry
		switch len(table) + early {
		case 512:
			width = 10
		case 1024:
			width = 11
		case 2048:
			width = 12
		}
	}
	return out, nil
}

func applyPredictor(doc *Document, data []byte, parm Dict) ([]byte, error) {
	predictor, _ := toInt(doc.resolve(parm.get("Predictor")))
	if predictor <= 1 {
		return data, nil
	}
	colors := 1
	if value, ok := toInt(doc.resolve(parm.get("Colors"))); ok && value > 0 {
		colors = value
	}
	bits := 8
	if value, ok := toInt(doc.resolve(parm.get("BitsPerComponent"))); ok && value > 0 {
		bits = value
	}
	columns := 1
	if value, ok := toInt(doc.resolve(parm.get("Columns"))); ok && value > 0 {
		columns = value
	}
	bytesPerPixel := (colors*bits + 7) / 8
	rowLength := (colors*bits*columns + 7) / 8

	if predictor == 2 {
		if bits != 8 {
			return data, nil
		}
		for row := 0; row+rowLength <= len(data); row += rowLength {
			line := data[row : row+rowLength]
			for index := bytesPerPixel; index < len(line); index++ {
				line[index] += line[index-bytesPerPixel]
			}
		}
		return data, nil
	}

	out := make([]byte, 0, len(data))
	previous := make([]byte, rowLength)
	for offset := 0; offset+1 <= len(data); offset += rowLength + 1 {
		filter := data[offset]
		end := offset + 1 + rowLength
		if end > len(data) {
			end = len(data)
		}
		row := make([]byte, rowLength)
		copy(row, data[offset+1:end])
		switch filter {
		case 0:
		case 1:
			for index := bytesPerPixel; index < rowLength; index++ {
				row[index] += row[index-bytesPerPixel]
			}
		case 2:
			for index := 0; index < rowLength; index++ {
				row[index] += previous[index]
			}
		case 3:
			for index := 0; index < rowLength; index++ {
				left := 0
				if index >= bytesPerPixel {
					left = int(row[index-bytesPerPixel])
				}
				row[index] += byte((left + int(previous[index])) / 2)
			}
		case 4:
			for index := 0; index < rowLength; index++ {
				left, upperLeft := 0, 0
				if index >= bytesPerPixel {
					left = int(row[index-bytesPerPixel])
					upperLeft = int(previous[index-bytesPerPixel])
				}
				row[index] += byte(paeth(left, int(previous[index]), upperLeft))
			}
		default:
			return out, nil
		}
		out = append(out, row...)
		previous = row
	}
	return out, nil
}

func paeth(left, above, upperLeft int) int {
	estimate := left + above - upperLeft
	deltaLeft := abs(estimate - left)
	deltaAbove := abs(estimate - above)
	deltaUpperLeft := abs(estimate - upperLeft)
	if deltaLeft <= deltaAbove && deltaLeft <= deltaUpperLeft {
		return left
	}
	if deltaAbove <= deltaUpperLeft {
		return above
	}
	return upperLeft
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
