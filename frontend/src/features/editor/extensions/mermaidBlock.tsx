import { useEffect, useRef, useState } from "react";
import CodeBlock from "@tiptap/extension-code-block";
import {
  ReactNodeViewRenderer,
  NodeViewContent,
  NodeViewWrapper,
} from "@tiptap/react";
import type { NodeViewProps } from "@tiptap/react";

/** The language a code block names to be drawn rather than printed. */
export const mermaidLanguage = "mermaid";

let loading: Promise<typeof import("mermaid").default> | null = null;

/**
 * mermaidLibrary loads the drawing library the first time a diagram is drawn.
 *
 * It is brought in on demand rather than with the rest of the editor: it is
 * the largest thing in the bundle by some way, and a document without a
 * diagram in it should not pay for one. muni ships offline, so it is bundled
 * rather than fetched — the import is a code split, not a network call.
 */
async function mermaidLibrary() {
  if (!loading) {
    loading = import("mermaid").then((module) => {
      module.default.initialize({
        startOnLoad: false,
        // A diagram that cannot be drawn is reported to us, not painted over
        // the document as a red error card.
        suppressErrorRendering: true,
        securityLevel: "strict",
        fontFamily: "'Pretendard','Noto Sans KR','Malgun Gothic',sans-serif",
      });
      return module.default;
    });
  }
  return loading;
}

let drawCount = 0;

/**
 * MermaidView draws the diagram under the text that describes it.
 *
 * Both, rather than one or the other: the text stays editable exactly as any
 * other code block, and what it draws appears as it is typed. Swapping the
 * text out for the picture would mean the only way to fix a diagram is to
 * make the editor give the text back.
 */
function MermaidView({ node }: NodeViewProps) {
  const source = node.textContent;
  const language = (node.attrs.language as string | null) ?? "";
  const isDiagram = language.toLowerCase() === mermaidLanguage;
  const [svg, setSvg] = useState("");
  const [problem, setProblem] = useState("");
  const holder = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!isDiagram || !source.trim()) {
      setSvg("");
      setProblem("");
      return;
    }
    let current = true;
    // A diagram is redrawn as it is typed, and half a diagram does not parse.
    // Waiting a moment keeps every keystroke from reporting a problem.
    const timer = setTimeout(() => {
      void (async () => {
        try {
          const mermaid = await mermaidLibrary();
          drawCount += 1;
          const { svg: drawn } = await mermaid.render(
            `muni-mermaid-${drawCount}`,
            source,
          );
          if (!current) return;
          setSvg(drawn);
          setProblem("");
        } catch (error) {
          if (!current) return;
          setSvg("");
          setProblem(error instanceof Error ? error.message : String(error));
        }
      })();
    }, 300);
    return () => {
      current = false;
      clearTimeout(timer);
    };
  }, [isDiagram, source]);

  return (
    <NodeViewWrapper className={isDiagram ? "muni-mermaid" : undefined}>
      <pre>
        <NodeViewContent />
      </pre>
      {isDiagram && svg ? (
        <div
          ref={holder}
          className="muni-mermaid-drawing"
          contentEditable={false}
          // The library returns SVG it built from the text above; nothing a
          // reader supplied reaches this except through mermaid's own parser,
          // which is run at its strict security level.
          dangerouslySetInnerHTML={{ __html: svg }}
        />
      ) : null}
      {isDiagram && problem ? (
        <div className="muni-mermaid-problem" contentEditable={false}>
          그림으로 그리지 못했습니다: {problem}
        </div>
      ) : null}
    </NodeViewWrapper>
  );
}

/**
 * MermaidCodeBlock is the ordinary code block, which draws itself when its
 * language says mermaid.
 *
 * A diagram is a code block rather than a node of its own on purpose: every
 * path muni already has — the schema, the .docx export, the Markdown fence,
 * the plain-text export — carries a code block and its language already, so a
 * diagram survives a round trip through all of them as the text it is.
 */
export const MermaidCodeBlock = CodeBlock.extend({
  addNodeView() {
    return ReactNodeViewRenderer(MermaidView);
  },
});
