<script lang="ts">
  import type { Snippet } from "svelte";
  import CodeDocumentView from "../../src/components/code/CodeDocumentView.svelte";
  import * as DropdownMenu from "../../src/lib/components/ui/dropdown-menu";
  import { Button } from "../../src/lib/components/ui/button";
  import { CodeDiffSchema, CodeFileSchema } from "../../src/contracts/schemas";
  import type { MarkdownWorkspace } from "../../src/domains/code/markdown-link";
  let {
    text,
    workspace,
    compared = false,
  }: {
    text: string;
    workspace?: MarkdownWorkspace;
    compared?: boolean;
  } = $props();
  let heading = $state<{ slug: string; key: number }>();
  let menu = $state<Snippet>();
  let requests = 0;
  const file = $derived(
    CodeFileSchema.parse({
      snapshotId: "snapshot-1",
      checkedAt: "2026-10-30T00:00:00Z",
      document: {
        nodeId: "ZG9jcy9ndWlkZS5tZA",
        path: "docs/guide.md",
        absolutePath: "/repo/docs/guide.md",
        source: "worktree",
        oid: "",
        contentVersion: "v1",
        kind: "text",
        text,
        lineCount: text.split("\n").length,
        referenceable: true,
        reason: "",
        mediaType: "text/markdown",
        size: text.length,
        downloadable: true,
      },
    }),
  );
  // An uncommitted change of the same file, for the 查看 Diff toggle.
  const comparison = $derived(
    compared
      ? CodeDiffSchema.parse({
          snapshotId: "snapshot-1",
          old: { ...file.document, source: "index", text: "# 指南\n" },
          new: file.document,
          kind: "text",
          reason: "",
          checkedAt: "2026-10-30T00:00:00Z",
          change: {
            nodeId: "ZG9jcy9ndWlkZS5tZA",
            changeId: "change-1",
            group: "unstaged",
            oldPath: "docs/guide.md",
            newPath: "docs/guide.md",
            status: "M",
            oldOid: "",
            newOid: "",
            oldMode: "100644",
            newMode: "100644",
          },
        })
      : undefined,
  );
</script>

<button onclick={() => (heading = { slug: "第三节", key: ++requests })}
  >Request heading</button
>
<p aria-label="Pending heading">{heading ? heading.slug : "none"}</p>
<!-- Stands in for the code panel's narrow-screen menu. -->
<DropdownMenu.Root>
  <DropdownMenu.Trigger>
    {#snippet child({ props })}
      <Button {...props} variant="outline" size="sm">代码菜单</Button>
    {/snippet}
  </DropdownMenu.Trigger>
  <DropdownMenu.Content>{@render menu?.()}</DropdownMenu.Content>
</DropdownMenu.Root>
<div style="height:500px;width:700px">
  <CodeDocumentView
    id="session"
    {file}
    status="unchanged"
    contextId="context"
    draftEnabled={false}
    paste={() => "terminal"}
    refresh={() => {}}
    beginOpen={() => 1}
    openSide={() => {}}
    {comparison}
    onComparison={() => {}}
    onState={() => {}}
    {workspace}
    {heading}
    onHeading={() => (heading = undefined)}
    bind:menu
  />
</div>
