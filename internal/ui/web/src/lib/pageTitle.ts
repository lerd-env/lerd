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

export function pageTitle(section: string, subject: string): string {
  return [subject, section, 'Lerd'].filter(Boolean).join(' · ');
}
