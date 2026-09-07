package hangul

import "archive/zip"

// A .docx and a .hwpx are both zips, and a zip says how large each of its
// entries will be before anything is read. Believing a small file cannot
// become a large one is the mistake a zip bomb is built on: deflate alone
// reaches about a thousand to one, so a twenty-megabyte upload can declare
// twenty gigabytes, and a reader that opens every entry will try to hold it.
const (
	// MaxArchiveEntries is more parts than any document has. A file with more
	// is not a document with many pictures; it is a file made to be counted.
	MaxArchiveEntries = 4096
	// MaxArchiveBytes is what all the entries may come to once unpacked.
	MaxArchiveBytes = 512 << 20
)

// CheckArchive reports whether an archive is within the size a document can
// honestly need. It reads only the directory the zip carries, so nothing is
// unpacked to find out.
func CheckArchive(archive *zip.Reader) error {
	if len(archive.File) > MaxArchiveEntries {
		return ErrArchiveTooLarge
	}
	total := uint64(0)
	for _, file := range archive.File {
		total += file.UncompressedSize64
		if total > MaxArchiveBytes {
			return ErrArchiveTooLarge
		}
	}
	return nil
}

// ErrArchiveTooLarge says an archive claims more content than a document has.
var ErrArchiveTooLarge = archiveError("문서 안의 내용이 너무 많습니다. 파일이 손상되었거나 압축 폭탄일 수 있습니다")

type archiveError string

func (e archiveError) Error() string { return string(e) }
