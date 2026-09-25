import { derived, get, writable } from 'svelte/store';
import { apiFetch, apiJson } from '$lib/api';
import { wsMessage } from '$lib/ws';
import { sites, sitesLoaded } from '$stores/sites';
import { coreServices } from '$stores/services';
import { status } from '$stores/status';
import { startOnDashboardOpen } from '$stores/autostart';
import { idleEnabled } from '$stores/idle';
import { permissionState, notifyDelivery } from '$lib/notify';
import { configTheme, desktopPalette } from '$stores/palettes';

export type SetupStepId = 'site' | 'service' | 'notify' | 'theme' | 'open' | 'idle';

export interface SetupStep {
  id: SetupStepId;
  done: boolean;
}

// The welcome card this replaced kept its dismissal in the browser; honouring
// it keeps someone who already waved setup away from being asked again.
const LEGACY_DISMISSED_KEY = 'lerd-onboarding-dismissed';

type SetupState = 'unset' | 'active' | 'done';

// Where setup stands comes from the global config, so the browser, the desktop
// app and every other window agree. Null until the daemon has answered.
const state = writable<SetupState | null>(null);

function legacyDismissed(): boolean {
  try {
    return localStorage.getItem(LEGACY_DISMISSED_KEY) === '1';
  } catch {
    return false;
  }
}

export async function loadSetup() {
  try {
    const res = await apiJson<{ setup?: string }>('/api/settings');
    const v = res.setup;
    if (v === 'active' || v === 'done') {
      state.set(v);
      return;
    }
    if (legacyDismissed()) {
      void saveSetup('done');
      return;
    }
    state.set('unset');
  } catch {
    /* keep what we had */
  }
}

async function saveSetup(v: 'active' | 'done') {
  state.set(v);
  try {
    await apiFetch('/api/settings/setup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ state: v })
    });
  } catch {
    /* this window already moved on; the next load asks again */
  }
}

// Another window moving setup on reaches this one as a bare ping.
export function watchSetup() {
  return wsMessage.subscribe((msg) => {
    if (msg?.type === 'setup') void loadSetup();
  });
}

export const setupSteps = derived(
  [sites, coreServices, permissionState, notifyDelivery, desktopPalette, configTheme, startOnDashboardOpen, idleEnabled],
  ([$sites, $services, $permission, $delivery, $desktop, $theme, $openStart, $idle]): SetupStep[] => [
    { id: 'site', done: $sites.length > 0 },
    { id: 'service', done: $services.some((s) => s.status === 'active') },
    { id: 'notify', done: $permission === 'granted' || $delivery === 'native' },
    // Matching the desktop only makes sense where there is a desktop to match.
    ...($desktop ? [{ id: 'theme' as const, done: !!$theme }] : []),
    { id: 'open', done: $openStart },
    { id: 'idle', done: $idle }
  ]
);

export const setupDone = derived(setupSteps, ($steps) => $steps.every((s) => s.done));

// Setup only ever starts on a dashboard that has no sites yet, so an install
// that is already in use keeps its Lerd card. Once started it runs until it is
// finished or dismissed, even after the first site lands. Streaming mode can
// hide every site, which is not a fresh install.
export const setupVisible = derived(
  [state, sitesLoaded, sites, status],
  ([$state, $loaded, $sites, $status]) =>
    $state === 'active' || ($state === 'unset' && $loaded && $sites.length === 0 && !$status.streaming_mode)
);

// The notifications and theme banners ask what Get started already asks, so
// they are for installs that never went through it: not while it is loading or
// showing, and not after it finished or was dismissed.
export const bannersAllowed = derived(
  [state, setupVisible],
  ([$state, $visible]) => $state === 'unset' && !$visible
);

export function startSetup() {
  if (get(state) === 'unset') void saveSetup('active');
}

export function finishSetup() {
  void saveSetup('done');
}
