/**
 * Heading numbers, worked out from the outline.
 *
 * A Korean report is expected to number its sections, and doing it by hand
 * means renumbering everything below whenever a section is inserted — exactly
 * the kind of work nobody does reliably. The numbers are derived from the
 * headings rather than typed, so inserting a section renumbers the rest by
 * itself. The server does the same when the document is exported.
 */

export type NumberingScheme = "none" | "decimal" | "korean";

export const numberingSchemes: { value: NumberingScheme; label: string; sample: string }[] = [
  { value: "none", label: "번호 없음", sample: "" },
  { value: "decimal", label: "1. 1.1. 1.1.1.", sample: "1.1." },
  { value: "korean", label: "I. 1. 가. 1)", sample: "가." },
];

export function validScheme(value: unknown): NumberingScheme {
  return value === "decimal" || value === "korean" ? value : "none";
}

const koreanOrder = [..."가나다라마바사아자차카타파하"];
const romanOrder = ["I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X"];

/**
 * headingNumbers labels each heading, taking the depth the outline panel shows
 * so that a document written in 제목 2 and 제목 3 numbers from the top rather
 * than starting part way down.
 */
export function headingNumbers(depths: number[], scheme: NumberingScheme): string[] {
  const out = depths.map(() => "");
  if (scheme === "none") return out;

  const counters: number[] = [];
  depths.forEach((rawDepth, index) => {
    const depth = Math.max(0, rawDepth);
    // Coming back out to a shallower level ends the deeper counts, so the next
    // subsection starts at one again.
    while (counters.length > depth + 1) counters.pop();
    while (counters.length < depth + 1) counters.push(0);
    counters[depth] = (counters[depth] ?? 0) + 1;

    if (scheme === "decimal") {
      out[index] = counters.join(".") + ".";
      return;
    }
    out[index] = koreanLabel(depth, counters[depth] ?? 1);
  });
  return out;
}

function koreanLabel(depth: number, count: number): string {
  if (depth === 0)
    return (count <= romanOrder.length ? romanOrder[count - 1] : String(count)) + ".";
  if (depth === 1) return `${count}.`;
  if (depth === 2)
    return (count <= koreanOrder.length ? koreanOrder[count - 1] : String(count)) + ".";
  if (depth === 3) return `${count})`;
  return (count <= koreanOrder.length ? koreanOrder[count - 1] : String(count)) + ")";
}
