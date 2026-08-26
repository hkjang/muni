import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Box, Paper, Popper, Stack, TextField, Typography } from "@mui/material";
import { api } from "../../../lib/api";
import type { User } from "../../../types";
import { readMention, applyMention } from "./mentions";

/**
 * MentionInput is the comment box, with a list of people that opens when an
 * "@" is typed.
 *
 * Mentions have always turned into a notification, and the box said so —
 * "@아이디로 멘션할 수 있습니다" — which asks the writer to have memorised a
 * username. Nobody has.
 */
export function MentionInput({
  value,
  onChange,
  placeholder,
  minRows = 2,
  autoFocus,
  onEscape,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  minRows?: number;
  autoFocus?: boolean;
  onEscape?: () => void;
}) {
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const [caret, setCaret] = useState(0);
  const [active, setActive] = useState(0);

  const mention = useMemo(() => readMention(value, caret), [value, caret]);
  const people = useQuery({
    queryKey: ["users-search", mention?.query ?? ""],
    queryFn: () =>
      api<User[]>(
        `/api/v1/users/search?q=${encodeURIComponent(mention?.query ?? "")}&limit=8`,
      ),
    enabled: mention !== null,
  });
  const matches = people.data ?? [];

  useEffect(() => setActive(0), [mention?.query]);

  const choose = (person: User) => {
    if (!mention) return;
    const next = applyMention(value, mention, person.username);
    onChange(next.value);
    // The caret goes after the name that was just inserted, not back to where
    // the "@" was.
    requestAnimationFrame(() => {
      const field = inputRef.current;
      if (!field) return;
      field.focus();
      field.setSelectionRange(next.caret, next.caret);
      setCaret(next.caret);
    });
  };

  const open = mention !== null && matches.length > 0;

  return (
    <>
      <TextField
        inputRef={inputRef}
        multiline
        minRows={minRows}
        autoFocus={autoFocus}
        value={value}
        placeholder={placeholder}
        onChange={(event) => {
          onChange(event.target.value);
          setCaret(event.target.selectionStart ?? event.target.value.length);
        }}
        onSelect={(event) =>
          setCaret((event.target as HTMLTextAreaElement).selectionStart ?? 0)
        }
        onKeyDown={(event) => {
          if (open) {
            if (event.key === "ArrowDown") {
              event.preventDefault();
              setActive((current) => (current + 1) % matches.length);
              return;
            }
            if (event.key === "ArrowUp") {
              event.preventDefault();
              setActive((current) => (current - 1 + matches.length) % matches.length);
              return;
            }
            if (event.key === "Enter" || event.key === "Tab") {
              const person = matches[active];
              if (person) {
                event.preventDefault();
                choose(person);
                return;
              }
            }
          }
          if (event.key === "Escape") onEscape?.();
        }}
      />
      <Popper
        open={open}
        anchorEl={inputRef.current}
        placement="bottom-start"
        sx={{ zIndex: 1400 }}
      >
        <Paper
          elevation={6}
          sx={{
            mt: 0.5,
            minWidth: 240,
            maxHeight: 260,
            overflowY: "auto",
            border: "1px solid",
            borderColor: "divider",
          }}
        >
          {matches.map((person, index) => (
            <Stack
              key={person.id}
              direction="row"
              alignItems="center"
              gap={1}
              onMouseDown={(event) => {
                event.preventDefault();
                choose(person);
              }}
              onMouseEnter={() => setActive(index)}
              sx={{
                px: 1.5,
                py: 0.85,
                cursor: "pointer",
                bgcolor: index === active ? "action.selected" : "transparent",
              }}
            >
              <Box sx={{ minWidth: 0 }}>
                <Typography variant="body2" noWrap>
                  {person.displayName}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  @{person.username}
                </Typography>
              </Box>
            </Stack>
          ))}
        </Paper>
      </Popper>
    </>
  );
}
