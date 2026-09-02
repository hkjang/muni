package hwp

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

// minimalHWP is one paragraph in a compound file, as a seed the fuzzer can
// mutate into something interesting rather than starting from noise.
func minimalHWP() []byte {
	text := utf16.Encode([]rune("씨앗 문단"))
	header := make([]byte, 22)
	binary.LittleEndian.PutUint32(header[0:], uint32(len(text)))
	body := append(recordHeader(tagParaHeader, 0, len(header)), header...)
	payload := make([]byte, len(text)*2)
	for index, unit := range text {
		binary.LittleEndian.PutUint16(payload[index*2:], unit)
	}
	body = append(body, recordHeader(tagParaText, 1, len(payload))...)
	body = append(body, payload...)
	return compoundBytes([]streamSpec{
		{path: "FileHeader", data: hwpFileHeader(false, false)},
		{path: "BodyText/Section0", data: body},
	})
}

// FuzzParse feeds malformed files to the HWP reader, which runs directly on
// uploaded files.
//
// Three layers each read lengths out of the file itself: the compound file's
// sector chains and directory, the record headers, and the paragraph text.
// A number in any of them can point past the end, or back at itself.
//
// testdata/fuzz holds a file that found the one that mattered — a directory
// entry declaring a stream longer than the file it sits in, which the reader
// believed and tried to allocate. It is kept as a seed, so the ordinary test
// run answers that question every time rather than only when someone fuzzes.
// Removing the clamp makes the fuzzer rediscover it in about two seconds.
func FuzzParse(f *testing.F) {
	f.Add(minimalHWP())
	f.Add([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 1<<20 {
			t.Skip()
		}
		document, assets, _, err := Parse(body)
		if err != nil {
			return
		}
		if document == nil {
			t.Fatal("nil document with no error")
		}
		if _, err := document.JSON(); err != nil {
			t.Fatalf("imported document is not serialisable: %v", err)
		}
		for _, asset := range assets {
			if asset.Placeholder == "" {
				t.Fatal("an asset with no placeholder cannot be referred to")
			}
		}
	})
}
