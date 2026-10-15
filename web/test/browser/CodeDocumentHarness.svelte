<script lang="ts">
  import CodeDocumentView from "../../src/components/CodeDocumentView.svelte";
  import { CodeDiffSchema } from "../../src/schemas";
  let { deleted = false }: { deleted?: boolean } = $props();
  let revision = $state(1);
  let active = $state(true);
  let status = $state("unchanged");
  const original = $derived(
    Array.from(
      { length: deleted ? 20000 : 120 },
      (_, i) => `ordinary line ${i + 1}${deleted ? " " + "x".repeat(60) : ""}`,
    ).join("\n") + "\n",
  );
  const diff = $derived(
    CodeDiffSchema.parse({
      snapshotId: `snapshot-${revision}`,
      checkedAt: "2026-10-14T00:00:00Z",
      kind: "text",
      reason: "",
      change: {
        nodeId: "Y29kZS50eHQ",
        changeId: "change",
        group: "unstaged",
        oldPath: "code.txt",
        newPath: "code.txt",
        status: deleted ? "D" : "M",
        oldOid: "",
        newOid: "",
        oldMode: "100644",
        newMode: deleted ? "000000" : "100644",
      },
      old: {
        nodeId: "Y29kZS50eHQ",
        path: "code.txt",
        absolutePath: "/repo/code.txt",
        source: "index",
        oid: "",
        contentVersion: "old",
        kind: "text",
        text: original,
        lineCount: deleted ? 20000 : 120,
        referenceable: true,
        reason: "",
      },
      new: {
        nodeId: "Y29kZS50eHQ",
        path: "code.txt",
        absolutePath: "/repo/code.txt",
        source: deleted ? "empty" : "worktree",
        oid: "",
        contentVersion: `new-${revision}`,
        kind: "text",
        text: deleted
          ? ""
          : original.replace(
              "ordinary line 50\n",
              `changed revision ${revision}\n`,
            ),
        lineCount: deleted ? 0 : 120,
        referenceable: !deleted,
        reason: "",
      },
    }),
  );
</script>

<button onclick={() => revision++}>Replace snapshot</button>
<button onclick={() => (active = !active)}>Toggle active</button>
<div style="height:650px;width:650px">
  <CodeDocumentView
    id="session"
    {diff}
    {active}
    {status}
    contextId="context"
    paste={() => true}
    refresh={() => {}}
    beginOpen={() => 1}
    openSide={() => {}}
    onComparison={() => {}}
    onState={(value) => (status = value)}
  />
</div>
