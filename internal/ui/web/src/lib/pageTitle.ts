import type { TabId } from '$stores/route';
import { serviceLabel } from '$stores/services';

// pageSubject is the thing the open view is about, for the tab title and the
// screen-reader heading. System items are left out: their names live in markup.
export function pageSubject(tab: TabId, rest: string): string {
  const id = rest.split('/')[0];
  if (!id) return '';
  if (tab === 'sites') return id;
  if (tab === 'services') return serviceLabel(id);
  return '';
}

// An app window leads with the product name: an installed app's window then
// leaves its own prefix off, and a --app window, which never adds one, still
// names Lerd.
export function pageTitle(section: string, subject: string, installed = false): string {
  const parts = [subject, section].filter(Boolean);
  return (installed ? ['Lerd', ...parts] : [...parts, 'Lerd']).join(' · ');
}
