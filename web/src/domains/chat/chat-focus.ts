const EDITABLE_SELECTOR =
  'textarea, input, select, [contenteditable]:not([contenteditable="false"])';
const OPEN_PORTAL_SELECTOR =
  '[data-slot="popover-content"][data-state="open"], [data-slot="dropdown-menu-content"][data-state="open"]';
export function isEditableElement(node: Node | null): node is HTMLElement {
  return node instanceof HTMLElement && node.matches(EDITABLE_SELECTOR);
}
export function hasOpenPortal(): boolean {
  return !!document.querySelector(OPEN_PORTAL_SELECTOR);
}
