<script lang="ts">
  import { ListButton } from "$lib/components/ui/list-button";
  import type { SessionInfo } from "../../contracts/schemas";
  import {
    rowStatus,
    sessionRowLabel,
    sessionRowTitle,
  } from "../../domains/sessions/session-status";
  import LaunchConfigIcon from "../settings/LaunchConfigIcon.svelte";
  import RowStatus from "../shared/RowStatus.svelte";
  import SidebarSessionMenu from "./SidebarSessionMenu.svelte";

  let {
    session,
    current,
    icon,
    pinned = false,
    onopen,
    onrename,
    onclose,
  }: {
    session: SessionInfo;
    current: string;
    icon: string;
    pinned?: boolean;
    onopen: (session: SessionInfo) => void;
    onrename: (session: SessionInfo) => void;
    onclose: (session: SessionInfo) => void;
  } = $props();
</script>

<li class="group/row relative">
  <ListButton
    variant="navigation-action"
    aria-current={session.id === current ? "page" : undefined}
    aria-label={pinned
      ? `置顶 · ${sessionRowLabel(session)}`
      : sessionRowLabel(session)}
    title={sessionRowTitle(session)}
    onclick={() => onopen(session)}
  >
    <LaunchConfigIcon {icon} size="sm" />
    <span class="min-w-0 flex-1 truncate">{session.title}</span>
    <RowStatus status={rowStatus(session)} />
  </ListButton>
  <SidebarSessionMenu
    label={pinned ? `置顶 · ${session.title}` : session.title}
    target={{ kind: "live", session, onrename, onclose }}
  />
</li>
