import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

const realFetch = globalThis.fetch;
import { get } from 'svelte/store';

// The setup state lives in the store module, so each test gets fresh modules.
async function load(
  done: { site?: boolean; service?: boolean; notify?: boolean; open?: boolean; idle?: boolean } = {},
  localControl = true,
  configSetup = ''
) {
  vi.resetModules();
  const fetchMock = vi.fn(async (_: RequestInfo | URL, init?: RequestInit) =>
    init?.method === 'POST'
      ? new Response('{"ok":true}', { status: 200 })
      : new Response(JSON.stringify({ setup: configSetup }), { status: 200, headers: { 'Content-Type': 'application/json' } })
  );
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  const setup = await import('$stores/setup');
  await setup.loadSetup();
  const { sites, sitesLoaded } = await import('$stores/sites');
  const { services } = await import('$stores/services');
  const { status } = await import('$stores/status');
  const { startOnDashboardOpen } = await import('$stores/autostart');
  const { idleEnabled } = await import('$stores/idle');
  const { accessMode } = await import('$stores/accessMode');
  const { permissionState, notifyDelivery } = await import('$lib/notify');
  permissionState.set(done.notify ? 'granted' : 'default');
  notifyDelivery.set('browser');
  sites.set(done.site ? [{ name: 'a' } as never] : []);
  sitesLoaded.set(true);
  services.set(done.service ? [{ name: 'mysql', status: 'active' } as never] : []);
  status.set({ streaming_mode: false } as never);
  startOnDashboardOpen.set(!!done.open);
  idleEnabled.set(!!done.idle);
  accessMode.update((a) => ({ ...a, localControl }));
  const { default: SetupCard } = await import('./SetupCard.svelte');
  // render comes from the same fresh module graph, or it drives a second Svelte runtime.
  const { render, screen, fireEvent } = await import('@testing-library/svelte');
  return { ...setup, idleEnabled, SetupCard, render, screen, fireEvent, fetchMock };
}

describe('SetupCard', () => {
  beforeEach(() => {
    localStorage.clear();
    document.body.innerHTML = '';
  });
  afterEach(() => {
    vi.useRealTimers();
    globalThis.fetch = realFetch;
  });

  it('lists the steps with their progress', async () => {
    const { SetupCard, render, screen, fireEvent } = await load();
    render(SetupCard);
    expect(screen.getByText('0/5')).toBeInTheDocument();
    expect(screen.getByText('Add your first site')).toBeInTheDocument();
    expect(screen.getByText('Suspend idle workers')).toBeInTheDocument();
  });

  it('collapses a finished step to its title', async () => {
    const { SetupCard, render, screen, fireEvent } = await load({ open: true });
    render(SetupCard);
    expect(screen.getByText('1/5')).toBeInTheDocument();
    expect(screen.getByText('Start when the dashboard opens')).toBeInTheDocument();
    expect(screen.queryByText('Opening the dashboard brings Lerd up, no terminal needed')).toBeNull();
  });

  it('hands the slot back to the Lerd card when dismissed', async () => {
    const { SetupCard, setupVisible, render, screen, fireEvent } = await load();
    render(SetupCard);
    await fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
    expect(get(setupVisible)).toBe(false);
  });

  it('offers no actions to a session without local control', async () => {
    const { SetupCard, render, screen, fireEvent } = await load({}, false);
    render(SetupCard);
    expect(screen.queryByRole('button', { name: 'Add a site' })).toBeNull();
    expect(screen.getAllByText('Run lerd locally to use this action.')).toHaveLength(5);
  });

  it('says so when the last step ticks, then gives the slot back', async () => {
    vi.useFakeTimers();
    const { SetupCard, setupVisible, idleEnabled, render, screen, fireEvent } = await load({ site: true, service: true, notify: true, open: true });
    render(SetupCard);

    idleEnabled.set(true);
    await vi.advanceTimersByTimeAsync(0);
    expect(screen.getByText("You're all set")).toBeInTheDocument();
    expect(get(setupVisible)).toBe(true);

    await vi.advanceTimersByTimeAsync(3500);
    expect(get(setupVisible)).toBe(false);
  });

  // Finishing while the dashboard was closed has nobody to celebrate with.
  it('goes straight back to the Lerd card when setup finished elsewhere', async () => {
    const { SetupCard, render, screen, fetchMock } = await load({ site: true, service: true, notify: true, open: true, idle: true }, true, 'active');
    render(SetupCard);
    expect(screen.queryByText("You're all set")).toBeNull();
    const posts = fetchMock.mock.calls.filter(([, init]) => init?.method === 'POST');
    expect(posts.map(([, init]) => JSON.parse(String(init!.body)).state)).toEqual(['done']);
  });
});
