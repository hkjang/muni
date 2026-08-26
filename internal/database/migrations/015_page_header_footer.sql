-- 머리글과 바닥글.
--
-- A Korean office document carries its classification on the page rather than
-- in the text — 대외비 in the header, 부서명 and 문서번호 beside it. Importing
-- such a document dropped those parts of the file entirely and said nothing,
-- so a confidential document arrived in muni with the word "대외비" gone.
--
-- One line each, plain text. Word can hold three headers per section (first
-- page, even, odd) and format them freely; muni holds the one that appears on
-- every page, which is the one carrying the marking.
ALTER TABLE documents ADD COLUMN IF NOT EXISTS page_header text NOT NULL DEFAULT '';
ALTER TABLE documents ADD COLUMN IF NOT EXISTS page_footer text NOT NULL DEFAULT '';
