import { useRef, useState } from "react";
import { TextField } from "@mui/material";
import { confirmsInput } from "../../lib/keyboard";

/**
 * DocumentTitleField is the document's name, at the top of the editor.
 *
 * It keeps what is being typed to itself and hands it over when the field is
 * left. Two things made Korean unusable here before. The field wrote every
 * keystroke into the shared document cache, so each letter — each *part* of a
 * letter, while a syllable is being composed — re-rendered the whole editor,
 * and the input method loses its composition when the value underneath it is
 * replaced mid-syllable. And the content autosave, which fires a second and a
 * half after any edit and puts the server's whole document back into that same
 * cache, would overwrite the title being typed with the one already saved.
 *
 * A draft held here is immune to both: nothing outside re-renders while it is
 * being typed, and an autosave landing in the middle changes the title
 * underneath the draft without disturbing it.
 */
export function DocumentTitleField({
  title,
  canEdit,
  onCommit,
}: {
  title: string;
  canEdit: boolean;
  onCommit: (title: string) => Promise<void>;
}) {
  const [draft, setDraft] = useState<string | null>(null);
  // A composition is open between the first letter of a syllable and the one
  // that finishes it; the value must not be touched from anywhere until then.
  const composing = useRef(false);

  const commit = async () => {
    const value = draft;
    if (value === null) return;
    if (value === title) {
      setDraft(null);
      return;
    }
    try {
      await onCommit(value);
      setDraft(null);
    } catch {
      // The text stays in the field rather than reverting to what the server
      // still has: it is the only copy of what was typed.
    }
  };

  return (
    <TextField
      variant="standard"
      value={draft ?? title}
      onChange={(event) => setDraft(event.target.value)}
      onCompositionStart={() => {
        composing.current = true;
      }}
      onCompositionEnd={(event) => {
        composing.current = false;
        const input = event.target as HTMLInputElement;
        setDraft(input.value);
      }}
      onKeyDown={(event) => {
        if (confirmsInput(event)) {
          event.preventDefault();
          event.currentTarget.blur();
        }
      }}
      onBlur={() => {
        if (composing.current) return;
        void commit();
      }}
      disabled={!canEdit}
      inputProps={{ "aria-label": "문서 제목", maxLength: 240 }}
      InputProps={{
        disableUnderline: true,
        sx: {
          fontWeight: 720,
          fontSize: { xs: 15, sm: 17 },
          minWidth: { xs: 120, sm: 280 },
        },
      }}
      sx={{ flex: { xs: 1, md: "0 1 480px" } }}
    />
  );
}
