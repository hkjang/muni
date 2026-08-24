import type { Editor } from "@tiptap/react";
import { Box, Tab, Tabs } from "@mui/material";
import {
  AddCommentOutlined,
  AutoAwesome,
  CommentOutlined,
  History,
} from "@mui/icons-material";
import type { DocumentItem } from "../../types";
import type { Capability, SideTab } from "./types";
import { AgentPanel } from "./ai/AgentPanel";
import { CommentsPanel } from "./comments/CommentsPanel";
import { SuggestionsPanel } from "./suggestions/SuggestionsPanel";
import { HistoryPanel } from "./history/HistoryPanel";

export function EditorSidebar({
  tab,
  setTab,
  document,
  editor,
  canComment,
  canEdit,
  capabilities,
}: {
  tab: SideTab;
  setTab: (value: SideTab) => void;
  document: DocumentItem;
  editor: Editor;
  canComment: boolean;
  canEdit: boolean;
  capabilities?: Capability;
}) {
  return (
    <Box sx={{ height: "100%", display: "flex", flexDirection: "column" }}>
      <Tabs
        value={tab}
        onChange={(_, value) => setTab(value)}
        variant="scrollable"
        scrollButtons={false}
        sx={{
          borderBottom: "1px solid",
          borderColor: "divider",
          minHeight: 49,
        }}
      >
        <Tab
          value="ai"
          icon={<AutoAwesome />}
          iconPosition="start"
          label="AI"
        />
        <Tab
          value="comments"
          icon={<CommentOutlined />}
          iconPosition="start"
          label="댓글"
        />
        <Tab
          value="suggestions"
          icon={<AddCommentOutlined />}
          iconPosition="start"
          label="제안"
        />
        <Tab
          value="history"
          icon={<History />}
          iconPosition="start"
          label="버전"
        />
      </Tabs>
      <Box sx={{ flex: 1, minHeight: 0, overflowY: "auto", p: 2 }}>
        {tab === "ai" && (
          <AgentPanel
            document={document}
            editor={editor}
            enabled={capabilities?.aiEnabled ?? true}
            canEdit={canEdit}
            maxTokens={capabilities?.maxAiTokens ?? 32768}
          />
        )}{" "}
        {tab === "comments" && (
          <CommentsPanel
            document={document}
            editor={editor}
            canComment={canComment}
          />
        )}{" "}
        {tab === "suggestions" && (
          <SuggestionsPanel
            document={document}
            editor={editor}
            canComment={canComment}
            canEdit={canEdit}
            aiEnabled={Boolean(capabilities?.aiEnabled)}
          />
        )}{" "}
        {tab === "history" && <HistoryPanel document={document} />}
      </Box>
    </Box>
  );
}
