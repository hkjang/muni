import { describe, expect, it } from "vitest";
import { notificationTarget, unreadLabel } from "./notifications";

describe("notificationTarget", () => {
  it("opens the document a notification is about", () => {
    expect(notificationTarget("DOCUMENT", "abc")).toBe("/docs/abc");
  });

  it("opens a workspace", () => {
    expect(notificationTarget("WORKSPACE", "abc")).toBe("/workspace/abc");
  });

  it("goes nowhere when there is no screen for it", () => {
    expect(notificationTarget("SETTINGS", "abc")).toBe("");
    expect(notificationTarget("DOCUMENT", undefined)).toBe("");
    expect(notificationTarget(undefined, "abc")).toBe("");
  });
});

describe("unreadLabel", () => {
  it("shows nothing when everything is read", () => {
    expect(unreadLabel(0)).toBe("");
    expect(unreadLabel(-1)).toBe("");
  });

  it("counts up to ninety-nine and then stops being precise", () => {
    expect(unreadLabel(7)).toBe("7");
    expect(unreadLabel(99)).toBe("99");
    expect(unreadLabel(250)).toBe("99+");
  });
});
