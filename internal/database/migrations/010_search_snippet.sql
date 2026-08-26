-- The search result snippet came from ts_headline, which highlights the words
-- a tsquery matched. Korean text produces no matches there — the tokeniser
-- splits on whitespace and "회의" is not a token of "회의록" — so a query for
-- 회의 came back with the opening lines of the document and no sign of what
-- had actually matched.
--
-- This cuts a window around the first occurrence instead, which is what a
-- reader is looking for when they scan a result list.
CREATE OR REPLACE FUNCTION snippet_around(body text, needle text)
RETURNS text
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT CASE
        WHEN body IS NULL OR btrim(body) = '' THEN ''
        WHEN btrim(coalesce(needle, '')) = '' THEN left(body, 160)
        WHEN position(lower(btrim(needle)) in lower(body)) = 0 THEN left(body, 160)
        ELSE
            -- Room before the match for context, and an ellipsis when the
            -- window does not start at the beginning of the document.
            CASE WHEN position(lower(btrim(needle)) in lower(body)) > 41 THEN '…' ELSE '' END
            || substring(
                body
                from greatest(1, position(lower(btrim(needle)) in lower(body)) - 40)
                for 200)
    END
$$;
