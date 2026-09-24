import type { Action } from 'svelte/action';

// scrollbarInPadding makes a scroll area's bar live in space that already
// exists, so showing it never adds a gap. A classic bar's size is only known at
// runtime (0 for macOS overlay bars, and 0 while the area is hidden), so it is
// re-measured on every resize, always from the padding the area started with:
// a vertical bar comes out of the right padding, a horizontal one hands its
// height and the row's bottom padding back to whatever sits below.
export const scrollbarInPadding: Action<HTMLElement, 'x' | 'y'> = (node, axis = 'y') => {
  const style = getComputedStyle(node);
  const px = (v: string) => parseFloat(v || '0');
  if (axis === 'y') node.style.scrollbarGutter = 'stable';
  else node.style.overflowX = 'scroll';
  const basePadding = px(axis === 'y' ? style.paddingRight : style.paddingBottom);

  function apply() {
    if (axis === 'y') {
      const bar = node.offsetWidth - node.clientWidth - px(style.borderLeftWidth) - px(style.borderRightWidth);
      node.style.paddingRight = `${Math.max(0, basePadding - bar)}px`;
    } else {
      const bar = node.offsetHeight - node.clientHeight - px(style.borderTopWidth) - px(style.borderBottomWidth);
      node.style.marginBottom = `-${bar + basePadding}px`;
    }
  }

  apply();
  if (typeof ResizeObserver === 'undefined') return;
  const observer = new ResizeObserver(apply);
  observer.observe(node);
  return { destroy: () => observer.disconnect() };
};
