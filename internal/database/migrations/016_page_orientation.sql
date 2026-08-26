-- 용지 방향.
--
-- The page was A4 portrait, written into the code in three places — the .docx
-- section properties, the PDF print settings, and the export stylesheet. A
-- Korean office document is full of wide tables, and printing one sideways is
-- routine in Word and was impossible here.
ALTER TABLE documents ADD COLUMN IF NOT EXISTS page_orientation text NOT NULL DEFAULT 'PORTRAIT';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'documents_page_orientation_check') THEN
        ALTER TABLE documents ADD CONSTRAINT documents_page_orientation_check
            CHECK (page_orientation IN ('PORTRAIT', 'LANDSCAPE'));
    END IF;
END $$;
