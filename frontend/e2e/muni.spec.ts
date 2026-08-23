import { expect, test } from "@playwright/test";

async function login(page: import("@playwright/test").Page) {
  await page.goto("/login");
  await page.getByLabel("아이디 또는 이메일").fill("admin@example.com");
  await page.getByLabel("비밀번호").fill("Integration-Admin-Password-2026");
  await page.getByRole("button", { name: "로그인", exact: true }).click();
  await expect(page.getByText(/님, 안녕하세요/)).toBeVisible();
}

test("login, create, edit, reload and open admin without browser errors", async ({
  page,
}) => {
  const browserErrors: string[] = [];
  page.on("pageerror", (error) => browserErrors.push(error.message));
  page.on("console", (message) => {
    if (
      message.type() === "error" &&
      !message.text().includes("401 (Unauthorized)")
    ) {
      browserErrors.push(message.text());
    }
  });

  await page.goto("/login");
  await expect(page.getByText(/muni v0\.1\.0-test/)).toBeVisible();
  await page.getByLabel("아이디 또는 이메일").fill("admin@example.com");
  await page.getByLabel("비밀번호").fill("Integration-Admin-Password-2026");
  await page.getByRole("button", { name: "로그인", exact: true }).click();
  await expect(page.getByText(/님, 안녕하세요/)).toBeVisible();

  await page
    .getByRole("button", { name: "새 문서", exact: true })
    .first()
    .click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("문서 제목").fill(`브라우저 검증 ${Date.now()}`);
  await dialog.getByRole("button", { name: "문서 만들기" }).click();
  await expect(page).toHaveURL(/\/docs\/[0-9a-f-]+/);

  const editor = page.locator('.tiptap[contenteditable="true"]');
  await expect(editor).toBeVisible({ timeout: 20_000 });
  await editor.click();
  await page.keyboard.type("브라우저 공동편집 검증 본문");
  await expect(page.getByText("저장됨", { exact: true })).toBeVisible({
    timeout: 15_000,
  });
  await page.reload();
  await expect(page).toHaveURL(/\/docs\/[0-9a-f-]+/);
  await expect(page.locator(".tiptap")).toContainText(
    "브라우저 공동편집 검증 본문",
    { timeout: 20_000 },
  );

  page.once("dialog", (dialog) => dialog.accept());
  await page.getByLabel("휴지통으로 이동").click();
  await expect(page).toHaveURL(/\/trash$/);
  await expect(page.getByLabel("문서 복원")).toBeVisible();
  await page.getByLabel("문서 복원").click();
  await expect(page.getByLabel("문서 복원")).toHaveCount(0);

  await page.getByRole("button", { name: "나에게 공유됨" }).click();
  await expect(page).toHaveURL(/\/shared$/);
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "나에게 공유됨" }),
  ).toBeVisible();
  await page.getByLabel("프로필 메뉴").click();
  await expect(page.getByText(/muni v0\.1\.0-test/)).toBeVisible();
  await page.getByRole("menuitem", { name: "서비스 관리" }).click();
  await expect(
    page.getByRole("heading", { name: "서비스 설정" }),
  ).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "서비스 설정" }),
  ).toBeVisible();
  expect(browserErrors).toEqual([]);
});

test("two isolated browser sessions receive realtime document updates", async ({
  browser,
}) => {
  const firstContext = await browser.newContext();
  const secondContext = await browser.newContext();
  const first = await firstContext.newPage();
  const second = await secondContext.newPage();
  const browserErrors: string[] = [];
  for (const page of [first, second]) {
    page.on("pageerror", (error) => browserErrors.push(error.message));
    page.on("console", (message) => {
      if (
        message.type() === "error" &&
        !message.text().includes("401 (Unauthorized)")
      )
        browserErrors.push(message.text());
    });
  }

  try {
    await login(first);
    await first
      .getByRole("button", { name: "새 문서", exact: true })
      .first()
      .click();
    const dialog = first.getByRole("dialog");
    await dialog
      .getByLabel("문서 제목")
      .fill(`실시간 두 세션 검증 ${Date.now()}`);
    await dialog.getByRole("button", { name: "문서 만들기" }).click();
    await expect(first).toHaveURL(/\/docs\/[0-9a-f-]+/);
    const documentURL = first.url();
    const firstEditor = first.locator('.tiptap[contenteditable="true"]');
    await expect(firstEditor).toBeVisible({ timeout: 20_000 });

    await login(second);
    await second.goto(documentURL);
    const secondEditor = second.locator(".tiptap");
    await expect(secondEditor).toBeVisible({ timeout: 20_000 });

    const marker = `동기화-${Date.now()}`;
    await firstEditor.click();
    await first.keyboard.type(marker);
    await expect(secondEditor).toContainText(marker, { timeout: 20_000 });
    expect(browserErrors).toEqual([]);
  } finally {
    await firstContext.close();
    await secondContext.close();
  }
});
