export type AIAction = {
  id: string;
  label: string;
  /** Instruction sent with the selected text. */
  instruction: string;
};

/**
 * The model must answer with the replacement text only — the result is written
 * straight back into the document, so any preamble would end up in the page.
 */
const outputRule =
  "설명, 인사말, 따옴표, 코드 블록 표시 없이 결과 텍스트만 출력하세요. 원문의 언어를 유지하세요.";

export const selectionActions: AIAction[] = [
  {
    id: "polish",
    label: "다듬기",
    instruction: "다음 글을 뜻을 바꾸지 말고 자연스럽고 명확하게 다듬어 주세요.",
  },
  {
    id: "shorten",
    label: "짧게",
    instruction: "다음 글을 핵심만 남겨 더 짧게 줄여 주세요.",
  },
  {
    id: "expand",
    label: "자세히",
    instruction: "다음 글을 같은 논지로 더 구체적이고 자세하게 확장해 주세요.",
  },
  {
    id: "proofread",
    label: "맞춤법",
    instruction:
      "다음 글의 맞춤법, 띄어쓰기, 문법 오류만 고쳐 주세요. 표현이나 어투는 그대로 두세요.",
  },
  {
    id: "formal",
    label: "격식체",
    instruction: "다음 글을 공식 문서에 어울리는 격식 있는 어투로 바꿔 주세요.",
  },
  {
    id: "summarize",
    label: "요약",
    instruction: "다음 글을 핵심 내용 위주로 요약해 주세요.",
  },
  {
    id: "bullets",
    label: "목록으로",
    instruction:
      "다음 글을 읽기 쉬운 불릿 목록으로 바꿔 주세요. 각 줄은 '- '로 시작하세요.",
  },
  {
    id: "translate-en",
    label: "영어로",
    instruction: "다음 글을 자연스러운 영어로 번역해 주세요.",
  },
];

export function buildPrompt(instruction: string, selection: string): string {
  return `${instruction}\n${outputRule}\n\n---\n${selection}\n---`;
}
