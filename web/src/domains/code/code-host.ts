import type { CodeHostState, CodeReveal } from "../../contracts/schemas";
import type { TextInsertion } from "../terminal/terminal-draft.svelte";

/** What a code workspace needs from the terminal page that hosts it. */
export interface CodeHost extends Readonly<CodeHostState> {
  /** A code location another view asks to show, such as from chat output. */
  readonly reveal?: CodeReveal;
  paste(text: string): Promise<TextInsertion>;
}
