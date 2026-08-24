/** Types shared by the editor page and the panels it hosts. */

export type SideTab =
  | "ai"
  | "comments"
  | "history"
  | "suggestions"
  | "presentations";

export type Suggestion = {
  id: string;
  author: { id: string; displayName: string };
  /** Older suggestions carry a document position; newer ones a block anchor. */
  range: { from?: number; to?: number; blockId?: string };
  blockId?: string | null;
  origin?: "USER" | "AI";
  note?: string | null;
  previousValue?: unknown;
  newValue: unknown;
  status: "PENDING" | "ACCEPTED" | "REJECTED";
  createdAt: string;
};

export type Capability = {
  workflowEnabled: boolean;
  aiEnabled: boolean;
  pdfExport: boolean;
  docxExport: boolean;
  /** An administrator has connected a presentation service. */
  presentations: boolean;
  maxAiTokens: number;
};

export type Permission = {
  id: string;
  subjectType: string;
  subjectId?: string;
  role: string;
  label: string;
  expiresAt?: string;
};

export type UserSearch = {
  id: string;
  displayName: string;
  username: string;
  email: string;
};
