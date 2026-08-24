export type PresentationStatus =
  | "pending"
  | "draft"
  | "queued"
  | "generating"
  | "completed"
  | "failed";

export type PresentationLink = {
  id: string;
  documentId: string;
  documentRevision: number;
  provider: string;
  presentationId: string;
  title: string;
  status: PresentationStatus;
  slideCount: number;
  templateId?: string | null;
  editorUrl?: string;
  createdAt: string;
  updatedAt: string;
  lastSyncedAt?: string | null;
  /** The document has been edited since this deck was built. */
  stale: boolean;
};

export type PresentationOptions = {
  title: string;
  audience: string;
  purpose: string;
  tone: string;
  language: string;
  slideCount: number;
  minutes: number;
  detail: string;
};

export const audiences = [
  { value: "경영진", label: "경영진" },
  { value: "실무진", label: "실무진" },
  { value: "기술 검토자", label: "기술 검토자" },
  { value: "고객", label: "고객" },
];

export const purposes = [
  { value: "의사결정", label: "의사결정" },
  { value: "현황 보고", label: "현황 보고" },
  { value: "제안", label: "제안" },
  { value: "교육", label: "교육" },
];

export const details = [
  { value: "간결", label: "간결" },
  { value: "보통", label: "보통" },
  { value: "상세", label: "상세" },
];

/**
 * suggestSlideCount keeps a forty page document from becoming forty slides.
 * The number of slides an audience can absorb follows the time available far
 * more closely than it follows the length of the source.
 */
export function suggestSlideCount(minutes: number, detail: string): number {
  const perMinute = detail === "상세" ? 1.1 : detail === "간결" ? 0.8 : 0.95;
  return Math.max(3, Math.min(50, Math.round(minutes * perMinute)));
}

export function statusLabel(status: PresentationStatus): string {
  switch (status) {
    case "completed":
      return "완료";
    case "failed":
      return "실패";
    case "generating":
      return "생성 중";
    case "queued":
      return "대기 중";
    case "draft":
      return "초안";
    default:
      return "준비 중";
  }
}

export function isBusy(status: PresentationStatus): boolean {
  return status === "pending" || status === "queued" || status === "generating";
}
