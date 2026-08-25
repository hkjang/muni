import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { useAIStream } from "./useAIStream";

function sse(...frames: string[]): Response {
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      const encoder = new TextEncoder();
      for (const frame of frames) controller.enqueue(encoder.encode(frame));
      controller.close();
    },
  });
  return new Response(body, {
    status: 200,
    headers: { "content-type": "text/event-stream" },
  });
}

function chunk(text: string): string {
  return `data: ${JSON.stringify({ choices: [{ delta: { content: text } }] })}\n\n`;
}

afterEach(() => vi.unstubAllGlobals());

describe("useAIStream", () => {
  it("assembles a streamed answer", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => sse(chunk("안녕"), chunk("하세요"), "data: [DONE]\n\n")));
    const { result } = renderHook(() => useAIStream());

    await act(async () => {
      await result.current.run({ prompt: "질문", action: "test" });
    });
    expect(result.current.text).toBe("안녕하세요");
    expect(result.current.running).toBe(false);
  });

  it("collects the tools the agent ran before the answer", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        sse(
          `data: ${JSON.stringify({ tool: "search_documents", arguments: '{"query":"계획"}' })}\n\n`,
          `data: ${JSON.stringify({ tool: "read_document", error: "권한이 없습니다" })}\n\n`,
          chunk("정리했습니다"),
          "data: [DONE]\n\n",
        ),
      ),
    );
    const { result } = renderHook(() => useAIStream());

    await act(async () => {
      await result.current.run({ prompt: "질문", action: "test", tools: true });
    });
    await waitFor(() => expect(result.current.toolCalls).toHaveLength(2));
    expect(result.current.toolCalls[0]!.tool).toBe("search_documents");
    expect(result.current.toolCalls[1]!.error).toBe("권한이 없습니다");
    // A tool event must not be mistaken for answer text.
    expect(result.current.text).toBe("정리했습니다");
  });

  it("asks for tools only when the caller wants them", async () => {
    const sent: RequestInit[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
        if (init) sent.push(init);
        return sse(chunk("ok"), "data: [DONE]\n\n");
      }),
    );
    const { result } = renderHook(() => useAIStream());

    await act(async () => {
      await result.current.run({ prompt: "질문", action: "test" });
    });
    expect(JSON.parse(sent[0]!.body as string).tools).toBeUndefined();

    await act(async () => {
      await result.current.run({ prompt: "질문", action: "test", tools: true });
    });
    expect(JSON.parse(sent[1]!.body as string).tools).toBe(true);
  });

  it("surfaces a rejected request", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        new Response(JSON.stringify({ error: { message: "AI가 꺼져 있습니다." } }), {
          status: 409,
          headers: { "content-type": "application/json" },
        }),
      ),
    );
    const { result } = renderHook(() => useAIStream());
    await act(async () => {
      await result.current.run({ prompt: "질문", action: "test" });
    });
    expect(result.current.error).toBe("AI가 꺼져 있습니다.");
  });

  it("clears the previous run when reset", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => sse(chunk("첫 답"), "data: [DONE]\n\n")));
    const { result } = renderHook(() => useAIStream());
    await act(async () => {
      await result.current.run({ prompt: "질문", action: "test" });
    });
    act(() => result.current.reset());
    expect(result.current.text).toBe("");
    expect(result.current.toolCalls).toHaveLength(0);
  });
});

describe("reasoning", () => {
  // A model that reasons out loud must not have its working applied to the
  // document by the selection menu.
  it("keeps the model's working out of the answer", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      sse(
        chunk("사용자가 요약을 원한다."),
        chunk("\n</think>\n"),
        chunk("요약: 세 항목입니다."),
        "data: [DONE]\n\n",
      ),
    ),
  );
  const { result } = renderHook(() => useAIStream());
  let returned = "";
  await act(async () => {
    returned = await result.current.run({ prompt: "요약", action: "test" });
  });
    expect(result.current.text).toBe("요약: 세 항목입니다.");
    expect(returned).toBe("요약: 세 항목입니다.");
  });
});
