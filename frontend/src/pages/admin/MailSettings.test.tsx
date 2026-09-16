import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { MailDelivery, MailSettings as MailValue } from "../../types";
import { MailSettings, eventLabel, mailEvents } from "./MailSettings";

const fresh: MailValue = {
  enabled: false,
  smtpHost: "",
  smtpPort: 25,
  security: "auto",
  skipTlsVerify: false,
  username: "",
  passwordSet: false,
  fromAddress: "",
  fromName: "",
  baseUrl: "",
  timeoutSeconds: 10,
  notify: {
    approvalRequest: true,
    approvalDecision: true,
    mention: true,
    apiKeyExpiring: true,
  },
};

const fetchMock = vi.fn();

function renderTab(
  value: MailValue,
  deliveries: MailDelivery[],
  testStatus = 200,
) {
  fetchMock.mockImplementation(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/admin/mail/deliveries")) {
        const sent = deliveries.filter((d) => d.status === "sent").length;
        return new Response(
          JSON.stringify({
            data: {
              items: deliveries,
              summary: {
                total: deliveries.length,
                sent,
                failed: deliveries.length - sent,
              },
            },
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }
      if (url.endsWith("/settings/test-mail") && init?.method === "POST") {
        if (testStatus !== 200) {
          return new Response(
            JSON.stringify({
              error: {
                code: "MAIL_TEST_FAILED",
                message: "메일 서버에 연결하지 못했습니다",
              },
            }),
            {
              status: testStatus,
              headers: { "content-type": "application/json" },
            },
          );
        }
        return new Response(
          JSON.stringify({ data: { ok: true, sentTo: "admin@example.com" } }),
          {
            status: 200,
            headers: { "content-type": "application/json" },
          },
        );
      }
      return new Response(null, { status: 204 });
    },
  );
  const onChange = vi.fn();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MailSettings value={value} onChange={onChange} />
    </QueryClientProvider>,
  );
  return { onChange };
}

beforeEach(() => vi.stubGlobal("fetch", fetchMock));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  fetchMock.mockReset();
});

describe("MailSettings", () => {
  it("offers one switch per event, all on by default", () => {
    renderTab(fresh, []);
    for (const event of mailEvents) {
      const box = screen.getByLabelText(
        new RegExp(event.label),
      ) as HTMLInputElement;
      expect(box.checked).toBe(true);
    }
    expect(mailEvents.length).toBe(4);
  });

  it("turns one event off without touching the rest", () => {
    const { onChange } = renderTab(fresh, []);
    fireEvent.click(screen.getByLabelText(/댓글 멘션/));
    expect(onChange).toHaveBeenLastCalledWith({
      ...fresh,
      notify: { ...fresh.notify, mention: false },
    });
  });

  it("never shows a stored password, only that one is set", () => {
    renderTab({ ...fresh, passwordSet: true }, []);
    const field = screen.getByLabelText(
      /비밀번호 \(설정됨\)/,
    ) as HTMLInputElement;
    expect(field.value).toBe("");
    expect(field.placeholder).toBe("변경할 때만 입력");
  });

  it("cannot test a relay that has no address", () => {
    renderTab(fresh, []);
    const button = screen.getByRole("button", {
      name: "내 주소로 시험 메일 보내기",
    });
    expect((button as HTMLButtonElement).disabled).toBe(true);
  });

  it("sends the form as it stands and reports where the test went", async () => {
    const value = {
      ...fresh,
      smtpHost: "relay.internal",
      fromAddress: "muni@example.com",
    };
    renderTab(value, []);
    fireEvent.click(
      screen.getByRole("button", { name: "내 주소로 시험 메일 보내기" }),
    );
    await waitFor(() =>
      expect(
        screen.getByText(/admin@example.com 주소로 시험 메일을 보냈습니다/),
      ).toBeTruthy(),
    );
    const call = fetchMock.mock.calls.find(([input]) =>
      String(input).endsWith("/settings/test-mail"),
    );
    expect(call).toBeTruthy();
    expect(JSON.parse(String(call![1]?.body))).toEqual(value);
  });

  it("shows the relay's refusal in place", async () => {
    renderTab({ ...fresh, smtpHost: "relay.internal" }, [], 502);
    fireEvent.click(
      screen.getByRole("button", { name: "내 주소로 시험 메일 보내기" }),
    );
    await waitFor(() =>
      expect(screen.getByText(/메일 서버에 연결하지 못했습니다/)).toBeTruthy(),
    );
  });

  it("lists every attempt with its outcome and no body", async () => {
    renderTab(fresh, [
      {
        id: "1",
        event: "DIGEST",
        recipient: "hong@example.com",
        subject: "[muni] 문서 검토 요청 외 2건",
        notifications: 3,
        status: "sent",
        error: "",
        createdAt: "2026-09-16T00:00:00Z",
      },
      {
        id: "2",
        event: "MENTION",
        recipient: "kim@example.com",
        subject: "문서 댓글에서 회원님을 멘션했습니다.",
        notifications: 1,
        status: "failed",
        error: "메일 서버에 연결하지 못했습니다",
        createdAt: "2026-09-16T00:01:00Z",
      },
    ]);
    await waitFor(() =>
      expect(screen.getByText("hong@example.com")).toBeTruthy(),
    );
    expect(screen.getByText("묶음 (3건)")).toBeTruthy();
    expect(screen.getByText("보냄")).toBeTruthy();
    expect(screen.getByText("실패")).toBeTruthy();
    expect(screen.getByText("메일 서버에 연결하지 못했습니다")).toBeTruthy();
    expect(screen.getByText(/전체 2건 · 성공 1 · 실패 1/)).toBeTruthy();
  });

  it("names events the way the settings do", () => {
    expect(eventLabel("APPROVAL_REQUEST")).toBe("검토·결재 요청");
    expect(eventLabel("TEST")).toBe("시험 발송");
    expect(eventLabel("SOMETHING_NEW")).toBe("SOMETHING_NEW");
  });
});
