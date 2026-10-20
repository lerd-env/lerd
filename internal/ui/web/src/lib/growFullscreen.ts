import { tick } from 'svelte';

// growFullscreen animates a panel between its page layout and full screen.
// apply switches the state; the panel stays fixed only for the 200ms between.
export async function growFullscreen(node: HTMLElement | undefined, apply: () => void): Promise<void> {
  const still = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
  if (!node || typeof node.animate !== 'function' || still) {
    apply();
    return;
  }
  // A click mid-flight starts from wherever the panel is now. The old run's
  // cancel event fires later, so its hold on the panel is let go here, before
  // the far end is measured.
  const first = frame(node);
  const firstBg = getComputedStyle(node).backgroundColor;
  const prev = running.get(node);
  if (prev) {
    running.delete(node);
    prev.cancel();
    release(node);
  }
  apply();
  await tick();
  const last = frame(node);
  // The overlay's opaque background, whichever end it is on, so the page never
  // shows through the panel while it moves.
  const lastBg = getComputedStyle(node).backgroundColor;
  const bg = opaque(lastBg) ? lastBg : firstBg;
  Object.assign(node.style, { position: 'fixed', zIndex: '50', margin: '0', backgroundColor: bg });
  const anim = node.animate([first, last], { duration: 200, easing: 'ease-out' });
  running.set(node, anim);
  const done = () => {
    if (running.get(node) !== anim) return;
    running.delete(node);
    release(node);
  };
  anim.onfinish = done;
  anim.oncancel = done;
}

const running = new WeakMap<HTMLElement, Animation>();

function release(node: HTMLElement) {
  for (const p of ['position', 'zIndex', 'margin', 'backgroundColor'] as const) node.style[p] = '';
}

// frame is the box and padding to animate between; padding moves with the box
// so the content does not jump when the two ends are padded differently.
function frame(node: HTMLElement): Keyframe {
  const r = node.getBoundingClientRect();
  const cs = getComputedStyle(node);
  return { top: `${r.top}px`, left: `${r.left}px`, width: `${r.width}px`, height: `${r.height}px`, padding: cs.padding };
}

function opaque(color: string): boolean {
  return color !== '' && color !== 'transparent' && !/rgba\(.*,\s*0\)$/.test(color);
}
