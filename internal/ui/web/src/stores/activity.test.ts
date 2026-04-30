import { describe, it, expect } from 'vitest';
import {
  diffSitesEvents,
  diffServicesEvents,
  diffUnhealthyEvents
} from './activity';
import type { Site } from './sites';
import type { Service } from './services';
import type { UnhealthyWorker } from './workerHealth';

function site(domain: string, extra: Partial<Site> = {}): Site {
  return { domain, ...extra };
}

function service(name: string, extra: Partial<Service> = {}): Service {
  return { name, status: 'active', site_count: 0, ...extra };
}

function unhealthy(unit: string, site: string, worker: string): UnhealthyWorker {
  return { unit, site, worker, state: 'failed' };
}

describe('diffSitesEvents', () => {
  it('returns empty when prev is null (initial hydration is silent)', () => {
    expect(diffSitesEvents(null, [site('a.test')])).toEqual([]);
  });

  it('emits site_linked for new domains', () => {
    const prev = new Map<string, Site>([['a.test', site('a.test')]]);
    const events = diffSitesEvents(prev, [site('a.test'), site('b.test')]);
    expect(events).toEqual([{ kind: 'site_linked', subject: 'b.test' }]);
  });

  it('emits site_removed for deleted domains', () => {
    const prev = new Map<string, Site>([
      ['a.test', site('a.test')],
      ['b.test', site('b.test')]
    ]);
    const events = diffSitesEvents(prev, [site('a.test')]);
    expect(events).toEqual([{ kind: 'site_removed', subject: 'b.test' }]);
  });

  it('emits site_paused / site_resumed on pause flag change', () => {
    const prev = new Map<string, Site>([['a.test', site('a.test', { paused: false })]]);
    expect(diffSitesEvents(prev, [site('a.test', { paused: true })])).toEqual([
      { kind: 'site_paused', subject: 'a.test' }
    ]);
    const prev2 = new Map<string, Site>([['a.test', site('a.test', { paused: true })]]);
    expect(diffSitesEvents(prev2, [site('a.test', { paused: false })])).toEqual([
      { kind: 'site_resumed', subject: 'a.test' }
    ]);
  });

  it('emits site_running / site_stopped on fpm flip while not paused', () => {
    const prev = new Map<string, Site>([['a.test', site('a.test', { fpm_running: false })]]);
    expect(diffSitesEvents(prev, [site('a.test', { fpm_running: true })])).toEqual([
      { kind: 'site_running', subject: 'a.test' }
    ]);
  });

  it('does not emit running/stopped when site is paused', () => {
    const prev = new Map<string, Site>([
      ['a.test', site('a.test', { fpm_running: true, paused: false })]
    ]);
    const events = diffSitesEvents(prev, [
      site('a.test', { fpm_running: false, paused: true })
    ]);
    // pause toggle is emitted, but stopped is not (it's a side-effect of the pause)
    expect(events).toEqual([{ kind: 'site_paused', subject: 'a.test' }]);
  });
});

describe('diffServicesEvents', () => {
  it('returns empty when prev is null', () => {
    expect(diffServicesEvents(null, [service('mysql')])).toEqual([]);
  });

  it('emits service_active / service_inactive on status flip', () => {
    const prev = new Map<string, Service>([['mysql', service('mysql', { status: 'inactive' })]]);
    expect(diffServicesEvents(prev, [service('mysql', { status: 'active' })])).toEqual([
      { kind: 'service_active', subject: 'mysql' }
    ]);
  });

  it('emits service_update when update_available goes false → true', () => {
    const prev = new Map<string, Service>([
      ['mysql', service('mysql', { update_available: false })]
    ]);
    const events = diffServicesEvents(prev, [
      service('mysql', { update_available: true, latest_version: '8.5' })
    ]);
    expect(events).toEqual([
      { kind: 'service_update', subject: 'mysql', meta: { version: '8.5' } }
    ]);
  });

  it('emits service_added when a non-worker service appears', () => {
    const prev = new Map<string, Service>();
    expect(diffServicesEvents(prev, [service('mysql')])).toEqual([
      { kind: 'service_added', subject: 'mysql' }
    ]);
  });

  it('does not emit service_added for worker services', () => {
    const prev = new Map<string, Service>();
    const worker = service('queue-foo', { queue_site: 'foo' });
    expect(diffServicesEvents(prev, [worker])).toEqual([]);
  });

  it('emits service_removed when a non-worker service disappears', () => {
    const prev = new Map<string, Service>([['mysql', service('mysql')]]);
    expect(diffServicesEvents(prev, [])).toEqual([
      { kind: 'service_removed', subject: 'mysql' }
    ]);
  });

  it('does not emit service_removed for worker services', () => {
    const prev = new Map<string, Service>([
      ['queue-foo', service('queue-foo', { queue_site: 'foo' })]
    ]);
    expect(diffServicesEvents(prev, [])).toEqual([]);
  });

  it('emits service_version on version bump', () => {
    const prev = new Map<string, Service>([
      ['mysql', service('mysql', { version: 'v8.4' })]
    ]);
    expect(diffServicesEvents(prev, [service('mysql', { version: 'v8.5' })])).toEqual([
      { kind: 'service_version', subject: 'mysql', meta: { version: 'v8.5' } }
    ]);
  });
});

describe('diffUnhealthyEvents', () => {
  it('returns empty when prev is null', () => {
    expect(diffUnhealthyEvents(null, [unhealthy('lerd-queue-foo', 'foo.test', 'queue')])).toEqual([]);
  });

  it('emits worker_failed for new units', () => {
    const prev = new Set<string>();
    const events = diffUnhealthyEvents(prev, [unhealthy('lerd-queue-foo', 'foo.test', 'queue')]);
    expect(events).toEqual([
      { kind: 'worker_failed', subject: 'foo.test', meta: { worker: 'queue' } }
    ]);
  });

  it('emits worker_healed when a unit drops out', () => {
    const prev = new Set<string>(['lerd-queue-foo']);
    const events = diffUnhealthyEvents(prev, []);
    expect(events).toEqual([{ kind: 'worker_healed', subject: 'lerd-queue-foo' }]);
  });

  it('emits nothing when set is unchanged', () => {
    const prev = new Set<string>(['lerd-queue-foo']);
    const events = diffUnhealthyEvents(prev, [unhealthy('lerd-queue-foo', 'foo.test', 'queue')]);
    expect(events).toEqual([]);
  });
});
