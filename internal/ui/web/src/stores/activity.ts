import { writable } from 'svelte/store';
import { wsMessage } from '$lib/ws';
import type { Site } from './sites';
import { isServiceWorker, type Service } from './services';
import type { UnhealthyWorker } from './workerHealth';

export type ActivityKind =
  | 'site_linked'
  | 'site_removed'
  | 'site_paused'
  | 'site_resumed'
  | 'site_running'
  | 'site_stopped'
  | 'service_added'
  | 'service_removed'
  | 'service_active'
  | 'service_inactive'
  | 'service_update'
  | 'service_version'
  | 'worker_failed'
  | 'worker_healed'
  | 'worker_slept'
  | 'worker_woke'
  | 'dns_degraded'
  | 'dns_down'
  | 'dns_recovered';

export interface DNSSnapshot {
  status: 'ok' | 'degraded' | 'down';
  vpn: boolean;
}

export interface ActivityEvent {
  id: string;
  kind: ActivityKind;
  subject: string;
  meta?: Record<string, string>;
  at: number;
}

const MAX = 30;
let counter = 0;
function nextId(): string {
  return Date.now().toString(36) + '-' + (++counter).toString(36);
}

export const activity = writable<ActivityEvent[]>([]);

// `now` ticks every 30s so relative timestamps re-render without per-event
// timers. The interval is armed by the first subscriber and cleared when the
// last one goes away, so it runs only while a view showing relative times is
// mounted rather than for the lifetime of the page.
export const now = writable<number>(Date.now(), (set) => {
  if (typeof window === 'undefined') return;
  const id = setInterval(() => set(Date.now()), 30000);
  return () => clearInterval(id);
});

type RawEvent = Pick<ActivityEvent, 'kind' | 'subject'> & { meta?: Record<string, string> };

// Pure diff helpers — kept side-effect free so tests can drive them with
// fixture data without touching stores or the WebSocket layer.

// suspendedWorkerCount totals a site's idle-suspended workers across the main
// checkout and every worktree, so the activity diff can emit a single per-site
// sleep/wake event rather than one per worker.
function suspendedWorkerCount(s: Site): number {
  let n = (s.idle_suspended_workers || []).length;
  for (const wt of s.worktrees || []) n += (wt.idle_suspended_workers || []).length;
  return n;
}

export function diffSitesEvents(prev: Map<string, Site> | null, current: Site[]): RawEvent[] {
  const out: RawEvent[] = [];
  const cur = new Map(current.map((s) => [s.domain, s]));
  if (!prev) return out;
  for (const [domain, s] of cur) {
    const old = prev.get(domain);
    if (!old) {
      out.push({ kind: 'site_linked', subject: domain });
      continue;
    }
    if (Boolean(old.paused) !== Boolean(s.paused)) {
      out.push({ kind: s.paused ? 'site_paused' : 'site_resumed', subject: domain });
    }
    if (Boolean(old.fpm_running) !== Boolean(s.fpm_running) && !s.paused) {
      out.push({ kind: s.fpm_running ? 'site_running' : 'site_stopped', subject: domain });
    }
    // One event when a site's workers first go to sleep and one when they all
    // wake, mirroring the per-site granularity of pause/resume so a multi-worker
    // site doesn't flood the timeline.
    const wasAsleep = suspendedWorkerCount(old);
    const nowAsleep = suspendedWorkerCount(s);
    if (wasAsleep === 0 && nowAsleep > 0) {
      out.push({ kind: 'worker_slept', subject: domain });
    } else if (wasAsleep > 0 && nowAsleep === 0) {
      out.push({ kind: 'worker_woke', subject: domain });
    }
  }
  for (const [domain] of prev) {
    if (!cur.has(domain)) out.push({ kind: 'site_removed', subject: domain });
  }
  return out;
}

export function diffServicesEvents(
  prev: Map<string, Service> | null,
  current: Service[]
): RawEvent[] {
  const out: RawEvent[] = [];
  if (!prev) return out;
  const cur = new Map(current.map((s) => [s.name, s]));
  for (const [name, s] of cur) {
    const old = prev.get(name);
    if (!old) {
      // New service. Skip workers — those come and go as side-effects of
      // site state changes and would flood the timeline. Core services
      // (mysql, redis, mailpit, etc.) are user-driven adds.
      if (!isServiceWorker(s)) {
        out.push({ kind: 'service_added', subject: name });
      }
      continue;
    }
    if (old.status !== s.status) {
      out.push({
        kind: s.status === 'active' ? 'service_active' : 'service_inactive',
        subject: name
      });
    }
    if (!old.update_available && s.update_available) {
      out.push({
        kind: 'service_update',
        subject: name,
        meta: s.latest_version ? { version: s.latest_version } : undefined
      });
    }
    if (old.version && s.version && old.version !== s.version) {
      out.push({ kind: 'service_version', subject: name, meta: { version: s.version } });
    }
  }
  for (const [name, old] of prev) {
    if (cur.has(name)) continue;
    if (isServiceWorker(old)) continue;
    out.push({ kind: 'service_removed', subject: name });
  }
  return out;
}

// diffDNSEvents emits one event per status transition (ok → degraded,
// degraded → down, anything → ok, etc.). The vpn flag alone changing
// under a steady degraded state does not emit, that's the same outcome
// from the user's perspective. The vpn=true case is forwarded as meta
// so the label can mention "VPN active".
export function diffDNSEvents(prev: DNSSnapshot | null, current: DNSSnapshot): RawEvent[] {
  if (!prev) return [];
  if (prev.status === current.status) return [];
  switch (current.status) {
    case 'ok':
      return [{ kind: 'dns_recovered', subject: 'DNS' }];
    case 'down':
      return [{ kind: 'dns_down', subject: 'DNS' }];
    case 'degraded':
      return current.vpn
        ? [{ kind: 'dns_degraded', subject: 'DNS', meta: { vpn: '1' } }]
        : [{ kind: 'dns_degraded', subject: 'DNS' }];
  }
}

export function diffUnhealthyEvents(
  prev: Set<string> | null,
  current: UnhealthyWorker[]
): RawEvent[] {
  const out: RawEvent[] = [];
  if (!prev) return out;
  const cur = new Set(current.map((u) => u.unit));
  for (const u of current) {
    if (!prev.has(u.unit)) {
      out.push({ kind: 'worker_failed', subject: u.site, meta: { worker: u.worker } });
    }
  }
  for (const unit of prev) {
    if (!cur.has(unit)) out.push({ kind: 'worker_healed', subject: unit });
  }
  return out;
}

function pushAll(raw: RawEvent[]) {
  if (raw.length === 0) return;
  const stamped: ActivityEvent[] = raw.map((e) => ({ ...e, id: nextId(), at: Date.now() }));
  activity.update((list) => {
    const next = [...stamped.reverse(), ...list];
    if (next.length > MAX) next.length = MAX;
    return next;
  });
}

let prevSitesMap: Map<string, Site> | null = null;
let prevServicesMap: Map<string, Service> | null = null;
let prevUnhealthySet: Set<string> | null = null;
let prevDNS: DNSSnapshot | null = null;

wsMessage.subscribe((msg) => {
  if (!msg) return;
  if (Array.isArray(msg.sites)) {
    const list = msg.sites as Site[];
    pushAll(diffSitesEvents(prevSitesMap, list));
    prevSitesMap = new Map(list.map((s) => [s.domain, s]));
  }
  if (Array.isArray(msg.services)) {
    const list = msg.services as Service[];
    pushAll(diffServicesEvents(prevServicesMap, list));
    prevServicesMap = new Map(list.map((s) => [s.name, s]));
  }
  if (Array.isArray(msg.unhealthy_workers)) {
    const list = msg.unhealthy_workers as UnhealthyWorker[];
    pushAll(diffUnhealthyEvents(prevUnhealthySet, list));
    prevUnhealthySet = new Set(list.map((u) => u.unit));
  }
  if (msg.status && typeof msg.status === 'object') {
    const dns = (msg.status as { dns?: Partial<DNSSnapshot> }).dns;
    if (dns && typeof dns.status === 'string') {
      const snap: DNSSnapshot = { status: dns.status, vpn: Boolean(dns.vpn) };
      pushAll(diffDNSEvents(prevDNS, snap));
      prevDNS = snap;
    }
  }
});

// Test-only helper: reset the in-memory state so each test starts clean.
export function _resetActivityForTest() {
  prevSitesMap = null;
  prevServicesMap = null;
  prevUnhealthySet = null;
  prevDNS = null;
  activity.set([]);
  counter = 0;
}
