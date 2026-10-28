<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";

  // D21: the interrupt key did not stop the terminal's turn within 10
  // seconds. Every device sees the question; the first answer applies.
  let {
    open = $bindable(false),
    agent,
    busy,
    onchoose,
  }: {
    open?: boolean;
    agent: string;
    busy: boolean;
    onchoose: (choice: "wait" | "force" | "cancel") => void;
  } = $props();
</script>

<AlertDialog.Root bind:open>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>TUI 仍在执行回合</AlertDialog.Title>
      <AlertDialog.Description>
        已发送中断键，10 秒内 TUI 仍未停下。
      </AlertDialog.Description>
    </AlertDialog.Header>
    <ul class="space-y-2 text-sm text-muted-foreground">
      <li><strong class="text-foreground">继续等待</strong>：再等待 10 秒。</li>
      <li>
        <strong class="text-foreground">强制切换</strong>：直接结束
        TUI。被强制结束的回合可能不完整，{agent === "omp"
          ? "omp 已输出的部分可能丢失"
          : "已输出的部分可能丢失；回合不会在后台继续"}。
      </li>
      <li>
        <strong class="text-foreground">取消</strong>：留在终端，TUI 继续运行。
      </li>
    </ul>
    <AlertDialog.Footer>
      <Button
        variant="outline"
        disabled={busy}
        onclick={() => onchoose("cancel")}>取消</Button
      >
      <Button
        variant="destructive"
        disabled={busy}
        onclick={() => onchoose("force")}>强制切换</Button
      >
      <Button disabled={busy} onclick={() => onchoose("wait")}>继续等待</Button>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
