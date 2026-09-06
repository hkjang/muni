import { Extension } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";

/** What a drop or a paste of files hands over: the files, and where. */
export type DroppedFiles = { files: File[]; position: number };

export interface FileDropOptions {
  /** Called with the files and the document position they landed on. */
  onFiles: ((drop: DroppedFiles) => void) | null;
}

/** The document formats the server can read into the editor. */
export const documentFileExtensions = [
  ".pdf",
  ".docx",
  ".hwp",
  ".hwpx",
  ".md",
  ".markdown",
  ".txt",
  ".html",
  ".htm",
];

/** The accept list for a file picker that takes what a drop takes. */
export const acceptedDropFiles =
  documentFileExtensions.join(",") + ",image/*";

/** The label people see for what can be dropped. */
export const droppableFilesLabel =
  "PDF · DOCX · HWP · HWPX · Markdown · TXT · HTML · 그림";

/**
 * fileKind says what a file is to the editor: a picture goes in as an image,
 * a document is parsed on the server and its content put in place, and
 * anything else is refused with a message rather than opened by the browser.
 */
export function fileKind(file: File): "image" | "document" | "unsupported" {
  if (file.type.startsWith("image/")) return "image";
  const name = file.name.toLowerCase();
  if (documentFileExtensions.some((extension) => name.endsWith(extension)))
    return "document";
  return "unsupported";
}

function filesOf(transfer: DataTransfer | null | undefined): File[] {
  if (!transfer) return [];
  return Array.from(transfer.files ?? []);
}

/**
 * FileDrop takes the files dropped or pasted into the editor.
 *
 * Without it a dropped file is opened by the browser, which navigates away
 * from the document being edited — the worst thing a drop can do. Here the
 * drop is claimed, the position under the pointer is worked out, and the
 * files are handed to whoever knows how to put them in. Files pasted from a
 * clipboard — a screenshot, a copied file — go the same way.
 */
export const FileDrop = Extension.create<FileDropOptions>({
  name: "muniFileDrop",

  addOptions() {
    return { onFiles: null };
  },

  addProseMirrorPlugins() {
    const extension = this;
    return [
      new Plugin({
        key: new PluginKey("muniFileDrop"),
        props: {
          handleDrop(view, event, _slice, moved) {
            const files = filesOf(event.dataTransfer);
            if (moved || files.length === 0) return false;
            event.preventDefault();
            const at = view.posAtCoords({
              left: event.clientX,
              top: event.clientY,
            });
            extension.options.onFiles?.({
              files,
              position: at ? at.pos : view.state.selection.to,
            });
            return true;
          },
          handlePaste(view, event) {
            const files = filesOf(event.clipboardData);
            if (files.length === 0) return false;
            event.preventDefault();
            extension.options.onFiles?.({
              files,
              position: view.state.selection.to,
            });
            return true;
          },
        },
      }),
    ];
  },
});
