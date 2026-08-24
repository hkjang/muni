/** Types shared by the editor page and the panels it hosts. */

export type SideTab = "ai" | "comments" | "history" | "suggestions";

export type Suggestion = {
  id: string;
  author: { id: string; displayName: string };
  range: { from?: number; to?: number };
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
