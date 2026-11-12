<script lang="ts">
  import type { z } from "zod";
  import CodeNavigator from "../../src/components/code/CodeNavigator.svelte";
  import type {
    CodeContextSchema,
    CodeListing,
    CodeNavigatorState,
  } from "../../src/contracts/schemas";

  let {
    context,
    rootListing,
    fetchDirectory,
    changed = () => {},
    restore,
  }: {
    context: z.infer<typeof CodeContextSchema>;
    rootListing: CodeListing;
    fetchDirectory: (
      path: string,
      cursor: string,
      signal: AbortSignal,
    ) => Promise<CodeListing>;
    changed?: (removed: string[]) => void;
    restore?: CodeNavigatorState;
  } = $props();
  let section = $state<HTMLElement>();
  let navigator = $state<CodeNavigator>();
  let dropStatus = $state("");

  export function capture() {
    return navigator!.capture();
  }
</script>

<section
  bind:this={section}
  aria-label="代码工作区"
  style="height:600px;display:flex"
>
  <CodeNavigator
    bind:this={navigator}
    id="s1"
    {context}
    listing={undefined}
    {rootListing}
    {fetchDirectory}
    treeRefresh={0}
    activePath=""
    changes={undefined}
    changesError=""
    pending={false}
    changesEnabled={false}
    load={() => {}}
    openFile={() => {}}
    openDiff={() => {}}
    locate={async () => {}}
    sessionCwd=""
    reportedCwd={null}
    navStatus=""
    root="workspace"
    rootError=""
    switchRoot={() => {}}
    retryRoot={() => {}}
    reload={() => {}}
    {changed}
    {restore}
    dropHost={section}
    bind:dropStatus
  />
  <div data-testid="document" style="flex:1">文档</div>
</section>
<output aria-label="拖放状态">{dropStatus}</output>
