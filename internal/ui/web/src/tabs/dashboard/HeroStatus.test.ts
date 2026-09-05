import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import HeroStatus from './HeroStatus.svelte';
import { status, statusLoaded } from '$stores/status';
import { accessMode } from '$stores/accessMode';
import { version } from '$stores/version';
import { unhealthyWorkers } from '$stores/workerHealth';
import { sites } from '$stores/sites';
import {
  lerdStart,
  lerdStarting,
  lerdStartStep,
  lerdStartUnit,
  lerdStartDone,
  lerdStartTotal
} from '$stores/lerdLifecycle';

vi.mock('$stores/lerdLifecycle', async () => {
  const { writable } = await import('svelte/store');
  return {
    lerdStart: vi.fn(async () => true),
    lerdStarting: writable(false),
    lerdStopping: writable(false),
    lerdStartStep: writable(''),
    lerdStartUnit: writable(''),
    lerdStartDone: writable(0),
    lerdStartTotal: writable(0)
  };
});

function setStatus(over: Record<string, unknown> = {}) {
  status.update((s) => ({
    ...s,
    dns: { ok: true, status: 'ok', vpn: false, enabled: true, tld: 'test' },
    nginx: { running: true },
    watcher_running: true,
    ...over
  }) as never);
}

describe('HeroStatus', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    statusLoaded.set(true);
    sites.set([]);
    unhealthyWorkers.set([]);
    version.set({ current: '1.0.0', latest: '1.0.0', hasUpdate: false } as never);
    accessMode.set({ localControl: true, lanExposed: false, checked: true });
    lerdStarting.set(false);
    lerdStartStep.set('');
    lerdStartUnit.set('');
    lerdStartDone.set(0);
    lerdStartTotal.set(0);
    setStatus();
  });

  // The Lerd card announces an available update, with a way to dismiss the
  // noise by scrolling past it. A banner that cannot be dismissed would say
  // the same thing twice above every other card.
  it('leaves an available update to the lerd card', () => {
    version.set({ current: '1.0.0', latest: '1.1.0', hasUpdate: true } as never);
    const { queryByText } = render(HeroStatus);

    expect(queryByText(/1\.1\.0 is available/i)).toBeNull();
    expect(queryByText(/open terminal/i)).toBeNull();
  });

  // The whole point of the banner: the stack is down and the user should not
  // have to find a terminal to bring it back.
  it('offers a start button when core services are down', async () => {
    setStatus({ nginx: { running: false } });
    const { getByText } = render(HeroStatus);

    await fireEvent.click(getByText('Start Lerd'));
    expect(lerdStart).toHaveBeenCalled();
  });

  it('keeps the system link alongside the start button', () => {
    setStatus({ nginx: { running: false } });
    const { getByText } = render(HeroStatus);
    expect(getByText('Open system')).toBeTruthy();
  });

  // A LAN viewer cannot run host commands, so it gets the link only.
  it('hides the start button without local control', () => {
    accessMode.set({ localControl: false, lanExposed: true, checked: true });
    setStatus({ nginx: { running: false } });
    const { queryByText, getByText } = render(HeroStatus);
    expect(queryByText('Start Lerd')).toBeNull();
    expect(getByText('Open system')).toBeTruthy();
  });

  it('shows no start button while everything is running', () => {
    const { queryByText } = render(HeroStatus);
    expect(queryByText('Start Lerd')).toBeNull();
  });

  // A start can take minutes on a cold Podman machine, so the button has to
  // say what it is doing rather than sit on a dead label.
  it('counts units on the button while starting', () => {
    setStatus({ nginx: { running: false } });
    lerdStarting.set(true);
    lerdStartTotal.set(12);
    lerdStartDone.set(4);
    const { getByText } = render(HeroStatus);
    expect(getByText('Starting... 4/12')).toBeTruthy();
  });

  it('names the stage it is in before any unit has come up', () => {
    setStatus({ nginx: { running: false } });
    lerdStarting.set(true);
    lerdStartStep.set('images');
    const { getByText } = render(HeroStatus);
    expect(getByText('Checking images')).toBeTruthy();
  });

  // Unit names are the same identifiers the CLI prints, so they go through as-is.
  it('shows the last unit that came up in place of the stage', () => {
    setStatus({ nginx: { running: false } });
    lerdStarting.set(true);
    lerdStartStep.set('units');
    lerdStartUnit.set('mysql');
    const { getByText } = render(HeroStatus);
    expect(getByText('mysql')).toBeTruthy();
  });

  // Failing workers are healed, not started: that branch keeps its own action.
  it('leaves the worker failure banner alone', () => {
    unhealthyWorkers.set([{ unit: 'lerd-q', worker: 'queue', site: 'app.test' }] as never);
    const { queryByText } = render(HeroStatus);
    expect(queryByText('Start Lerd')).toBeNull();
  });
});
