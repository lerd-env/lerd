// The colours lerd/debug's Color enum offers, as the classes a timeline row and
// a chart draw with. A colour lerd does not know is drawn in slate.
const COLORS: Record<string, { bar: string; text: string; fill: string }> = {
  blue: { bar: 'bg-blue-500', text: 'text-blue-700 dark:text-blue-300', fill: '#3b82f6' },
  indigo: { bar: 'bg-indigo-500', text: 'text-indigo-700 dark:text-indigo-300', fill: '#6366f1' },
  violet: { bar: 'bg-violet-500', text: 'text-violet-700 dark:text-violet-300', fill: '#8b5cf6' },
  pink: { bar: 'bg-pink-500', text: 'text-pink-700 dark:text-pink-300', fill: '#ec4899' },
  rose: { bar: 'bg-rose-500', text: 'text-rose-700 dark:text-rose-300', fill: '#f43f5e' },
  orange: { bar: 'bg-orange-500', text: 'text-orange-700 dark:text-orange-300', fill: '#f97316' },
  amber: { bar: 'bg-amber-500', text: 'text-amber-700 dark:text-amber-300', fill: '#f59e0b' },
  lime: { bar: 'bg-lime-500', text: 'text-lime-700 dark:text-lime-300', fill: '#84cc16' },
  emerald: { bar: 'bg-emerald-500', text: 'text-emerald-700 dark:text-emerald-300', fill: '#10b981' },
  teal: { bar: 'bg-teal-500', text: 'text-teal-700 dark:text-teal-300', fill: '#14b8a6' },
  cyan: { bar: 'bg-cyan-500', text: 'text-cyan-700 dark:text-cyan-300', fill: '#06b6d4' },
  slate: { bar: 'bg-slate-500', text: 'text-slate-700 dark:text-slate-300', fill: '#64748b' }
};
export const PALETTE = Object.keys(COLORS);

export function customColor(name: string | undefined) {
  return COLORS[name ?? ''] ?? COLORS.slate;
}
