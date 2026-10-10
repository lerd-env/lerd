import { readable } from 'svelte/store';

// DESKTOP_QUERY is Tailwind's md breakpoint, where the dashboard swaps its
// mobile layout for the rail and side panel.
export const DESKTOP_QUERY = '(min-width: 768px)';

// isDesktop tracks that breakpoint so the app mounts one layout. Hiding the
// other with CSS still ran every page twice, each loading its own data.
// Without matchMedia (a test DOM) the desktop layout is the one rendered.
export const isDesktop = readable(true, (set) => {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return;
  const mq = window.matchMedia(DESKTOP_QUERY);
  set(mq.matches);
  const onChange = (e: MediaQueryListEvent) => set(e.matches);
  mq.addEventListener('change', onChange);
  return () => mq.removeEventListener('change', onChange);
});
