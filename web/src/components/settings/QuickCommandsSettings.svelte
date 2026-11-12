<script lang="ts">
  import type { QuickCommand } from "../../contracts/schemas";
  import {
    QUICK_COMMANDS_LIMIT,
    commandLabel,
    commandPreview,
  } from "../../domains/settings/quick-commands";
  import {
    quickCommands,
    saveQuickCommands,
  } from "../../domains/settings/quick-commands.svelte";
  import QuickCommandDialog from "./QuickCommandDialog.svelte";
  import QuickCommandPresetMenu from "./QuickCommandPresetMenu.svelte";
  import {
    settings,
    changeSettings,
    saveSettings,
  } from "../../domains/settings/settings.svelte";
  import { errorMessage } from "../../platform/error";
  import IconArrowDown from "~icons/tabler/arrow-down";
  import IconArrowUp from "~icons/tabler/arrow-up";
  import IconBolt from "~icons/tabler/bolt";
  import IconPencil from "~icons/tabler/pencil";
  import IconPlus from "~icons/tabler/plus";
  import IconTrash from "~icons/tabler/trash";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import { ListButton } from "$lib/components/ui/list-button";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import InfoHint from "../shared/InfoHint.svelte";
  import SettingsRowMenu from "./SettingsRowMenu.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import * as Card from "$lib/components/ui/card";
  import * as Empty from "$lib/components/ui/empty";
  import * as Item from "$lib/components/ui/item";

  let dialogOpen = $state(false);
  let editing = $state<QuickCommand>();
  let deleteOpen = $state(false);
  let clearQuickOpen = $state(false);
  let deleting = $state<QuickCommand>();
  let presetBusy = $state(false);
  let presetError = $state("");
  const full = $derived(quickCommands.list.length >= QUICK_COMMANDS_LIMIT);

  function edit(cmd?: QuickCommand) {
    editing = cmd;
    dialogOpen = true;
  }

  async function upsert(cmd: QuickCommand) {
    const list = quickCommands.list;
    const exists = list.some((item) => item.id === cmd.id);
    if (!exists && list.length >= QUICK_COMMANDS_LIMIT)
      return `最多保存 ${QUICK_COMMANDS_LIMIT} 条快捷指令，请先删除一条后再保存。`;
    await saveQuickCommands(
      exists
        ? list.map((item) => (item.id === cmd.id ? cmd : item))
        : [...list, cmd],
    );
    return "";
  }

  async function addPresets(presets: QuickCommand[]) {
    presetBusy = true;
    presetError = "";
    try {
      await saveQuickCommands([...quickCommands.list, ...presets]);
    } catch (err) {
      presetError = errorMessage(err);
    } finally {
      presetBusy = false;
    }
  }

  function move(index: number, offset: -1 | 1) {
    const list = [...quickCommands.list];
    const from = list[index];
    const to = list[index + offset];
    if (!from || !to) return;
    list[index] = to;
    list[index + offset] = from;
    void saveQuickCommands(list).catch(() => undefined);
  }
</script>

{#snippet presetMenu()}
  <QuickCommandPresetMenu
    commands={quickCommands.list}
    busy={!settings.ready || presetBusy}
    onadd={(presets) => void addPresets(presets)}
  />
{/snippet}

<Card.Root flat id="quick-commands" class="scroll-mt-4">
  <Card.Header>
    <Card.Title>快捷指令</Card.Title>
    <Card.Description
      ><span class="flex items-center gap-1"
        >常用命令一键发送，同步到所有设备。<InfoHint label="快捷指令"
          >点击终端底部工具栏或顶栏的 <IconBolt
            class="inline size-3.5 align-[-2px]"
            aria-label="闪电"
          /> 按钮即可发送；保存到服务端并同步至所有设备。</InfoHint
        ></span
      ></Card.Description
    >
  </Card.Header>
  <Card.Content class="space-y-4">
    {#if quickCommands.list.length === 0}
      <Empty.Root variant="outline">
        <Empty.Header>
          <Empty.Media variant="icon"
            ><IconBolt aria-hidden="true" /></Empty.Media
          >
          <Empty.Title>还没有快捷指令</Empty.Title>
          <Empty.Description
            >把常用命令或提示词存起来，在终端一键发送。</Empty.Description
          >
        </Empty.Header>
        <Empty.Content>
          <div class="flex flex-wrap items-center justify-center gap-2">
            <Button disabled={!settings.ready} onclick={() => edit()}
              ><IconPlus aria-hidden="true" />添加快捷指令</Button
            >
            {@render presetMenu()}
          </div>
        </Empty.Content>
      </Empty.Root>
    {:else}
      <Item.Group variant="divided" aria-label="快捷指令列表">
        {#each quickCommands.list as cmd, index (cmd.id)}
          {@const label = commandLabel(cmd)}
          <Item.Root variant="divided" size="flush" class="min-h-14">
            <ListButton
              variant="content"
              class="flex min-w-0 flex-1 items-center"
              aria-label={`编辑 ${label}`}
              onclick={() => edit(cmd)}
            >
              <span class="flex min-w-0 flex-1 items-center gap-2">
                <span class="min-w-0 truncate text-sm font-medium">{label}</span
                >
                {#if cmd.enter}<Badge variant="secondary" class="shrink-0"
                    >回车</Badge
                  >{/if}
                {#if cmd.name}<span
                    class="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground"
                    >{commandPreview(cmd)}</span
                  >{/if}
              </span>
            </ListButton>
            <SettingsRowMenu {label} disabled={!settings.ready}>
              <DropdownMenu.Item
                aria-label={`编辑 ${label}`}
                onSelect={() => edit(cmd)}
                ><IconPencil aria-hidden="true" />编辑</DropdownMenu.Item
              >
              <DropdownMenu.Item
                aria-label={`上移 ${label}`}
                disabled={index === 0}
                onSelect={() => move(index, -1)}
                ><IconArrowUp aria-hidden="true" />上移</DropdownMenu.Item
              >
              <DropdownMenu.Item
                aria-label={`下移 ${label}`}
                disabled={index === quickCommands.list.length - 1}
                onSelect={() => move(index, 1)}
                ><IconArrowDown aria-hidden="true" />下移</DropdownMenu.Item
              >
              <DropdownMenu.Item
                variant="destructive"
                aria-label={`删除 ${label}`}
                onSelect={() => {
                  deleting = cmd;
                  deleteOpen = true;
                }}><IconTrash aria-hidden="true" />删除</DropdownMenu.Item
              >
            </SettingsRowMenu>
          </Item.Root>
        {/each}
      </Item.Group>
      <div class="flex flex-wrap items-center gap-2">
        <Button
          variant="outline"
          disabled={!settings.ready || full}
          onclick={() => edit()}
          ><IconPlus aria-hidden="true" />添加快捷指令</Button
        >
        {@render presetMenu()}
        {#if full}<span class="text-xs text-muted-foreground"
            >最多 {QUICK_COMMANDS_LIMIT} 条</span
          >{/if}
      </div>
    {/if}
    {#if presetError}
      <InlineNotice tone="danger">
        <p>添加系统预设失败：{presetError}</p>
      </InlineNotice>
    {/if}
    <div class="flex flex-wrap gap-2">
      <Button
        variant="outline"
        size="sm"
        disabled={!settings.ready}
        onclick={() => changeSettings({ quickCommands: null })}
        >恢复预置指令</Button
      >
      <Button
        variant="destructive-outline"
        size="sm"
        disabled={!settings.ready || quickCommands.list.length === 0}
        onclick={() => {
          clearQuickOpen = true;
        }}>删除全部快捷指令</Button
      >
    </div>
  </Card.Content>
</Card.Root>

<QuickCommandDialog
  bind:open={dialogOpen}
  initial={editing}
  existing={quickCommands.list}
  save={upsert}
/>
<AlertDialog.Root bind:open={deleteOpen}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>删除快捷指令？</AlertDialog.Title>
      <AlertDialog.Description
        >将删除“{deleting
          ? commandLabel(deleting)
          : ""}”，此操作不能撤销。</AlertDialog.Description
      >
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel>取消</AlertDialog.Cancel>
      <Button
        variant="destructive"
        onclick={async () => {
          try {
            await saveQuickCommands(
              quickCommands.list.filter((item) => item.id !== deleting?.id),
            );
            deleteOpen = false;
          } catch {
            /* shared settings status retains error */
          }
        }}>删除</Button
      >
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
<AlertDialog.Root bind:open={clearQuickOpen}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>删除全部快捷指令？</AlertDialog.Title>
      <AlertDialog.Description
        >所有设备上的快捷指令列表都会清空。恢复预置指令可重新显示默认列表。</AlertDialog.Description
      >
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel>取消</AlertDialog.Cancel>
      <Button
        variant="destructive"
        disabled={!settings.ready || settings.saving}
        onclick={async () => {
          try {
            await saveSettings({ quickCommands: [] });
            clearQuickOpen = false;
          } catch {
            /* shared status */
          }
        }}>删除全部</Button
      >
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
