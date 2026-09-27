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

// installed drops the product name: an installed app's window prefixes the app
// name on its own, which would otherwise read "Lerd - Dashboard · Lerd".
export function pageTitle(section: string, subject: string, installed = false): string {
  return [subject, section, installed ? '' : 'Lerd'].filter(Boolean).join(' · ');
}
