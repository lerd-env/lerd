const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function focusables(node: HTMLElement): HTMLElement[] {
  return [...node.querySelectorAll<HTMLElement>(FOCUSABLE)];
}

// Keeps keyboard focus inside an open modal: focus moves in on open, Tab wraps
// at either end instead of wandering into the page behind the backdrop, and the
// control that opened it gets focus back on close.
export function dialog(node: HTMLElement) {
  const opener = document.activeElement as HTMLElement | null;
  // A form modal starts in its first field rather than on the header's close button.
  if (!node.contains(document.activeElement)) {
    const items = focusables(node);
    (items.find((el) => el.matches('input, select, textarea')) ?? items[0] ?? node).focus();
  }

  function onKey(e: KeyboardEvent) {
    if (e.key !== 'Tab') return;
    const items = focusables(node);
    if (items.length === 0) return;
    const first = items[0];
    const last = items[items.length - 1];
    const wrapTo = e.shiftKey ? (document.activeElement === first ? last : null) : document.activeElement === last ? first : null;
    if (!wrapTo) return;
    e.preventDefault();
    wrapTo.focus();
  }

  node.addEventListener('keydown', onKey);
  return {
    destroy() {
      node.removeEventListener('keydown', onKey);
      if (opener?.isConnected) opener.focus();
    }
  };
}
