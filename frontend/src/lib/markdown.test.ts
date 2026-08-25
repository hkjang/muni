import { describe, expect, it } from "vitest";
import { looksLikeMarkdown, parseInline, parseMarkdown } from "./markdown";
import { markdownToContent } from "./markdownContent";
import { contentToMarkdown } from "./contentMarkdown";
import type { EditorContent } from "./markdownContent";

describe("parseInline", () => {
  it("reads bold, italic, strike and inline code", () => {
    expect(parseInline("**굵게** *기울임* ~~취소~~ `코드`")).toEqual([
      { text: "굵게", marks: [{ type: "bold" }] },
      { text: " ", marks: [] },
      { text: "기울임", marks: [{ type: "italic" }] },
      { text: " ", marks: [] },
      { text: "취소", marks: [{ type: "strike" }] },
      { text: " ", marks: [] },
      { text: "코드", marks: [{ type: "code" }] },
    ]);
  });

  it("does not read markup inside a code span", () => {
    expect(parseInline("`**굵지 않음**`")).toEqual([
      { text: "**굵지 않음**", marks: [{ type: "code" }] },
    ]);
  });

  it("keeps a lone asterisk as text", () => {
    expect(parseInline("2 * 3 = 6")).toEqual([
      { text: "2 * 3 = 6", marks: [] },
    ]);
  });

  it("reads a link and keeps its label formatting", () => {
    expect(parseInline("[**보고서**](https://example.com/a)")).toEqual([
      {
        text: "보고서",
        marks: [
          { type: "link", href: "https://example.com/a" },
          { type: "bold" },
        ],
      },
    ]);
  });

  it("treats an escaped marker as a character", () => {
    expect(parseInline("\\*강조 아님\\*")).toEqual([
      { text: "*강조 아님*", marks: [] },
    ]);
  });
});

describe("parseMarkdown", () => {
  it("reads headings, paragraphs and rules", () => {
    const blocks = parseMarkdown("# 제목\n\n본문입니다.\n\n---\n");
    expect(blocks.map((block) => block.type)).toEqual([
      "heading",
      "paragraph",
      "rule",
    ]);
  });

  it("reads a nested list", () => {
    const blocks = parseMarkdown("- 상위\n  - 하위\n- 다음");
    expect(blocks).toHaveLength(1);
    const list = blocks[0];
    if (list?.type !== "list") throw new Error("expected a list");
    expect(list.items).toHaveLength(2);
    expect(list.items[0]?.blocks.map((block) => block.type)).toEqual([
      "paragraph",
      "list",
    ]);
  });

  it("reads a task list", () => {
    const blocks = parseMarkdown("- [x] 완료\n- [ ] 예정");
    const list = blocks[0];
    if (list?.type !== "list") throw new Error("expected a list");
    expect(list.items.map((item) => item.checked)).toEqual([true, false]);
  });

  it("reads a fenced code block without interpreting it", () => {
    const blocks = parseMarkdown('```go\nfmt.Println("**hi**")\n```');
    expect(blocks[0]).toEqual({
      type: "code",
      language: "go",
      text: 'fmt.Println("**hi**")',
    });
  });

  it("reads a table", () => {
    const blocks = parseMarkdown(
      "| 항목 | 값 |\n| --- | --- |\n| 비용 | 3억 |",
    );
    const table = blocks[0];
    if (table?.type !== "table") throw new Error("expected a table");
    expect(table.header[0]?.[0]?.text).toBe("항목");
    expect(table.rows[0]?.[1]?.[0]?.text).toBe("3억");
  });

  it("does not mistake a table-like line without a divider for a table", () => {
    const blocks = parseMarkdown("| 그냥 | 문장 |");
    expect(blocks[0]?.type).toBe("paragraph");
  });

  it("keeps a quote together", () => {
    const blocks = parseMarkdown("> 인용 첫 줄\n> 둘째 줄");
    const quote = blocks[0];
    if (quote?.type !== "quote") throw new Error("expected a quote");
    expect(quote.blocks).toHaveLength(1);
  });
});

describe("looksLikeMarkdown", () => {
  it("recognises formatting worth keeping", () => {
    expect(looksLikeMarkdown("**중요**")).toBe(true);
    expect(looksLikeMarkdown("- 항목")).toBe(true);
    expect(looksLikeMarkdown("## 제목")).toBe(true);
  });

  it("leaves ordinary prose alone", () => {
    expect(looksLikeMarkdown("그냥 한 문장입니다.")).toBe(false);
  });
});

describe("markdownToContent", () => {
  it("keeps a plain one-liner as a string so it stays inside the sentence", () => {
    expect(markdownToContent("다듬어진 문장입니다.")).toBe(
      "다듬어진 문장입니다.",
    );
  });

  it("returns inline nodes when a one-liner carries formatting", () => {
    expect(markdownToContent("**중요한** 문장")).toEqual([
      { type: "text", text: "중요한", marks: [{ type: "bold" }] },
      { type: "text", text: " 문장" },
    ]);
  });

  it("builds a bullet list", () => {
    expect(markdownToContent("- 첫째\n- 둘째")).toEqual([
      {
        type: "bulletList",
        content: [
          {
            type: "listItem",
            content: [
              { type: "paragraph", content: [{ type: "text", text: "첫째" }] },
            ],
          },
          {
            type: "listItem",
            content: [
              { type: "paragraph", content: [{ type: "text", text: "둘째" }] },
            ],
          },
        ],
      },
    ]);
  });

  it("builds a heading with the right level", () => {
    expect(markdownToContent("### 소제목")).toEqual([
      {
        type: "heading",
        attrs: { level: 3 },
        content: [{ type: "text", text: "소제목" }],
      },
    ]);
  });

  it("keeps a link as a mark rather than as characters", () => {
    const content = markdownToContent("[문서](https://example.com)", {
      forceBlocks: true,
    });
    expect(content).toEqual([
      {
        type: "paragraph",
        content: [
          {
            type: "text",
            text: "문서",
            marks: [
              {
                type: "link",
                attrs: { href: "https://example.com", target: "_blank" },
              },
            ],
          },
        ],
      },
    ]);
  });

  it("returns an empty string for a blank answer", () => {
    expect(markdownToContent("   \n\n ")).toBe("");
  });
});

describe("contentToMarkdown", () => {
  const nodes: EditorContent[] = [
    {
      type: "heading",
      attrs: { level: 2 },
      content: [{ type: "text", text: "추진 계획" }],
    },
    {
      type: "paragraph",
      content: [
        { type: "text", text: "예산은 " },
        { type: "text", text: "3억원", marks: [{ type: "bold" }] },
        { type: "text", text: "이며 " },
        {
          type: "text",
          text: "상세",
          marks: [{ type: "link", attrs: { href: "https://example.com" } }],
        },
        { type: "text", text: "를 보세요." },
      ],
    },
    {
      type: "bulletList",
      content: [
        {
          type: "listItem",
          content: [
            { type: "paragraph", content: [{ type: "text", text: "첫째" }] },
          ],
        },
      ],
    },
  ];

  it("writes formatting out as markers the model can see", () => {
    expect(contentToMarkdown(nodes)).toBe(
      "## 추진 계획\n\n예산은 **3억원**이며 [상세](https://example.com)를 보세요.\n\n- 첫째",
    );
  });

  it("survives a round trip", () => {
    const markdown = contentToMarkdown(nodes);
    const back = markdownToContent(markdown, { forceBlocks: true });
    expect(contentToMarkdown(back as EditorContent[])).toBe(markdown);
  });

  it("writes a table as a markdown table", () => {
    const table: EditorContent[] = [
      {
        type: "table",
        content: [
          {
            type: "tableRow",
            content: [
              {
                type: "tableHeader",
                content: [
                  {
                    type: "paragraph",
                    content: [{ type: "text", text: "항목" }],
                  },
                ],
              },
              {
                type: "tableHeader",
                content: [
                  {
                    type: "paragraph",
                    content: [{ type: "text", text: "값" }],
                  },
                ],
              },
            ],
          },
          {
            type: "tableRow",
            content: [
              {
                type: "tableCell",
                content: [
                  {
                    type: "paragraph",
                    content: [{ type: "text", text: "비용" }],
                  },
                ],
              },
              {
                type: "tableCell",
                content: [
                  {
                    type: "paragraph",
                    content: [{ type: "text", text: "3억" }],
                  },
                ],
              },
            ],
          },
        ],
      },
    ];
    expect(contentToMarkdown(table)).toBe(
      "| 항목 | 값 |\n| --- | --- |\n| 비용 | 3억 |",
    );
  });

  it("keeps a hard break inside a paragraph", () => {
    expect(
      contentToMarkdown([
        {
          type: "paragraph",
          content: [
            { type: "text", text: "한 줄" },
            { type: "hardBreak" },
            { type: "text", text: "다음 줄" },
          ],
        },
      ]),
    ).toBe("한 줄\n다음 줄");
  });
});
