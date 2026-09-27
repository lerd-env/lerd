// The WAI tabs pattern for a bar of role="tab" buttons: arrow keys, Home and
// End move between tabs. Moving clicks the tab, so each bar keeps its own
// change handler, and then focuses it.
export function tablist(node: HTMLElement) {
  function onKey(e: KeyboardEvent) {
    const tabs = [...node.querySelectorAll<HTMLElement>('[role="tab"]')];
    const i = tabs.indexOf(e.target as HTMLElement);
    if (i < 0) return;
    const n = tabs.length;
    const next =
      e.key === 'ArrowRight' ? (i + 1) % n
      : e.key === 'ArrowLeft' ? (i - 1 + n) % n
      : e.key === 'Home' ? 0
      : e.key === 'End' ? n - 1
      : -1;
    if (next < 0) return;
    e.preventDefault();
    tabs[next].click();
    tabs[next].focus();
  }
  node.addEventListener('keydown', onKey);
  return {
    destroy() {
      node.removeEventListener('keydown', onKey);
    }
  };
}
