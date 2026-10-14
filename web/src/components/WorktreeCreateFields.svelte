<script lang="ts">
  import { api } from "../api";
  import { BranchesSchema, type Repository } from "../schemas";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import type { z } from "zod";
  let {
    repository,
    path = $bindable(""),
    mode = $bindable<"new" | "existing">("new"),
    branch = $bindable(""),
    base = $bindable(""),
    ready = $bindable(false),
  }: {
    repository: Repository;
    path: string;
    mode: "new" | "existing";
    branch: string;
    base: string;
    ready: boolean;
  } = $props();
  let data = $state<z.infer<typeof BranchesSchema> | null>(null);
  let error = $state("");
  let loading = $state(false);
  let pathEdited = $state(false);
  $effect(() => {
    const id = repository.id;
    let active = true;
    data = null;
    error = "";
    loading = true;
    pathEdited = false;
    branch = "";
    base = "";
    void api(`/repositories/${id}/branches`, BranchesSchema)
      .then((value) => {
        if (active) {
          data = value;
          base = value.defaultBase || "";
        }
      })
      .catch((err) => {
        if (active) error = (err as Error).message;
      })
      .finally(() => {
        if (active) loading = false;
      });
    return () => {
      active = false;
    };
  });
  $effect(() => {
    if (!pathEdited)
      path = data
        ? `${data.defaultParent}/${branch.split("/").at(-1) || ""}`
        : "";
  });
  $effect(() => {
    ready =
      !!data &&
      !loading &&
      !!branch &&
      !!path &&
      (mode === "existing" || !!base);
  });
</script>

<div class="space-y-4 rounded-lg border p-3">
  <fieldset class="space-y-2">
    <legend class="mb-2 text-sm font-medium">工作树分支</legend>
    <div class="flex flex-wrap gap-2">
      <Button
        type="button"
        variant={mode === "new" ? "default" : "outline"}
        aria-pressed={mode === "new"}
        onclick={() => {
          mode = "new";
          branch = "";
        }}>创建新分支</Button
      >
      <Button
        type="button"
        variant={mode === "existing" ? "default" : "outline"}
        aria-pressed={mode === "existing"}
        onclick={() => {
          mode = "existing";
          branch = "";
        }}>使用已有分支</Button
      >
    </div>
  </fieldset>
  {#if loading}<p role="status" class="text-sm">正在读取本地分支…</p>{/if}
  {#if error}<p role="alert" class="break-all text-sm text-destructive">
      {error}
    </p>{/if}
  {#if mode === "new"}
    <div class="space-y-2">
      <Label for="worktree-branch">新分支名称</Label><Input
        id="worktree-branch"
        bind:value={branch}
        required
        placeholder="feature/my-task"
        autocomplete="off"
        autocapitalize="off"
        autocorrect="off"
        spellcheck={false}
      />
    </div>
    <div class="space-y-2">
      <Label for="worktree-base">起始基线</Label>
      <Input
        id="worktree-base"
        bind:value={base}
        list="worktree-bases"
        required
        placeholder="本地分支、远程跟踪引用或提交 OID"
        autocomplete="off"
        autocapitalize="off"
        autocorrect="off"
        spellcheck={false}
      />
      <datalist id="worktree-bases"
        >{#each data?.bases || [] as item}<option value={item.name}
            >{item.head.slice(0, 12)}</option
          >{/each}</datalist
      >
      {#if data && !data.bases.length}<p class="text-sm text-muted-foreground">
          尚无可用基线。请先在终端创建提交；仍可在主工作区启动会话。
        </p>{/if}
    </div>
  {:else}
    <div class="space-y-2">
      <Label for="existing-branch">已有本地分支</Label>
      <select
        id="existing-branch"
        bind:value={branch}
        required
        class="h-10 w-full min-w-0 rounded-md border bg-background px-3 text-sm"
      >
        <option value="">选择未被占用的分支</option>
        {#each data?.branches || [] as item}<option
            value={item.name}
            disabled={item.worktreePaths.length > 0}
            >{item.name}{item.worktreePaths.length
              ? "（已被工作树占用）"
              : ""}</option
          >{/each}
      </select>
    </div>
  {/if}
  <div class="space-y-2">
    <Label for="worktree-path">新工作树完整路径</Label>
    <Input
      id="worktree-path"
      bind:value={path}
      oninput={() => {
        pathEdited = true;
      }}
      required
      autocomplete="off"
      autocapitalize="off"
      autocorrect="off"
      spellcheck={false}
    />
    <p
      class="font-mono text-xs break-all whitespace-pre-wrap text-muted-foreground"
    >
      {path}
    </p>
    <p class="text-xs text-muted-foreground">
      默认放在仓库旁，目录名取分支最后一段；可以修改完整路径，不覆盖已有目录。
    </p>
  </div>
  <p class="text-xs text-muted-foreground">
    仅检出版本控制内容；不会 fetch、复制 .env
    或安装依赖。创建成功后即使命令启动失败，工作树与分支仍保留。
  </p>
</div>
