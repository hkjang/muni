import { useCallback, useRef, useState } from "react";
import { errorMessage } from "../../../lib/api";

export type AIStreamRequest = {
  prompt: string;
  action: string;
  documentId?: string;
  maxTokens?: number;
  /** Let the model search and read documents before it answers. */
  tools?: boolean;
};

/** A tool the agent ran on the way to its answer. */
export type AIToolCall = {
  tool: string;
  arguments?: string;
  error?: string;
};

/**
 * useAIStream reads the SSE response of POST /api/v1/ai/chat and exposes the
 * answer as it arrives. The server normalises provider differences, so the
 * client only has to deal with OpenAI-shaped chunks.
 */
export function useAIStream() {
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  const [running, setRunning] = useState(false);
  const [toolCalls, setToolCalls] = useState<AIToolCall[]>([]);
  const controller = useRef<AbortController | null>(null);

  const stop = useCallback(() => {
    controller.current?.abort();
    controller.current = null;
  }, []);

  const reset = useCallback(() => {
    stop();
    setText("");
    setError("");
    setToolCalls([]);
    setRunning(false);
  }, [stop]);

  const run = useCallback(
    async (request: AIStreamRequest): Promise<string> => {
      controller.current?.abort();
      const current = new AbortController();
      controller.current = current;
      setText("");
      setError("");
      setToolCalls([]);
      setRunning(true);

      let answer = "";
      try {
        const response = await fetch("/api/v1/ai/chat", {
          method: "POST",
          credentials: "same-origin",
          headers: {
            "Content-Type": "application/json",
            Accept: "text/event-stream",
          },
          body: JSON.stringify({
            action: request.action,
            documentId: request.documentId ?? null,
            messages: [{ role: "user", content: request.prompt }],
            ...(request.maxTokens ? { maxTokens: request.maxTokens } : {}),
            ...(request.tools ? { tools: true } : {}),
          }),
          signal: current.signal,
        });
        if (!response.ok) {
          const body = await response.json().catch(() => null);
          throw new Error(body?.error?.message ?? "AI 요청에 실패했습니다.");
        }
        const reader = response.body?.getReader();
        if (!reader) throw new Error("스트리밍 응답을 읽을 수 없습니다.");
        const decoder = new TextDecoder();
        let buffer = "";
        for (;;) {
          const { value, done } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split("\n");
          buffer = lines.pop() ?? "";
          for (const line of lines) {
            const call = readToolEvent(line);
            if (call) {
              setToolCalls((current) => [...current, call]);
              continue;
            }
            const chunk = readChunk(line);
            if (chunk) {
              answer += chunk;
              setText(answer);
            }
          }
        }
        return answer;
      } catch (cause) {
        if ((cause as Error).name === "AbortError") return answer;
        setError(errorMessage(cause));
        return "";
      } finally {
        if (controller.current === current) controller.current = null;
        setRunning(false);
      }
    },
    [],
  );

  return { text, error, running, toolCalls, run, stop, reset };
}

/**
 * The agent reports each tool it ran as its own SSE event, so the reader can
 * see what it looked at before the answer arrives.
 */
function readToolEvent(line: string): AIToolCall | null {
  if (!line.startsWith("data:")) return null;
  const payload = line.slice(5).trim();
  if (!payload || payload === "[DONE]") return null;
  try {
    const parsed = JSON.parse(payload);
    if (typeof parsed.tool === "string") {
      return {
        tool: parsed.tool,
        arguments: parsed.arguments,
        error: parsed.error || undefined,
      };
    }
  } catch {
    // Not JSON: a heartbeat or a comment.
  }
  return null;
}

function readChunk(line: string): string {
  if (!line.startsWith("data:")) return "";
  const payload = line.slice(5).trim();
  if (!payload || payload === "[DONE]") return "";
  try {
    const parsed = JSON.parse(payload);
    const content = parsed.choices?.[0]?.delta?.content;
    if (typeof content === "string") return content;
    if (Array.isArray(content)) {
      return content
        .map((part: { text?: string }) => part.text ?? "")
        .join("");
    }
  } catch {
    // Provider heartbeats and comments are not JSON; ignore them.
  }
  return "";
}
