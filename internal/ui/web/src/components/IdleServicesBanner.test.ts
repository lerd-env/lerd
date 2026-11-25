import { render, screen, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import IdleServicesBanner from './IdleServicesBanner.svelte';
import { accessMode } from '$stores/accessMode';
import { idleEnabled, idleServices } from '$stores/idle';
import { idleSuggestion } from '$stores/idleSuggestion';

const suggestion = { services: ['meilisearch', 'mysql', 'redis'], mem_bytes: 800 * 1024 * 1024 };
let answer: object;
let settings: object;
let saved: object;
let posted: Record<string, unknown>[];
let asked: string[];

beforeEach(() => {
  answer = suggestion;
  settings = { idle_suspend_enabled: true, idle_suspend_timeout_minutes: 45, idle_suspend_services: false };
  saved = { ok: true };
  posted = [];
  asked = [];
  accessMode.update((a) => ({ ...a, localControl: true }));
  idleEnabled.set(true);
  idleServices.set(false);
  idleSuggestion.set({ services: [], mem_bytes: 0 });
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      asked.push(String(url));
      if (init?.method === 'POST') {
        posted.push(JSON.parse(String(init.body)));
        return new Response(JSON.stringify(saved), { status: 200 });
      }
      const body = String(url).includes('services-suggestion') ? answer : settings;
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
    })
  );
});

afterEach(() => vi.unstubAllGlobals());

const settle = () => new Promise((r) => setTimeout(r, 0));
const title = () => screen.queryByText(/3 idle services/);

describe('IdleServicesBanner', () => {
  it('names the services and the memory they hold', async () => {
    render(IdleServicesBanner);
    expect(await screen.findByText(/3 idle services are holding 800 MB/)).toBeInTheDocument();
    expect(screen.getByText(/meilisearch, mysql, redis/)).toBeInTheDocument();
  });

  it('stays hidden when there is nothing to suggest', async () => {
    answer = { services: [], mem_bytes: 0 };
    render(IdleServicesBanner);
    await settle();
    expect(title()).not.toBeInTheDocument();
  });

  it.each([
    ['services already sleep', () => idleServices.set(true)],
    ['idle-suspend is off', () => idleEnabled.set(false)],
    ['this device has no control', () => accessMode.update((a) => ({ ...a, localControl: false }))]
  ])('does not even ask when %s', async (_name, arrange) => {
    arrange();
    render(IdleServicesBanner);
    await settle();
    expect(asked).toEqual([]);
    expect(title()).not.toBeInTheDocument();
  });

  it('turns sleeping services on, keeping the current timeout', async () => {
    render(IdleServicesBanner);
    (await screen.findByRole('button', { name: 'Let them sleep' })).click();
    await waitFor(() => expect(title()).not.toBeInTheDocument());
    expect(posted).toEqual([{ enabled: true, timeout_minutes: 45, services: true }]);
  });

  it('records a dismissal as a choice', async () => {
    render(IdleServicesBanner);
    (await screen.findByRole('button', { name: /Keep services running/ })).click();
    await waitFor(() => expect(title()).not.toBeInTheDocument());
    expect(posted).toEqual([{ enabled: true, timeout_minutes: 45, services: false }]);
  });

  it('does not turn idle-suspend back on from a stale window', async () => {
    render(IdleServicesBanner);
    const dismiss = await screen.findByRole('button', { name: /Keep services running/ });
    settings = { idle_suspend_enabled: false, idle_suspend_timeout_minutes: 45, idle_suspend_services: false };
    dismiss.click();
    await waitFor(() => expect(posted).toEqual([{ enabled: false, timeout_minutes: 45, services: false }]));
  });

  it('stays up when the save fails', async () => {
    // The daemon answers a failed save with a 200.
    saved = { ok: false, error: 'config is read-only' };
    render(IdleServicesBanner);
    (await screen.findByRole('button', { name: 'Let them sleep' })).click();
    await waitFor(() => expect(posted.length).toBe(1));
    await settle();
    expect(title()).toBeInTheDocument();
  });
});
