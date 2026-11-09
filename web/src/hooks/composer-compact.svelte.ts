// Tracks whether a chat composer container has active focus (including focus
// moved into a portaled popover, menu or drawer opened from the composer) and
// whether it can collapse into its mobile single-line bar when idle.
export function useComposerCompact(
  getContainer: () => HTMLElement | null,
  isBusy: () => boolean,
) {
  let expanded = $state(false);
  let pointerInControl = false;
  const compact = $derived(!expanded && !isBusy());

  function hasOpenPortal(): boolean {
    return !!document.querySelector(
      '[data-slot="popover-content"][data-state="open"], [data-slot="dropdown-menu-content"][data-state="open"], [data-slot="drawer-content"][data-state="open"]',
    );
  }

  function isPortalTarget(target: Node | null): boolean {
    return (
      target instanceof Element &&
      !!target.closest(
        '[data-slot="popover-content"], [data-slot="dropdown-menu-content"], [data-slot="drawer-content"]',
      )
    );
  }

  function checkFocus() {
    requestAnimationFrame(() => {
      if (pointerInControl) {
        pointerInControl = false;
        return;
      }
      const container = getContainer();
      if (container?.contains(document.activeElement) || hasOpenPortal()) {
        return;
      }
      expanded = false;
    });
  }

  $effect(() => {
    if (!expanded) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    function onPointerDown(event: PointerEvent) {
      const target = event.target as Node | null;
      const container = getContainer();
      if (target instanceof HTMLTextAreaElement) {
        pointerInControl = false;
        return;
      }
      if ((target && container?.contains(target)) || isPortalTarget(target)) {
        pointerInControl = true;
        clearTimeout(timer);
        timer = setTimeout(() => {
          pointerInControl = false;
        }, 100);
        return;
      }
      pointerInControl = false;
      checkFocus();
    }
    window.addEventListener("pointerdown", onPointerDown, true);
    return () => {
      clearTimeout(timer);
      pointerInControl = false;
      window.removeEventListener("pointerdown", onPointerDown, true);
    };
  });

  return {
    get compact() {
      return compact;
    },
    containerProps: {
      onfocusin: (event: FocusEvent) => {
        if (event.target instanceof HTMLTextAreaElement) {
          expanded = true;
        }
      },
      onfocusout: (event: FocusEvent) => {
        if (pointerInControl) {
          pointerInControl = false;
          return;
        }
        const container = getContainer();
        const related = event.relatedTarget as Node | null;
        if (related && container?.contains(related)) return;
        if (!related && container?.contains(document.activeElement)) return;
        checkFocus();
      },
    },
  };
}
