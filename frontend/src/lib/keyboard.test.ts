import { describe, expect, it } from "vitest";
import type { KeyboardEvent } from "react";
import { confirmsInput } from "./keyboard";

function press(key: string, isComposing: boolean): KeyboardEvent {
  return { key, nativeEvent: { isComposing } } as KeyboardEvent;
}

describe("confirmsInput", () => {
  it("is true for Enter typed as a command", () => {
    expect(confirmsInput(press("Enter", false))).toBe(true);
  });

  // The Enter that turns ㅎㅏㄴ into 한 is not a command; a form that submits
  // on it loses the syllable being composed.
  it("is false for the Enter that finishes a Korean syllable", () => {
    expect(confirmsInput(press("Enter", true))).toBe(false);
  });

  it("is false for every other key", () => {
    expect(confirmsInput(press("a", false))).toBe(false);
    expect(confirmsInput(press("Escape", false))).toBe(false);
  });
});
