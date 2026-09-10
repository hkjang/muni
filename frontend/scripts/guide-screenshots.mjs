// 가이드 문서(docs/USER_GUIDE.md, docs/ADMIN_GUIDE.md)에 싣는 화면 캡처를 찍습니다.
//
// 실제로 띄운 muni 를 데모 데이터로 채운 뒤 1440x900 headless Chromium 으로 찍습니다.
// 데모 데이터를 만들기 때문에 **버려도 되는 배포**에만 쓸 수 있습니다. 대상 주소는
// e2e 와 공유하지 않는 전용 변수로 받고, 자격 증명도 환경 변수로만 받습니다.
//
//   MUNI_GUIDE_BASE_URL=http://127.0.0.1:8080 \
//   MUNI_GUIDE_ADMIN=admin@example.com MUNI_GUIDE_ADMIN_PASSWORD=... \
//   node scripts/guide-screenshots.mjs
//
// 원격 주소를 찍어야 하면 MUNI_GUIDE_ALLOW_REMOTE=1 을 함께 줍니다.
import { chromium } from "@playwright/test";
import { mkdir } from "node:fs/promises";
import { randomBytes } from "node:crypto";
import path from "node:path";

const OUT = path.resolve(process.argv[2] ?? "../docs/assets/guide");
const VIEWPORT = { width: 1440, height: 900 };

function required(name) {
  const value = process.env[name]?.trim();
  if (!value) {
    console.error(`${name} 가 필요합니다. 이 스크립트는 데모 데이터를 만들기 때문에 대상을 짐작하지 않습니다.`);
    process.exit(2);
  }
  return value;
}

const baseURL = required("MUNI_GUIDE_BASE_URL");
const adminIdentity = required("MUNI_GUIDE_ADMIN");
const adminPassword = required("MUNI_GUIDE_ADMIN_PASSWORD");
const localHost = /^https?:\/\/(127\.0\.0\.1|localhost|\[::1\])(:\d+)?$/i.test(
  baseURL.replace(/\/$/, ""),
);
if (!localHost && process.env.MUNI_GUIDE_ALLOW_REMOTE !== "1") {
  console.error(
    `${baseURL} 는 로컬 주소가 아닙니다. 운영 배포에 데모 데이터를 만들지 않도록, 정말 버려도 되는 배포라면 MUNI_GUIDE_ALLOW_REMOTE=1 을 주세요.`,
  );
  process.exit(2);
}

// 데모 계정 비밀번호는 스크립트에 적지 않고 실행할 때마다 새로 만듭니다.
const demoPassword = () => `${randomBytes(12).toString("base64url")}Aa1!`;

const heading = (level, text) => ({
  type: "heading",
  attrs: { level },
  content: [{ type: "text", text }],
});
const para = (text, marks) => ({
  type: "paragraph",
  content: text ? [{ type: "text", text, ...(marks ? { marks } : {}) }] : [],
});
const bullets = (items) => ({
  type: "bulletList",
  content: items.map((text) => ({
    type: "listItem",
    content: [para(text)],
  })),
});
const cell = (type, text) => ({
  type,
  attrs: { colspan: 1, rowspan: 1 },
  content: [para(text)],
});
const table = (rows) => ({
  type: "table",
  content: rows.map((row, index) => ({
    type: "tableRow",
    content: row.map((text) => cell(index === 0 ? "tableHeader" : "tableCell", text)),
  })),
});

const documents = [
  {
    title: "2026년 상반기 사업계획",
    folder: "기획",
    content: [
      heading(1, "2026년 상반기 사업계획"),
      para(
        "본 문서는 데모용으로 작성한 예시입니다. 상반기에 추진할 과제와 일정, 예산을 한 곳에 모았습니다.",
      ),
      heading(2, "추진 과제"),
      bullets([
        "문서 협업 도구 사내 도입 — 3월 착수",
        "결재 흐름 전자화 — 4월 시범 운영",
        "문서 보존 정책 정비 — 6월 완료",
      ]),
      heading(2, "분기별 일정"),
      table([
        ["과제", "1분기", "2분기", "담당"],
        ["도구 도입", "요건 정리", "전사 확대", "기획팀"],
        ["결재 전자화", "설계", "시범 운영", "총무팀"],
        ["보존 정책", "-", "규정 개정", "법무팀"],
      ]),
      heading(2, "검토 의견"),
      para(
        "예산은 확정 전 수치이며, 검토 및 승인 단계에서 다시 확인합니다.",
      ),
    ],
  },
  {
    title: "제품 회의록 (3월 2주)",
    folder: "회의록",
    content: [
      heading(1, "제품 회의록 (3월 2주)"),
      para("일시: 2026-03-09 10:00 / 장소: 회의실 A / 작성: 데모 사용자"),
      heading(2, "논의"),
      bullets([
        "가져오기 형식에 HWP 를 추가하기로 함",
        "내보내기 기본값은 PDF 로 유지",
        "다음 회의까지 사용 통계 정리",
      ]),
      heading(2, "결정 사항"),
      table([
        ["안건", "결정", "기한"],
        ["HWP 가져오기", "적용", "3월 말"],
        ["PDF 기본값", "유지", "-"],
      ]),
    ],
  },
  {
    title: "문서 작성 가이드라인",
    folder: "기획",
    content: [
      heading(1, "문서 작성 가이드라인"),
      para("공용 문서를 쓸 때 지키는 약속입니다."),
      bullets([
        "제목은 개요 1, 절은 개요 2 로 씁니다",
        "표에는 반드시 머리글 행을 둡니다",
        "확정되지 않은 수치는 댓글로 표시합니다",
      ]),
      { type: "horizontalRule" },
      para("문의: 기획팀 (plan@example.com)"),
    ],
  },
  {
    title: "월간 운영 보고 (2월)",
    folder: "보고",
    content: [
      heading(1, "월간 운영 보고 (2월)"),
      para("2월 한 달 동안의 문서 처리 현황을 정리했습니다."),
      table([
        ["지표", "1월", "2월"],
        ["신규 문서", "128", "164"],
        ["결재 완료", "43", "57"],
        ["내보내기", "212", "258"],
      ]),
      heading(2, "다음 달 계획"),
      bullets(["보존 정책 미리보기 실행", "휴지통 정리 안내 공지"]),
    ],
  },
  {
    title: "신규 입사자 온보딩 안내",
    folder: "보고",
    content: [
      heading(1, "신규 입사자 온보딩 안내"),
      para("입사 첫 주에 해야 할 일을 순서대로 적었습니다."),
      {
        type: "orderedList",
        attrs: { start: 1 },
        content: [
          "계정을 받아 비밀번호를 바꿉니다",
          "개인 워크스페이스에서 문서를 하나 만들어 봅니다",
          "팀 워크스페이스 초대를 확인합니다",
        ].map((text) => ({ type: "listItem", content: [para(text)] })),
      },
      {
        type: "blockquote",
        content: [para("막히면 팀 채널에 물어보세요. 관리자가 도와드립니다.")],
      },
    ],
  },
];

const shots = [];
async function shoot(page, name, options = {}) {
  await page.waitForTimeout(options.settle ?? 700);
  const file = path.join(OUT, `${name}.png`);
  await page.screenshot({ path: file, ...(options.fullPage ? { fullPage: true } : {}) });
  shots.push(name);
  console.log(`  ✓ ${name}.png`);
}

async function api(page, method, url, body) {
  const response = await page.request.fetch(`${baseURL}${url}`, {
    method,
    headers: { "Content-Type": "application/json" },
    ...(body ? { data: body } : {}),
  });
  if (!response.ok()) {
    throw new Error(`${method} ${url} → ${response.status()} ${await response.text()}`);
  }
  const text = await response.text();
  return text ? JSON.parse(text).data : null;
}

async function main() {
  await mkdir(OUT, { recursive: true });
  const browser = await chromium.launch({
    executablePath: process.env.MUNI_GUIDE_CHROMIUM || undefined,
    args: ["--no-sandbox", "--font-render-hinting=none"],
  });
  const context = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: 1,
    locale: "ko-KR",
    timezoneId: "Asia/Seoul",
  });
  const page = await context.newPage();
  page.setDefaultTimeout(30_000);

  // 1. 로그인 화면 — 아직 아무것도 채우지 않은 상태로 먼저 찍습니다.
  await page.goto(`${baseURL}/login`);
  await page.getByLabel("아이디 또는 이메일").waitFor();
  await shoot(page, "login");

  await page.getByLabel("아이디 또는 이메일").fill(adminIdentity);
  await page.getByLabel("비밀번호").fill(adminPassword);
  await page.getByRole("button", { name: "로그인", exact: true }).click();
  await page.getByText(/님, 안녕하세요/).waitFor();

  // 2. 데모 데이터 — 화면이 비어 있으면 가이드에 실을 수 없습니다.
  const me = (await api(page, "GET", "/api/v1/auth/me")).user;
  await api(page, "PATCH", `/api/v1/admin/users/${me.id}`, {
    displayName: "데모 관리자",
  });
  const teammates = [
    { email: "hong@example.com", displayName: "홍길동", role: "USER" },
    { email: "kim@example.com", displayName: "김서연", role: "USER" },
    { email: "lee@example.com", displayName: "이준호", role: "USER" },
  ];
  const created = [];
  for (const teammate of teammates) {
    try {
      created.push(
        await api(page, "POST", "/api/v1/admin/users", {
          ...teammate,
          password: demoPassword(),
        }),
      );
    } catch (error) {
      console.log(`  · 계정 ${teammate.email} 을 만들지 못했습니다: ${error.message}`);
    }
  }

  const workspaces = await api(page, "GET", "/api/v1/workspaces");
  let team = workspaces.find((workspace) => workspace.slug === "demo-plan");
  if (!team) {
    team = await api(page, "POST", "/api/v1/workspaces", {
      name: "데모 회사 기획팀",
      slug: "demo-plan",
      description: "기획팀이 함께 쓰는 문서 공간입니다.",
    });
  }
  for (const user of created) {
    await api(page, "PUT", `/api/v1/workspaces/${team.id}/members`, {
      userId: user.id ?? user.user?.id,
      role: "EDITOR",
    }).catch(() => {});
  }

  const existingFolders = await api(page, "GET", `/api/v1/workspaces/${team.id}/folders`);
  const folders = new Map(existingFolders.map((folder) => [folder.name, folder]));
  for (const name of ["기획", "회의록", "보고"]) {
    if (!folders.has(name)) {
      folders.set(
        name,
        await api(page, "POST", `/api/v1/workspaces/${team.id}/folders`, {
          name,
          parentId: null,
        }),
      );
    }
  }

  const existingDocuments = await api(page, "GET", `/api/v1/workspaces/${team.id}/documents`);
  const byTitle = new Map((existingDocuments.items ?? existingDocuments).map((d) => [d.title, d]));
  const saved = [];
  for (const document of documents) {
    let record = byTitle.get(document.title);
    if (!record) {
      record = await api(page, "POST", "/api/v1/documents", {
        workspaceId: team.id,
        folderId: folders.get(document.folder).id,
        title: document.title,
        content: { type: "doc", content: document.content },
      });
    }
    saved.push(record);
  }
  const main = saved[0];

  // 문서 하나에는 댓글과 공유를 남겨 둡니다 — 빈 패널을 찍지 않기 위해서입니다.
  const teammate = created[0];
  if (teammate) {
    await api(page, "PUT", `/api/v1/documents/${main.id}/permissions`, {
      subjectType: "USER",
      subjectId: teammate.id ?? teammate.user?.id,
      role: "COMMENTER",
    }).catch(() => {});
  }
  const comments = await api(page, "GET", `/api/v1/documents/${main.id}/comments`);
  if (!(comments.items ?? comments).length) {
    await api(page, "POST", `/api/v1/documents/${main.id}/comments`, {
      body: "예산 수치는 확정 전입니다. 3월 회의 뒤에 다시 확인해 주세요.",
    }).catch((error) => console.log(`  · 댓글 생성 건너뜀: ${error.message}`));
  }
  await api(page, "POST", `/api/v1/documents/${main.id}/favorite`, {}).catch(() => {});

  // 버전 기록 패널이 빈 채로 찍히지 않도록 개정을 하나 더 만듭니다.
  const current = await api(page, "GET", `/api/v1/documents/${main.id}`);
  await api(page, "PUT", `/api/v1/documents/${main.id}`, {
    content: {
      type: "doc",
      content: [...documents[0].content, para("예산 항목은 3월 회의 뒤에 확정합니다.")],
    },
    expectedRevision: current.revision,
    reason: "검토 의견 반영",
  }).catch((error) => console.log(`  · 개정 생성 건너뜀: ${error.message}`));

  // 3. 사용자 화면
  await page.goto(`${baseURL}/`);
  await page.getByText("최근 문서").first().waitFor();
  await shoot(page, "home");

  await page.getByRole("button", { name: "새 문서", exact: true }).first().click();
  await page.getByRole("dialog").waitFor();
  await shoot(page, "new-document");
  await page.keyboard.press("Escape");

  await page.goto(`${baseURL}/workspace/${team.id}`);
  await page.getByText(documents[0].title).first().waitFor();
  await shoot(page, "workspace");

  // 편집기는 목록에서 열어 들어갑니다.
  const openEditor = async () => {
    await page.goto(`${baseURL}/`);
    await page.getByText("최근 문서").first().waitFor();
    await page.getByText(documents[0].title).first().click();
    await page.locator(".tiptap").first().waitFor();
    await page.waitForTimeout(2000);
  };
  await openEditor();
  await shoot(page, "editor", { settle: 1500 });

  await page.getByRole("tab", { name: "댓글" }).click();
  await shoot(page, "editor-comments");

  await page.getByRole("tab", { name: "버전" }).click();
  await shoot(page, "editor-history", { settle: 1500 });

  await page.getByRole("button", { name: "내보내기" }).click();
  await shoot(page, "editor-export");
  await page.keyboard.press("Escape");

  await page.getByRole("button", { name: "공유" }).click();
  await page.getByRole("dialog").waitFor();
  await shoot(page, "share", { settle: 1200 });
  await page.keyboard.press("Escape");

  await page.goto(`${baseURL}/`);
  await page.getByText("최근 문서").first().waitFor();
  await page.keyboard.press("Control+k");
  await page.waitForTimeout(500);
  await page.keyboard.type("보고");
  await shoot(page, "quick-switcher", { settle: 1500 });
  await page.keyboard.press("Escape");

  await page.goto(`${baseURL}/search?q=${encodeURIComponent("문서")}`);
  await page.getByRole("heading", { name: "검색" }).first().waitFor();
  await shoot(page, "search", { settle: 2000 });

  await page.goto(`${baseURL}/settings`);
  await page.getByRole("heading", { name: "개인 설정" }).first().waitFor();
  await shoot(page, "settings", { settle: 1200 });

  // 4. 관리자 화면
  const adminScreens = [
    ["admin-overview", "/admin", "운영 현황"],
    ["admin-settings", "/admin/settings", "서비스 설정"],
    ["admin-users", "/admin/users", "사용자 관리"],
    ["admin-workspaces", "/admin/workspaces", "워크스페이스"],
    ["admin-documents", "/admin/documents", "문서 관리"],
    ["admin-key-policies", "/admin/key-policies", "키 권한 정책"],
    ["admin-audit", "/admin/audit", "감사 로그"],
  ];
  for (const [name, url, headingText] of adminScreens) {
    await page.goto(`${baseURL}${url}`);
    await page
      .getByRole("heading", { name: headingText })
      .first()
      .waitFor()
      .catch(() => {});
    await shoot(page, name, { settle: 1800 });
  }

  await page.goto(`${baseURL}/admin/users`);
  await page.getByRole("button", { name: /계정 만들기/ }).first().click();
  await page.getByRole("dialog").waitFor();
  await shoot(page, "admin-users-create", { settle: 1000 });

  await probe(page);

  await context.close();
  await browser.close();
  console.log(`\n${shots.length}장을 ${OUT} 에 저장했습니다.`);
}

// 화면마다 어떤 조작 대상이 있는지 확인할 때 씁니다 (MUNI_GUIDE_PROBE=1).
async function probe(page) {
  if (process.env.MUNI_GUIDE_PROBE !== "1") return;
  const controls = await page.evaluate(() =>
    Array.from(document.querySelectorAll("button,[role=button],[role=tab]"))
      .map((element) =>
        [
          element.getAttribute("aria-label"),
          element.getAttribute("title"),
          element.textContent?.trim().slice(0, 24),
        ]
          .filter(Boolean)
          .join(" | "),
      )
      .filter(Boolean),
  );
  console.log("PROBE:", JSON.stringify(Array.from(new Set(controls)), null, 1));
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
