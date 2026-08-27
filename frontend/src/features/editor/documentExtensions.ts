import type { Extensions } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import Highlight from "@tiptap/extension-highlight";
import { TableKit } from "@tiptap/extension-table";
import TaskList from "@tiptap/extension-task-list";
import TaskItem from "@tiptap/extension-task-item";
import TextAlign from "@tiptap/extension-text-align";
import { TextStyleKit } from "@tiptap/extension-text-style";
import Superscript from "@tiptap/extension-superscript";
import Subscript from "@tiptap/extension-subscript";
import { SizedImage } from "./extensions/imageAttributes";
import { BlockId } from "./extensions/blockId";
import { CellBackground } from "./extensions/cellBackground";
import { CellVerticalAlign } from "./extensions/cellAlign";
import { LineHeight } from "./extensions/lineHeight";
import { PageBreak } from "./extensions/pageBreak";
import { ParagraphIndent } from "./extensions/paragraphIndent";
import { HeadingNumbers } from "./extensions/headingNumbers";
import { TableOfContentsNode } from "./extensions/tableOfContents";
import { Footnote } from "./extensions/footnote";
import { MermaidCodeBlock } from "./extensions/mermaidBlock";

/**
 * Everything that decides what a muni document *is*.
 *
 * This list used to be written out twice — once in the editor, once in the
 * public share view — and the two drifted: the share view never learned about
 * the contents-list node, so a report shared by link arrived without its 목차.
 *
 * The list matters more than it looks. ProseMirror does not ignore what its
 * schema does not recognise: an unknown mark takes the whole paragraph it sits
 * in, text and all, and an unknown node takes itself and its contents. A
 * screen missing one entry does not render a document slightly wrong — it
 * renders a different document.
 *
 * Anything that needs a live connection or belongs only to editing —
 * collaboration, the placeholder, paste handling, search highlighting — is
 * added by the screen that needs it. Those do not change the schema.
 */
export function documentExtensions(): Extensions {
  return [
    // The code block comes from the mermaid extension instead: it is the
    // same node, drawn as a diagram when its language says so.
    StarterKit.configure({ undoRedo: false, codeBlock: false }),
    MermaidCodeBlock,
    Highlight.configure({ multicolor: true }),
    SizedImage.configure({ allowBase64: true, inline: false }),
    TableKit.configure({ table: { resizable: true } }),
    TaskList,
    TaskItem.configure({ nested: true }),
    TextAlign.configure({ types: ["heading", "paragraph"] }),
    TextStyleKit,
    // muni reads these out of Word files and writes them back; without them
    // in the schema every paragraph containing one is discarded on load.
    Superscript,
    Subscript,
    BlockId,
    CellBackground,
    CellVerticalAlign,
    LineHeight,
    PageBreak,
    ParagraphIndent,
    HeadingNumbers,
    TableOfContentsNode,
    Footnote,
  ];
}
