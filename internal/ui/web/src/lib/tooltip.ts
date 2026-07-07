import type { Action } from 'svelte/action';

export type TooltipPlacement = 'bottom' | 'right';

export interface TooltipOptions {
  label: string;
  placement?: TooltipPlacement;
}

type TooltipParam = string | TooltipOptions | null | undefined;

const GAP = 8;
const MARGIN = 6;
const ARROW = 8;

// One shared, body-level node reused by every trigger. Fixed positioning plus a
// high z-index lets it escape the overflow-x-auto control rows and the panel
// stacking that clip an in-flow tooltip.
let box: HTMLDivElement | null = null;
let arrowEl: HTMLSpanElement | null = null;
let textEl: HTMLSpanElement | null = null;
let owner: HTMLElement | null = null;

function ensure(): HTMLDivElement {
  if (box) {
    if (!box.isConnected) document.body.appendChild(box);
    return box;
  }
  box = document.createElement('div');
  box.setAttribute('role', 'tooltip');
  box.className =
    'pointer-events-none whitespace-nowrap rounded-md border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card px-2 py-1 text-xs text-gray-800 dark:text-gray-100 shadow-lg transition-opacity duration-100';
  box.style.position = 'fixed';
  box.style.zIndex = '9999';
  box.style.opacity = '0';
  arrowEl = document.createElement('span');
  arrowEl.style.position = 'absolute';
  box.appendChild(arrowEl);
  textEl = document.createElement('span');
  box.appendChild(textEl);
  document.body.appendChild(box);
  return box;
}

function hide(node: HTMLElement) {
  if (owner !== node) return;
  owner = null;
  if (box) box.style.opacity = '0';
}

function place(node: HTMLElement, label: string, placement: TooltipPlacement) {
  const b = ensure();
  textEl!.textContent = label;
  arrowEl!.className =
    'h-2 w-2 rotate-45 border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card ' +
    (placement === 'right' ? 'border-b border-l' : 'border-t border-l');
  b.style.opacity = '0';
  b.style.left = '0px';
  b.style.top = '0px';
  const r = node.getBoundingClientRect();
  const bw = b.offsetWidth;
  const bh = b.offsetHeight;
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  if (placement === 'right') {
    const left = r.right + GAP;
    const top = Math.max(MARGIN, Math.min(r.top + r.height / 2 - bh / 2, vh - bh - MARGIN));
    b.style.left = left + 'px';
    b.style.top = top + 'px';
    arrowEl!.style.left = -ARROW / 2 + 'px';
    arrowEl!.style.top = r.top + r.height / 2 - top - ARROW / 2 + 'px';
  } else {
    const left = Math.max(MARGIN, Math.min(r.left + r.width / 2 - bw / 2, vw - bw - MARGIN));
    b.style.left = left + 'px';
    b.style.top = r.bottom + GAP + 'px';
    arrowEl!.style.top = -ARROW / 2 + 'px';
    arrowEl!.style.left = r.left + r.width / 2 - left - ARROW / 2 + 'px';
  }
  b.style.opacity = '1';
}

function normalize(p: TooltipParam): TooltipOptions {
  if (p == null) return { label: '' };
  return typeof p === 'string' ? { label: p } : p;
}

// use:tooltip={label} or use:tooltip={{ label, placement }} on any element.
export const tooltip: Action<HTMLElement, TooltipParam> = (node, param) => {
  let opts = normalize(param);
  const show = () => {
    if (!opts.label) return;
    owner = node;
    place(node, opts.label, opts.placement ?? 'bottom');
  };
  const leave = () => hide(node);
  node.addEventListener('mouseenter', show);
  node.addEventListener('mouseleave', leave);
  node.addEventListener('click', leave);
  node.addEventListener('focusin', show);
  node.addEventListener('focusout', leave);
  window.addEventListener('scroll', leave, true);
  window.addEventListener('resize', leave);
  return {
    update(p: TooltipParam) {
      opts = normalize(p);
      if (owner === node) {
        if (opts.label) place(node, opts.label, opts.placement ?? 'bottom');
        else hide(node);
      }
    },
    destroy() {
      node.removeEventListener('mouseenter', show);
      node.removeEventListener('mouseleave', leave);
      node.removeEventListener('click', leave);
      node.removeEventListener('focusin', show);
      node.removeEventListener('focusout', leave);
      window.removeEventListener('scroll', leave, true);
      window.removeEventListener('resize', leave);
      hide(node);
    }
  };
};
