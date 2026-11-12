import {
  emptySearchSpec,
  findMatchLines,
  matchStatus,
  type MatchCursor,
  type SearchSpec,
} from "./code-search";
import {
  createReaderPanelActions,
  type CodeReaderPanel,
} from "./code-reader-controls";

export type CodeFindPanel = CodeReaderPanel;

/**
 * Search panel, query and match cursor shared by the file and diff readers.
 * `lines` is read reactively; `enabled` turns matches off while a diff has
 * nothing to search.
 */
export function createCodeFind(
  lines: () => readonly string[],
  host: () => HTMLElement,
  root: () => ParentNode,
  reveal: (line: number) => void,
  enabled?: () => boolean,
) {
  let panel = $state<CodeFindPanel>();
  let search = $state<SearchSpec>(emptySearchSpec());
  let cursor = $state.raw<MatchCursor>();
  const matches = $derived(
    enabled && !enabled() ? [] : findMatchLines(lines(), search),
  );
  const actions = createReaderPanelActions({
    root,
    host,
    getPanel: () => panel,
    setPanel: (value) => (panel = value),
    getCursor: () => cursor,
    setCursor: (value) => (cursor = value),
    setSearch: (value) => (search = value),
    getMatches: () => matches,
    reveal,
    enabled,
  });
  // Props both readers pass unchanged to CodeReaderPanels.
  const panelProps = {
    onquery: (value: SearchSpec) => actions.query(value),
    onnext: () => actions.nextMatch(1),
    onprevious: () => actions.nextMatch(-1),
    onclose: () => actions.closePanel(),
  };
  return {
    actions,
    panelProps,
    get panel() {
      return panel;
    },
    set panel(value) {
      panel = value;
    },
    get search() {
      return search;
    },
    set search(value) {
      search = value;
    },
    get cursor() {
      return cursor;
    },
    set cursor(value) {
      cursor = value;
    },
    get matches() {
      return matches;
    },
    get status() {
      return matchStatus(cursor, matches);
    },
  };
}
