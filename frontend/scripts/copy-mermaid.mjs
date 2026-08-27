// mermaid is bundled twice over, on purpose.
//
// The editor imports it as a module, so vite code-splits it and a document
// without a diagram never loads it. An exported HTML file and the page the PDF
// is printed from cannot import a module chunk — they need one self-contained
// script — so the browser build of mermaid is copied verbatim into the assets
// the server already embeds, and the server hands it to whichever export needs
// drawing.
//
// Copied at build time rather than committed: it is three and a half megabytes
// of somebody else's minified output, and the repository should not carry it.
import { copyFileSync, mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const from = resolve(
  here,
  "..",
  "node_modules",
  "mermaid",
  "dist",
  "mermaid.min.js",
);
const to = resolve(here, "..", "public", "mermaid.min.js");

mkdirSync(dirname(to), { recursive: true });
copyFileSync(from, to);
console.log("copied mermaid.min.js into public/");
