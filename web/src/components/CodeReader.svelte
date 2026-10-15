<script lang="ts">
  import { onMount } from "svelte";
  import {
    EditorState,
    StateEffect,
    StateField,
    Text,
  } from "@codemirror/state";
  import {
    EditorView,
    lineNumbers,
    highlightActiveLine,
    highlightSpecialChars,
    keymap,
    GutterMarker,
    gutter,
    Decoration,
  } from "@codemirror/view";
  import {
    search,
    searchKeymap,
    openSearchPanel,
    gotoLine,
  } from "@codemirror/search";
  import {
    syntaxHighlighting,
    defaultHighlightStyle,
  } from "@codemirror/language";
  import { Button } from "$lib/components/ui/button";
  import type { CodeLine } from "../code-diff-model";
  let {
    text,
    path,
    mapping,
    onselect,
    restoreLine = 1,
  }: {
    text: string;
    path: string;
    mapping?: (CodeLine | null)[];
    onselect: (start: number, end: number) => void;
    restoreLine?: number;
  } = $props();
  let host: HTMLDivElement;
  let view: EditorView | undefined;
  let anchor: number | undefined;
  export function line() {
    return view ? view.state.doc.lineAt(view.viewport.from).number : 1;
  }
  function select(number: number, shift: boolean) {
    const coarse = matchMedia("(pointer: coarse)").matches;
    if ((shift || coarse) && anchor !== undefined) {
      onselect(Math.min(anchor, number), Math.max(anchor, number));
      anchor = undefined;
    } else {
      anchor = number;
      onselect(number, number);
    }
  }
  class NumberMarker extends GutterMarker {
    label: string;
    constructor(label: string) {
      super();
      this.label = label;
    }
    toDOM() {
      const element = document.createElement("span");
      element.textContent = this.label;
      return element;
    }
  }
  onMount(() => {
    let cancelled = false;
    const extensions = [
      EditorState.readOnly.of(true),
      EditorView.editable.of(false),
      search(),
      keymap.of(searchKeymap),
      syntaxHighlighting(defaultHighlightStyle),
      highlightActiveLine(),
      highlightSpecialChars(),
      EditorView.theme({
        "&": { height: "100%", fontSize: "13px" },
        ".cm-scroller": {
          overflow: "auto",
          fontFamily: "var(--font-mono, monospace)",
        },
        ".cm-content": { minWidth: "100%" },
        ".cm-gutters": { cursor: "pointer" },
      }),
      mapping
        ? gutter({
            class: "diff-gutter",
            lineMarker: (v, l) => {
              const m = mapping?.[v.state.doc.lineAt(l.from).number - 1];
              return new NumberMarker(
                m ? `${m.oldLine || ""} ${m.newLine || ""}` : "⋯",
              );
            },
            domEventHandlers: {
              click: (v, l, e) => {
                select(
                  v.state.doc.lineAt(l.from).number,
                  (e as MouseEvent).shiftKey,
                );
                return true;
              },
            },
          })
        : lineNumbers({
            domEventHandlers: {
              click: (v, l, e) => {
                select(
                  v.state.doc.lineAt(l.from).number,
                  (e as MouseEvent).shiftKey,
                );
                return true;
              },
            },
          }),
      StateField.define({
        create(state) {
          return Decoration.set(
            mapping
              ? mapping.flatMap((line, i) =>
                  line && (line.kind === "add" || line.kind === "delete")
                    ? [
                        Decoration.line({
                          class: `code-diff-${line.kind}`,
                        }).range(state.doc.line(i + 1).from),
                      ]
                    : [],
                )
              : [],
          );
        },
        update(value) {
          return value;
        },
        provide: (field) => EditorView.decorations.from(field),
      }),
      EditorView.updateListener.of((u) => {
        if (u.selectionSet) {
          const range = u.state.selection.main;
          if (!range.empty)
            onselect(
              u.state.doc.lineAt(range.from).number,
              u.state.doc.lineAt(Math.max(range.from, range.to - 1)).number,
            );
        }
      }),
    ];
    view = new EditorView({
      parent: host,
      state: EditorState.create({
        doc: Text.of(text.split(/\r?\n/)),
        extensions,
      }),
    });
    const target = Math.min(Math.max(restoreLine, 1), view.state.doc.lines);
    view.dispatch({
      effects: EditorView.scrollIntoView(view.state.doc.line(target).from, {
        y: "start",
      }),
    });
    void (async () => {
      const extension = path.split(".").at(-1)?.toLowerCase();
      let language;
      if (["js", "jsx", "ts", "tsx", "mjs", "svelte"].includes(extension ?? ""))
        language = (await import("@codemirror/lang-javascript")).javascript({
          typescript: extension?.startsWith("ts"),
          jsx: extension?.endsWith("x"),
        });
      else if (extension === "py")
        language = (await import("@codemirror/lang-python")).python();
      else if (extension === "json")
        language = (await import("@codemirror/lang-json")).json();
      else if (extension === "go")
        language = (await import("@codemirror/lang-go")).go();
      else if (extension === "rs")
        language = (await import("@codemirror/lang-rust")).rust();
      else if (extension === "html")
        language = (await import("@codemirror/lang-html")).html();
      else if (extension === "css")
        language = (await import("@codemirror/lang-css")).css();
      if (!cancelled && view && language) {
        if (!cancelled)
          view.dispatch({ effects: StateEffect.appendConfig.of(language) });
      }
    })().catch(() => {
      /* Plain text remains readable if language loading fails. */
    });
    return () => {
      cancelled = true;
      view?.destroy();
      view = undefined;
    };
  });
</script>

<div class="flex min-h-0 flex-1 flex-col overflow-hidden">
  <div class="flex shrink-0 gap-2 border-b px-2 py-1">
    <Button
      variant="ghost"
      size="sm"
      onclick={() => view && openSearchPanel(view)}>文件内查找</Button
    >
    <Button variant="ghost" size="sm" onclick={() => view && gotoLine(view)}
      >跳转行</Button
    >
    <span class="self-center text-xs text-muted-foreground"
      >点击行号；Shift 或手机再次点击选择终点</span
    >
  </div>
  <div
    bind:this={host}
    class="code-reader min-h-0 flex-1 overflow-hidden"
    aria-label="只读代码"
  ></div>
</div>
