export type User = {
  id: string;
  username: string;
  email: string;
  displayName: string;
  role: "ADMIN" | "USER";
  status: "ACTIVE" | "SUSPENDED";
  avatarUrl?: string;
  locale: string;
  createdAt: string;
  /**
   * Set when the password was chosen by somebody else — muni generated it, or
   * an administrator typed it. The server refuses every other endpoint until
   * it is replaced, so the app has to send the person to that screen rather
   * than to a wall of 403s.
   */
  mustChangePassword?: boolean;
};

export type BuildInfo = { version: string; commit: string; buildTime: string };

export type PublicSystem = {
  serviceName: string;
  version: string;
  commit: string;
  localLoginEnabled: boolean;
  oidcEnabled: boolean;
  oidcLoginUrl: string;
  maxAiTokens: number;
};

export type Workspace = {
  id: string;
  name: string;
  slug: string;
  description: string;
  kind: "PERSONAL" | "TEAM" | "DEPARTMENT" | "ORGANIZATION";
  role: "OWNER" | "MANAGER" | "MEMBER" | "VIEWER";
  updatedAt: string;
};

export type Folder = {
  id: string;
  workspaceId: string;
  parentId?: string;
  name: string;
  createdAt: string;
  updatedAt: string;
};

export type DocumentContent = {
  type: "doc";
  content?: Array<Record<string, unknown>>;
};

export type DocumentItem = {
  id: string;
  workspaceId: string;
  folderId?: string;
  ownerId: string;
  ownerName: string;
  title: string;
  status: "DRAFT" | "REVIEW" | "PUBLISHED" | "ARCHIVED";
  visibility: "RESTRICTED" | "WORKSPACE" | "ORGANIZATION" | "LINK";
  workflowStatus: "NONE" | "DRAFT" | "PENDING" | "APPROVED" | "REJECTED";
  content?: DocumentContent;
  revision: number;
  /** Counts up when the shared editing state is replaced, such as by a restore. */
  crdtGeneration: number;
  /** "none", "decimal" or "korean"; the numbers themselves are never stored. */
  headingNumbering: string;
  /** Labels that group documents across folders. */
  tags: string[];
  favorite: boolean;
  permission: "OWNER" | "EDITOR" | "COMMENTER" | "VIEWER";
  createdAt: string;
  updatedAt: string;
  deletedAt?: string;
};

export type Settings = {
  general: {
    serviceName: string;
    allowLocalLogin: boolean;
    defaultLocale: string;
    pageSize: number;
  };
  oidc: {
    enabled: boolean;
    issuerUrl: string;
    clientId: string;
    clientSecret?: string;
    secretSet: boolean;
    redirectUrl: string;
    scopes: string[];
    autoProvision: boolean;
    defaultRole: "ADMIN" | "USER";
  };
  ai: {
    enabled: boolean;
    baseUrl: string;
    apiKey?: string;
    apiKeySet: boolean;
    model: string;
    maxTokens: number;
    timeoutSeconds: number;
    systemPrompt: string;
  };
  workflow: {
    enabled: boolean;
    requiredApprovals: number;
    allowSelfApproval: boolean;
  };
  security: {
    sessionHours: number;
    apiKeyMaxDays: number;
    allowPublicLinks: boolean;
    maxUploadMb: number;
    auditReads: boolean;
  };
  export: { enablePdf: boolean; enableDocx: boolean };
  smtp: {
    enabled: boolean;
    host: string;
    port: number;
    username: string;
    password?: string;
    passwordSet: boolean;
    security: string;
    from: string;
    fromName: string;
    skipVerify: boolean;
    baseUrl: string;
  };
  retention: {
    trashDays: number;
    revisionDays: number;
    revisionKeep: number;
    auditDays: number;
    aiAuditDays: number;
  };
  ptium: {
    enabled: boolean;
    baseUrl: string;
    webUrl: string;
    apiKey?: string;
    apiKeySet: boolean;
    defaultTheme: string;
    defaultLocale: string;
    timeoutSeconds: number;
  };
};

export type Template = {
  id: string;
  /** Absent for a template shared across the whole service. */
  workspaceId?: string;
  name: string;
  description: string;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
};

export type CommentItem = {
  id: string;
  parentId?: string;
  author: { id: string; displayName: string };
  anchor?: Record<string, unknown>;
  body: string;
  resolvedAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type RevisionItem = {
  id: string;
  revision: number;
  reason?: string;
  name?: string;
  createdAt: string;
  author: { id: string; displayName: string };
};
