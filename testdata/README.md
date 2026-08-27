# testdata

`every-node.json` is one document carrying every kind of block and inline the
editor can make. Three test suites read it, and it is one file so they cannot
disagree about what "every kind" means:

- `internal/httpapi/export_coverage_test.go` — HTML, Markdown and plain text
- `internal/docx/export_coverage_test.go` — .docx, out and back again
- `frontend/src/serverDocument.test.ts` — the editor itself, which has to be
  able to open what the server sends

The Go tests check that each distinctive Korean phrase survives the format.
The editor test checks something the Go tests cannot see: that the *shape* is
one the ProseMirror schema can hold. A paragraph with an image inside it
carried every phrase through every export and still would not open.

Adding a node kind here is how a new feature earns its place in all four.
