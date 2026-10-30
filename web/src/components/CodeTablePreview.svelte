<script lang="ts">
  import { maxTableRows, parseDelimited } from "../code-csv";
  import { Button } from "$lib/components/ui/button";
  import * as Table from "$lib/components/ui/table";
  let {
    text,
    delimiter,
    onsource,
  }: {
    text: string;
    delimiter: "," | "\t";
    onsource: () => void;
  } = $props();
  const table = $derived(parseDelimited(text, delimiter));
</script>

<div class="flex min-h-0 flex-1 flex-col">
  {#if !table.ok}
    <div class="space-y-2 p-2 text-sm">
      <p class="text-destructive" role="alert">
        表格无法解析：{table.message}
      </p>
      <Button variant="outline" size="sm" onclick={onsource}>返回源码</Button>
    </div>
  {:else if table.columns === 0}
    <p class="p-2 text-sm text-muted-foreground" role="status">文件为空</p>
  {:else}
    {#if table.total > table.rows.length}<p
        class="shrink-0 border-b px-2 py-1 text-xs text-muted-foreground"
        role="status"
      >
        仅显示前 {maxTableRows} 行，共 {table.total} 行
      </p>{/if}
    <Table.Root variant="sheet" aria-label="表格预览">
      <Table.Header>
        <Table.Row>
          {#each table.header as cell, index (index)}
            <Table.Head>{cell}</Table.Head>
          {/each}
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {#each table.rows as row, index (index)}
          <Table.Row>
            {#each row as cell, column (column)}
              <Table.Cell>{cell}</Table.Cell>
            {/each}
          </Table.Row>
        {/each}
      </Table.Body>
    </Table.Root>
  {/if}
</div>
