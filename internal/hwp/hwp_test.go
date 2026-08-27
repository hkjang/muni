package hwp

import (
	"strings"
	"testing"
)

func hwpFile(t *testing.T, compressed, encrypted bool, sections ...[]byte) []byte {
	t.Helper()
	streams := []streamSpec{{path: "FileHeader", data: hwpFileHeader(compressed, encrypted)}}
	for index, section := range sections {
		data := section
		if compressed {
			data = deflateRaw(t, section)
		}
		streams = append(streams, streamSpec{
			path: "BodyText/Section" + string(rune('0'+index)), data: data,
		})
	}
	return buildCompound(t, streams)
}

func TestAParagraphComesThrough(t *testing.T) {
	body := paraTextRecord(units("첫 문단입니다"))
	document, _, meta, err := Parse(hwpFile(t, false, false, body))
	if err != nil {
		t.Fatal(err)
	}
	if text := document.PlainText(); !strings.Contains(text, "첫 문단입니다") {
		t.Fatalf("본문 = %q", text)
	}
	if !strings.HasPrefix(meta.Version, "5.") {
		t.Errorf("판 = %q", meta.Version)
	}
}

// A .hwp stream is raw deflate with no zlib wrapper, which is why a reader
// that reaches for zlib gets "invalid header" on a perfectly good file.
func TestACompressedFileIsRead(t *testing.T) {
	body := paraTextRecord(units("압축된 문단입니다"))
	document, _, _, err := Parse(hwpFile(t, true, false, body))
	if err != nil {
		t.Fatal(err)
	}
	if text := document.PlainText(); !strings.Contains(text, "압축된 문단입니다") {
		t.Fatalf("본문 = %q", text)
	}
}

// The trap the format's own documentation sets: a control mark's size is
// given as 8, and it means 8 WCHAR — sixteen bytes. A reader that skips eight
// lands inside the mark and reads its second half as text, which comes out as
// ideographs that were never in the document.
func TestAControlMarkIsSkippedWholeNotHalf(t *testing.T) {
	// 앞 [table mark, 8 WCHAR] 뒤
	code := make([]uint16, 0, 16)
	code = append(code, units("앞말")...)
	code = append(code, 11) // an extended control
	for filler := 0; filler < 6; filler++ {
		code = append(code, 0x4E00) // bytes inside the mark that must not be read
	}
	code = append(code, 11)
	code = append(code, units("뒷말")...)

	document, _, _, err := Parse(hwpFile(t, false, false, paraTextRecord(code)))
	if err != nil {
		t.Fatal(err)
	}
	text := document.PlainText()
	for _, want := range []string{"앞말", "뒷말"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q 가 사라졌습니다: %q", want, text)
		}
	}
	if strings.Contains(text, "一") {
		t.Errorf("표시 안쪽 바이트를 글자로 읽었습니다: %q", text)
	}
}

// A tab is a control mark too, and the same width.
func TestATabIsKeptAndSkippedWhole(t *testing.T) {
	code := make([]uint16, 0, 16)
	code = append(code, units("앞")...)
	code = append(code, 9)
	for filler := 0; filler < 6; filler++ {
		code = append(code, 0x4E00)
	}
	code = append(code, 9)
	code = append(code, units("뒤")...)

	document, _, _, err := Parse(hwpFile(t, false, false, paraTextRecord(code)))
	if err != nil {
		t.Fatal(err)
	}
	text := document.PlainText()
	if !strings.Contains(text, "앞\t뒤") {
		t.Errorf("탭이 남지 않았습니다: %q", text)
	}
}

// A line break inside a paragraph is one WCHAR, not eight.
func TestALineBreakIsOneCharacter(t *testing.T) {
	code := append(units("첫 줄"), 10)
	code = append(code, units("둘째 줄")...)
	document, _, _, err := Parse(hwpFile(t, false, false, paraTextRecord(code)))
	if err != nil {
		t.Fatal(err)
	}
	text := document.PlainText()
	for _, want := range []string{"첫 줄", "둘째 줄"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q 가 사라졌습니다: %q", want, text)
		}
	}
}

// Saying so beats importing a document of mojibake.
func TestAnEncryptedFileIsRefusedClearly(t *testing.T) {
	_, _, _, err := Parse(hwpFile(t, false, true, paraTextRecord(units("안 보임"))))
	if err == nil {
		t.Fatal("암호가 걸린 문서를 받아들였습니다")
	}
	if !strings.Contains(err.Error(), "암호") {
		t.Errorf("무엇이 문제인지 말하지 않습니다: %v", err)
	}
}

func TestSomethingThatIsNotAnHwpIsRefused(t *testing.T) {
	if _, _, _, err := Parse([]byte("이건 hwp 가 아닙니다")); err == nil {
		t.Fatal("HWP 가 아닌 파일을 받아들였습니다")
	}
}

// Several sections are read in the order their names give.
func TestSectionsAreReadInOrder(t *testing.T) {
	document, _, _, err := Parse(hwpFile(t, false, false,
		paraTextRecord(units("첫 구역")),
		paraTextRecord(units("둘째 구역"))))
	if err != nil {
		t.Fatal(err)
	}
	text := document.PlainText()
	first, second := strings.Index(text, "첫 구역"), strings.Index(text, "둘째 구역")
	if first < 0 || second < 0 {
		t.Fatalf("구역이 사라졌습니다: %q", text)
	}
	if first > second {
		t.Errorf("구역 순서가 뒤바뀌었습니다: %q", text)
	}
}
