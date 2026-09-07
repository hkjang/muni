package hangul

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestLinkAddressKeepsOnlyAddressesADocumentMayCarry(t *testing.T) {
	cases := map[string]string{
		`http\://www.hancom.co.kr;1;0;0;`: "http://www.hancom.co.kr",
		`https\://example.com/보고서;1;0;`:   "https://example.com/보고서",
		`mailto\:hong@example.com;1;`:     "mailto:hong@example.com",
		// A bare host is a web address and is completed as one.
		`www.hancom.co.kr;1;0;0;`: "http://www.hancom.co.kr",
		// Anything that would run instead of open is not a destination.
		`javascript\:alert(1);1;0;`:   "",
		`JaVaScRiPt\:alert(1)`:        "",
		`data\:text/html,<script>;1;`: "",
		`file\:///etc/passwd;1;`:      "",
		`vbscript\:msgbox(1)`:         "",
		``:                            "",
		`;1;0;0;`:                     "",
	}
	for command, want := range cases {
		if got := LinkAddress(command); got != want {
			t.Errorf("LinkAddress(%q) = %q, want %q", command, got, want)
		}
	}
}

func archiveOf(t *testing.T, entries int, each int) *zip.Reader {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for index := 0; index < entries; index++ {
		part, err := writer.Create("part" + string(rune('a'+index%26)) + string(rune('a'+index/26)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(bytes.Repeat([]byte{'a'}, each)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func TestCheckArchiveAllowsARealDocumentAndRefusesABomb(t *testing.T) {
	if err := CheckArchive(archiveOf(t, 40, 1024)); err != nil {
		t.Errorf("평범한 문서를 거절했습니다: %v", err)
	}
	if err := CheckArchive(archiveOf(t, MaxArchiveEntries+1, 1)); err == nil {
		t.Errorf("부품이 %d개가 넘는 파일을 받아들였습니다", MaxArchiveEntries)
	}
	// The declared sizes are read from the directory, so a bomb is refused
	// without unpacking a byte of it.
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	part, _ := writer.Create("bomb")
	// A megabyte of one letter compresses to almost nothing; ask for enough
	// of them to pass the cap.
	block := bytes.Repeat([]byte{'a'}, 1<<20)
	for index := 0; index < (MaxArchiveBytes>>20)+1; index++ {
		if _, err := part.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	writer.Close()
	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckArchive(reader); err == nil {
		t.Errorf("압축 폭탄을 받아들였습니다 (푼 크기 %d바이트)", reader.File[0].UncompressedSize64)
	}
}
