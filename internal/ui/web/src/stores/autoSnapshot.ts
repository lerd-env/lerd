import { writable } from 'svelte/store';
import { apiJson, apiFetch } from '$lib/api';

// AutoSnapshotSite is one site's database and where it stands with the schedule:
// its own override, whether the policy currently covers it, and when it was last
// snapshotted.
export interface AutoSnapshotSite {
  site: string;
  domain?: string;
  service: string;
  database: string;
  mode: '' | 'on' | 'off';
  covered: boolean;
  last?: string;
  next?: string;
}

export interface AutoSnapshotPolicy {
  enabled: boolean;
  every: string;
  keep: number;
  keep_for: string;
  // opt_out covers every site until one is excluded; opt_in covers none until
  // a site is included. A site's own mode wins either way.
  selection: 'opt_in' | 'opt_out';
  sites: AutoSnapshotSite[];
}

const empty: AutoSnapshotPolicy = {
  enabled: false,
  every: '24h0m0s',
  keep: 7,
  keep_for: '',
  selection: 'opt_in',
  sites: []
};

export const autoSnapshot = writable<AutoSnapshotPolicy>(empty);
export const autoSnapshotLoading = writable(false);

// writes counts the policy writes that have landed. A read that started before
// one of them must not apply its answer: the write already carried the server's
// state, and the older read would put the stale policy back on screen, which
// reads as a toggle that refuses to move.
let writes = 0;

export async function loadAutoSnapshot() {
  const seen = writes;
  autoSnapshotLoading.set(true);
  try {
    const data = await apiJson<AutoSnapshotPolicy>('/api/auto-snapshot');
    if (writes === seen) autoSnapshot.set(data);
  } catch {
    /* keep what is on screen */
  } finally {
    autoSnapshotLoading.set(false);
  }
}

// save posts the whole policy and adopts the server's answer, so the covered
// column reflects the schedule that was actually stored.
async function save(path: string, body: unknown): Promise<{ ok: boolean; error?: string }> {
  try {
    const res = await apiFetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
    const data = (await res.json()) as AutoSnapshotPolicy & { ok?: boolean; error?: string };
    if (data.error) return { ok: false, error: data.error };
    writes++;
    autoSnapshot.set(data);
    return { ok: true };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : 'request failed' };
  }
}

export function saveAutoSnapshot(policy: {
  enabled: boolean;
  every: string;
  keep: number;
  keep_for: string;
  selection: 'opt_in' | 'opt_out';
}) {
  return save('/api/auto-snapshot', policy);
}

// nextSnapshotAt is the soonest scheduled run among the databases the policy
// covers, or null when nothing is covered or none has been taken yet (the first
// run then lands on the watcher's next check rather than a known time).
export function nextSnapshotAt(policy: AutoSnapshotPolicy, service?: string): Date | null {
  if (!policy.enabled) return null;
  const times = policy.sites
    .filter((s) => s.covered && s.next && (!service || s.service === service))
    .map((s) => new Date(s.next as string).getTime())
    .filter((t) => !Number.isNaN(t));
  return times.length > 0 ? new Date(Math.min(...times)) : null;
}

export function setSiteAutoSnapshot(site: string, mode: '' | 'on' | 'off') {
  return save('/api/auto-snapshot/site', { site, mode });
}
