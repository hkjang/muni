import { useCallback, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { Editor } from "@tiptap/react";
import type { JSONContent } from "@tiptap/core";
import { api, errorMessage } from "../../../lib/api";
import { fileKind, type DroppedFiles } from "../extensions/fileDrop";

/** What the server sends back for a document file put into a document. */
export type ImportedFile = {
  content: JSONContent;
  title: string;
  header: string;
  footer: string;
  landscape: boolean;
  format: string;
  images: number;
};

export type ImportProgress =
  | { state: "idle" }
  | { state: "working"; name: string; index: number; total: number }
  | { state: "done"; message: string }
  | { state: "error"; message: string };

/**
 * The document was empty when the file arrived, so the file is not being
 * added to something: it is becoming the document. What the file said about
 * itself — its title, its header and footer, which way its paper turns —
 * is offered for the document to take on.
 */
export type ImportedFurniture = Pick<
  ImportedFile,
  "title" | "header" | "footer" | "landscape"
>;

/** A document with nothing in it but the empty paragraph it starts with. */
function isBlank(editor: Editor): boolean {
  const { doc } = editor.state;
  return doc.childCount <= 1 && doc.textContent.trim() === "";
}

/**
 * blockBoundary moves a position to the end of the top-level block it is
 * in, so content dropped onto a paragraph goes after that paragraph rather
 * than splitting it at the letter the pointer happened to be over.
 */
function blockBoundary(editor: Editor, position: number): number {
  const { doc } = editor.state;
  const clamped = Math.max(0, Math.min(position, doc.content.size));
  const $pos = doc.resolve(clamped);
  return $pos.depth === 0 ? clamped : $pos.after(1);
}

/**
 * useFileImport puts dropped, pasted or picked files into the document.
 *
 * A picture is uploaded and placed as an image. A document file is sent to
 * the server, which reads it the way the import that makes a document does,
 * and its blocks are put in at the drop — one transaction, so one undo takes
 * a whole file back out. Files go in one after another in the order they
 * came, each after the last, and a file that cannot be read is reported
 * without stopping the rest.
 */
export function useFileImport({
  editor,
  documentId,
  canEdit,
  onFurniture,
}: {
  editor: Editor | null;
  documentId: string;
  canEdit: boolean;
  onFurniture?: (furniture: ImportedFurniture) => void;
}) {
  const [progress, setProgress] = useState<ImportProgress>({ state: "idle" });
  const queryClient = useQueryClient();

  const importFiles = useCallback(
    async ({ files, position }: DroppedFiles) => {
      if (!editor) return;
      if (!canEdit) {
        setProgress({
          state: "error",
          message: "편집 모드에서만 파일 내용을 넣을 수 있습니다.",
        });
        return;
      }
      const supported = files.filter((file) => fileKind(file) !== "unsupported");
      const skipped = files.length - supported.length;
      if (supported.length === 0) {
        setProgress({
          state: "error",
          message:
            "넣을 수 있는 파일은 PDF · DOCX · HWP · HWPX · Markdown · TXT · HTML 과 그림입니다.",
        });
        return;
      }
      const blank = isBlank(editor);
      let at = blank ? 0 : blockBoundary(editor, position);
      let furnitureTaken = false;
      let placed = 0;
      const failures: string[] = [];
      for (const [index, file] of supported.entries()) {
        setProgress({
          state: "working",
          name: file.name,
          index: index + 1,
          total: supported.length,
        });
        const form = new FormData();
        form.set("file", file);
        try {
          const before = editor.state.doc.content.size;
          if (fileKind(file) === "image") {
            const uploaded = await api<{ url: string }>(
              `/api/v1/documents/${documentId}/attachments`,
              { method: "POST", body: form },
            );
            editor
              .chain()
              .insertContentAt(at, {
                type: "image",
                attrs: { src: uploaded.url, alt: file.name },
              })
              .run();
          } else {
            const result = await api<ImportedFile>(
              `/api/v1/documents/${documentId}/import`,
              { method: "POST", body: form },
            );
            const blocks = result.content.content ?? [];
            if (blocks.length > 0) {
              if (blank && placed === 0) {
                // The file replaces the empty paragraph rather than
                // following it.
                editor
                  .chain()
                  .insertContentAt(
                    { from: 0, to: editor.state.doc.content.size },
                    blocks,
                  )
                  .run();
              } else {
                editor.chain().insertContentAt(at, blocks).run();
              }
            }
            if (blank && !furnitureTaken && onFurniture) {
              onFurniture(result);
              furnitureTaken = true;
            }
            if (result.images > 0)
              void queryClient.invalidateQueries({
                queryKey: ["attachments", documentId],
              });
          }
          // The next file goes after what this one put in.
          at = blank && placed === 0
            ? editor.state.doc.content.size
            : at + (editor.state.doc.content.size - before);
          placed++;
        } catch (error) {
          failures.push(`${file.name}: ${errorMessage(error)}`);
        }
      }
      void queryClient.invalidateQueries({
        queryKey: ["attachments", documentId],
      });
      const notes: string[] = [];
      if (placed > 0)
        notes.push(
          placed === 1 && supported.length === 1
            ? `${supported[0]?.name ?? "파일"}의 내용을 넣었습니다.`
            : `파일 ${placed}개의 내용을 넣었습니다.`,
        );
      if (skipped > 0) notes.push(`지원하지 않는 파일 ${skipped}개는 건너뛰었습니다.`);
      if (failures.length > 0) {
        setProgress({
          state: "error",
          message: [...notes, ...failures].join(" "),
        });
      } else {
        setProgress({ state: "done", message: notes.join(" ") });
      }
    },
    [canEdit, documentId, editor, onFurniture, queryClient],
  );

  const dismiss = useCallback(() => setProgress({ state: "idle" }), []);
  return { progress, importFiles, dismiss };
}
