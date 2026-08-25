import { Fragment, type ReactNode } from "react";
import { Box, Link, Typography } from "@mui/material";
import type { MDBlock, MDInline } from "../lib/markdown";
import { parseMarkdown } from "../lib/markdown";

/**
 * Markdown renders a model's answer the way it was written.
 *
 * Answers come back as Markdown whether or not the model was asked for it, and
 * shown as preformatted text they read as a wall of `###` and `**`. Nothing is
 * ever parsed as HTML — the tree here is built from the source text, so there
 * is no path from a model's output to markup in the page.
 */
export function Markdown({ text }: { text: string }) {
  const blocks = parseMarkdown(text);
  return (
    <Box
      sx={{
        fontSize: 15,
        lineHeight: 1.7,
        wordBreak: "break-word",
        "& > *:first-of-type": { mt: 0 },
        "& > *:last-child": { mb: 0 },
      }}
    >
      {blocks.map((block, index) => (
        <Fragment key={index}>{renderBlock(block)}</Fragment>
      ))}
    </Box>
  );
}

function renderBlock(block: MDBlock): ReactNode {
  switch (block.type) {
    case "heading":
      return (
        <Typography
          component={`h${Math.min(6, block.level + 1)}` as "h2"}
          sx={{
            fontWeight: 700,
            fontSize: [19, 17, 16, 15, 15, 15][block.level - 1] ?? 15,
            mt: 2,
            mb: 0.75,
          }}
        >
          {renderInline(block.inline)}
        </Typography>
      );
    case "paragraph":
      return (
        <Typography sx={{ my: 1 }}>{renderInline(block.inline)}</Typography>
      );
    case "rule":
      return (
        <Box sx={{ my: 2, borderTop: "1px solid", borderColor: "divider" }} />
      );
    case "code":
      return (
        <Box
          component="pre"
          sx={{
            my: 1.25,
            p: 1.25,
            bgcolor: "#1f2130",
            color: "#eef0ff",
            borderRadius: 1,
            overflowX: "auto",
            fontSize: 13,
            lineHeight: 1.6,
          }}
        >
          <code>{block.text}</code>
        </Box>
      );
    case "quote":
      return (
        <Box
          sx={{
            my: 1.25,
            pl: 1.5,
            borderLeft: "3px solid",
            borderColor: "divider",
            color: "text.secondary",
          }}
        >
          {block.blocks.map((child, index) => (
            <Fragment key={index}>{renderBlock(child)}</Fragment>
          ))}
        </Box>
      );
    case "list": {
      const tasks = block.items.every((item) => item.checked !== undefined);
      return (
        <Box
          component={block.ordered ? "ol" : "ul"}
          start={block.ordered ? block.start : undefined}
          sx={{
            my: 1,
            pl: tasks ? 0.5 : 3,
            listStyle: tasks ? "none" : undefined,
            "& li": { mb: 0.4 },
          }}
        >
          {block.items.map((item, index) => (
            <li key={index}>
              {item.checked !== undefined && (
                <Box component="span" sx={{ mr: 0.75 }}>
                  {item.checked ? "☑" : "☐"}
                </Box>
              )}
              {item.blocks.map((child, childIndex) => (
                <Fragment key={childIndex}>
                  {child.type === "paragraph" ? (
                    <Box component="span">{renderInline(child.inline)}</Box>
                  ) : (
                    renderBlock(child)
                  )}
                </Fragment>
              ))}
            </li>
          ))}
        </Box>
      );
    }
    case "table":
      return (
        <Box sx={{ my: 1.25, overflowX: "auto" }}>
          <Box
            component="table"
            sx={{
              borderCollapse: "collapse",
              width: "100%",
              fontSize: 14,
              "& th, & td": {
                border: "1px solid",
                borderColor: "divider",
                px: 1,
                py: 0.6,
                textAlign: "left",
                verticalAlign: "top",
              },
              "& th": { bgcolor: "action.hover", fontWeight: 700 },
            }}
          >
            <thead>
              <tr>
                {block.header.map((cell, index) => (
                  <th key={index}>{renderInline(cell)}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {block.rows.map((row, rowIndex) => (
                <tr key={rowIndex}>
                  {row.map((cell, index) => (
                    <td key={index}>{renderInline(cell)}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </Box>
        </Box>
      );
    default:
      return null;
  }
}

function renderInline(pieces: MDInline[]): ReactNode {
  return pieces.map((piece, index) => {
    // A newline inside a paragraph was a soft break in the source.
    const lines = piece.text.split("\n");
    const body: ReactNode = lines.map((line, lineIndex) => (
      <Fragment key={lineIndex}>
        {lineIndex > 0 && <br />}
        {line}
      </Fragment>
    ));
    return <Fragment key={index}>{decorate(body, piece)}</Fragment>;
  });
}

function decorate(body: ReactNode, piece: MDInline): ReactNode {
  let node = body;
  for (const mark of piece.marks) {
    if (mark.type === "code")
      node = (
        <Box
          component="code"
          sx={{
            px: 0.5,
            py: 0.15,
            bgcolor: "action.hover",
            borderRadius: 0.5,
            fontSize: "0.9em",
          }}
        >
          {node}
        </Box>
      );
    else if (mark.type === "bold")
      node = (
        <Box component="strong" sx={{ fontWeight: 700 }}>
          {node}
        </Box>
      );
    else if (mark.type === "italic") node = <em>{node}</em>;
    else if (mark.type === "strike") node = <s>{node}</s>;
    else if (mark.type === "link")
      node = (
        <Link
          href={safeHref(mark.href)}
          target="_blank"
          rel="noopener noreferrer"
        >
          {node}
        </Link>
      );
  }
  return node;
}

/** Only ordinary web links are followable; anything else renders as plain text. */
function safeHref(href: string): string | undefined {
  const trimmed = href.trim();
  if (/^(https?:|mailto:|\/|#)/i.test(trimmed)) return trimmed;
  return undefined;
}
