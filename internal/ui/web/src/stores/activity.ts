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
  | 'worker_healed';

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

// `now` ticks every 30s so relative timestamps re-render without per-event timers.
export const now = writable<number>(Date.now());
if (typeof window !== 'undefined') {
  setInterval(() => now.set(Date.now()), 30000);
}

type RawEvent = Pick<ActivityEvent, 'kind' | 'subject'> & { meta?: Record<string, string> };

// Pure diff helpers — kept side-effect free so tests can drive them with
// fixture data without touching stores or the WebSocket layer.

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
});

// Test-only helper: reset the in-memory state so each test starts clean.
export function _resetActivityForTest() {
  prevSitesMap = null;
  prevServicesMap = null;
  prevUnhealthySet = null;
  activity.set([]);
  counter = 0;
}
