import type { KeyboardEvent } from "react";

/**
 * confirmsInput reports whether Enter was pressed as a command rather than as
 * the key that finishes a Korean syllable.
 *
 * Hangul is typed through the input method: the letters ㅎ ㅏ ㄴ are gathered
 * into 한 and Enter confirms the syllable. A handler that submits on Enter
 * without asking whether a composition is open sends the form while the last
 * word is still half-typed — and the word is lost. The browser says so in
 * `isComposing`, and every field that acts on Enter has to ask.
 */
export function confirmsInput(event: KeyboardEvent): boolean {
  return event.key === "Enter" && !event.nativeEvent.isComposing;
}
