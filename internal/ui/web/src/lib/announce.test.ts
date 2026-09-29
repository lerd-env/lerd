import { describe, it, expect } from 'vitest';
import { get } from 'svelte/store';
import { announce, announcement, serviceStatusChanges } from './announce';

const svc = (name: string, status: string, extra: Record<string, unknown> = {}) => ({ name, status, ...extra });

describe('serviceStatusChanges', () => {
  it('reports a service that stopped or started', () => {
    const before = [svc('redis', 'active'), svc('mysql', 'inactive')];
    const after = [svc('redis', 'inactive'), svc('mysql', 'active')];
    expect(serviceStatusChanges(before, after)).toEqual([
      { name: 'redis', running: false },
      { name: 'mysql', running: true }
    ]);
  });

  it('stays quiet when nothing changed', () => {
    const list = [svc('redis', 'active')];
    expect(serviceStatusChanges(list, list)).toEqual([]);
  });

  it('ignores a service that just appeared, so the first load is not read out', () => {
    expect(serviceStatusChanges([], [svc('redis', 'active')])).toEqual([]);
  });

  it('ignores workers, which idle-suspend flips on and off all day', () => {
    const before = [svc('lerd-queue-app', 'active', { site_domain: 'app.test' })];
    const after = [svc('lerd-queue-app', 'inactive', { site_domain: 'app.test' })];
    expect(serviceStatusChanges(before, after, (s) => s.name.startsWith('lerd-queue'))).toEqual([]);
  });
});

describe('announce', () => {
  it('puts the text in the live region', () => {
    announce('Redis stopped');
    expect(get(announcement)).toBe('Redis stopped');
  });
});
