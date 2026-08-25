import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Editor } from "@tiptap/react";
import {
  Box,
  IconButton,
  InputBase,
  Paper,
  Stack,
  ToggleButton,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  ChangeCircleOutlined,
  CloseOutlined,
  KeyboardArrowDown,
  KeyboardArrowUp,
  SearchOutlined,
} from "@mui/icons-material";
import { searchState } from "../extensions/searchHighlight";
import { nextMatchIndex, step, type FindOptions } from "./findMatches";

/**
 * FindReplaceBar is the editor's own find, because the browser's cannot see
 * into a document that scrolls inside a panel and cannot replace anything.
 *
 * It floats over the page rather than pushing it down, so the line being
 * looked at does not jump the moment the bar opens.
 */
export function FindReplaceBar({
  editor,
  open,
  withReplace,
  canEdit,
  onClose,
}: {
  editor: Editor;
  open: boolean;
  withReplace: boolean;
  canEdit: boolean;
  onClose: () => void;
}) {
  const [query, setQuery] = useState("");
  const [replacement, setReplacement] = useState("");
  const [options, setOptions] = useState<FindOptions>({});
  const [showReplace, setShowReplace] = useState(withReplace);
  const [tick, setTick] = useState(0);
  const queryRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) setShowReplace(withReplace);
  }, [open, withReplace]);

  // The decorations live in the editor, so the count has to be read back out
  // of it after every change rather than kept here.
  useEffect(() => {
    if (!open) return;
    const onTransaction = () => setTick((value) => value + 1);
    editor.on("transaction", onTransaction);
    return () => {
      editor.off("transaction", onTransaction);
    };
  }, [editor, open]);

  const state = useMemo(
    () => searchState(editor.state),
    // tick is what makes this recompute; the editor state object is mutable.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [editor, tick],
  );

  const search = useCallback(
    (value: string, next: FindOptions) => {
      editor.commands.setSearch(value, next);
      const matches = searchState(editor.state).matches;
      if (matches.length > 0) {
        const index = nextMatchIndex(
          matches.map((match) => ({ start: match.from, end: match.to })),
          editor.state.selection.from,
        );
        editor.commands.setActiveMatch(index);
        reveal(editor, index);
      }
      setTick((current) => current + 1);
    },
    [editor],
  );

  useEffect(() => {
    if (!open) {
      editor.commands.clearSearch();
      return;
    }
    queryRef.current?.focus();
    queryRef.current?.select();
  }, [editor, open]);

  const move = (direction: 1 | -1) => {
    const total = state.matches.length;
    if (total === 0) return;
    const index = step(state.active, total, direction);
    editor.commands.setActiveMatch(index);
    reveal(editor, index);
    setTick((current) => current + 1);
  };

  const replaceOne = () => {
    const match = state.matches[state.active];
    if (!match || !canEdit) return;
    editor
      .chain()
      .focus()
      .insertContentAt({ from: match.from, to: match.to }, replacement)
      .run();
    // The document changed, so the plugin has already found the matches
    // again; land on the one that took this one's place.
    const matches = searchState(editor.state).matches;
    if (matches.length > 0) {
      const index = Math.min(state.active, matches.length - 1);
      editor.commands.setActiveMatch(index);
      reveal(editor, index);
    }
    setTick((current) => current + 1);
  };

  const replaceAll = () => {
    if (!canEdit || state.matches.length === 0) return;
    // Back to front, so replacing one does not move the ones still to do.
    const chain = editor.chain().focus();
    for (const match of [...state.matches].reverse())
      chain.insertContentAt({ from: match.from, to: match.to }, replacement);
    chain.run();
    setTick((current) => current + 1);
  };

  if (!open) return null;

  const total = state.matches.length;
  return (
    <Paper
      elevation={6}
      sx={{
        position: "absolute",
        top: 12,
        right: 24,
        zIndex: 20,
        p: 1,
        borderRadius: 2,
        border: "1px solid",
        borderColor: "divider",
      }}
    >
      <Stack gap={0.75}>
        <Stack direction="row" alignItems="center" gap={0.5}>
          <SearchOutlined fontSize="small" color="disabled" />
          <InputBase
            inputRef={queryRef}
            placeholder="문서에서 찾기"
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              search(event.target.value, options);
            }}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                move(event.shiftKey ? -1 : 1);
              }
              if (event.key === "Escape") onClose();
            }}
            sx={{ width: 200, fontSize: 14 }}
          />
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ minWidth: 54, textAlign: "right" }}
          >
            {query ? (total === 0 ? "없음" : `${state.active + 1}/${total}`) : ""}
          </Typography>
          <Tooltip title="이전 (Shift+Enter)">
            <span>
              <IconButton size="small" disabled={total === 0} onClick={() => move(-1)}>
                <KeyboardArrowUp fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
          <Tooltip title="다음 (Enter)">
            <span>
              <IconButton size="small" disabled={total === 0} onClick={() => move(1)}>
                <KeyboardArrowDown fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
          <Tooltip title="대문자 구분">
            <ToggleButton
              size="small"
              value="case"
              selected={Boolean(options.caseSensitive)}
              onClick={() => {
                const next = { ...options, caseSensitive: !options.caseSensitive };
                setOptions(next);
                search(query, next);
              }}
              sx={{ px: 1, py: 0.2, fontSize: 12, lineHeight: 1.4 }}
            >
              Aa
            </ToggleButton>
          </Tooltip>
          <Tooltip title="단어 단위">
            <ToggleButton
              size="small"
              value="word"
              selected={Boolean(options.wholeWord)}
              onClick={() => {
                const next = { ...options, wholeWord: !options.wholeWord };
                setOptions(next);
                search(query, next);
              }}
              sx={{ px: 1, py: 0.2, fontSize: 12, lineHeight: 1.4 }}
            >
              단어
            </ToggleButton>
          </Tooltip>
          {canEdit && (
            <Tooltip title="바꾸기">
              <IconButton
                size="small"
                onClick={() => setShowReplace((value) => !value)}
              >
                <ChangeCircleOutlined fontSize="small" />
              </IconButton>
            </Tooltip>
          )}
          <Tooltip title="닫기 (Esc)">
            <IconButton size="small" onClick={onClose}>
              <CloseOutlined fontSize="small" />
            </IconButton>
          </Tooltip>
        </Stack>

        {showReplace && canEdit && (
          <Stack direction="row" alignItems="center" gap={0.5}>
            <Box sx={{ width: 20 }} />
            <InputBase
              placeholder="바꿀 내용"
              value={replacement}
              onChange={(event) => setReplacement(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  event.preventDefault();
                  replaceOne();
                }
                if (event.key === "Escape") onClose();
              }}
              sx={{ width: 200, fontSize: 14 }}
            />
            <Box
              component="button"
              type="button"
              disabled={total === 0}
              onClick={replaceOne}
              sx={buttonStyle}
            >
              바꾸기
            </Box>
            <Box
              component="button"
              type="button"
              disabled={total === 0}
              onClick={replaceAll}
              sx={buttonStyle}
            >
              모두 바꾸기
            </Box>
          </Stack>
        )}
      </Stack>
    </Paper>
  );
}

const buttonStyle = {
  border: "1px solid",
  borderColor: "divider",
  bgcolor: "transparent",
  borderRadius: 1,
  px: 1.2,
  py: 0.5,
  fontSize: 13,
  cursor: "pointer",
  fontFamily: "inherit",
  "&:disabled": { opacity: 0.45, cursor: "default" },
  "&:hover:not(:disabled)": { bgcolor: "action.hover" },
} as const;

/** reveal puts the caret on a match so the browser scrolls it into view. */
function reveal(editor: Editor, index: number) {
  const match = searchState(editor.state).matches[index];
  if (!match) return;
  editor
    .chain()
    .setTextSelection({ from: match.from, to: match.to })
    .scrollIntoView()
    .run();
}
