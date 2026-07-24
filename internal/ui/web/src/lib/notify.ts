import { writable, derived, get } from 'svelte/store';
import { wsMessage, type NotificationEvent } from './ws';
import { apiFetch } from './api';
import { m } from '../paraglide/messages.js';

const PREFS_KEY = 'lerd:notify:prefs';
const DISMISS_KEY = 'lerd:notify:dismissed';
// AUTO_SUB_KEY records "user has forgotten this browser; don't silently
// re-subscribe on next mount". Set to "0" by forgetCurrentBrowser, cleared
// by enableNotifications. Without it, ensurePushSubscription would re-post
// the same endpoint right after Forget undeleted the row from the server.
const AUTO_SUB_KEY = 'lerd:notify:auto-subscribe';

// NotifyKind is the canonical set of notification categories the user can
// toggle. The list lives client-side because the page-context dispatcher is
// the only filter; the backend forwards every kind and trusts each
// subscription's stored EnabledKinds to gate Web Push delivery.
export type NotifyKind =
  | 'mail'
  | 'worker_failed'
  | 'op_done'
  | 'update_available'
  | 'nplusone'
  | 'slow_route'
  | 'dump';

export const ALL_KINDS: NotifyKind[] = [
  'mail',
  'worker_failed',
  'op_done',
  'update_available',
  'nplusone',
  'slow_route',
  'dump'
];

export interface NotifyPrefs {
  enabled: boolean;
  kinds: Record<NotifyKind, boolean>;
}

const DEFAULTS: NotifyPrefs = {
  enabled: true,
  kinds: {
    mail: true,
    worker_failed: true,
    op_done: true,
    update_available: true,
    // N+1 warnings are deduped once per route/worker per session, so they
    // stay low-volume and useful; on by default, matching prior behaviour
    // where the kind had no toggle and always fired.
    nplusone: true,
    // slow_route is edge-triggered in the watcher (fires once when a route goes
    // slow, rearms when it recovers), so it stays low-volume; on by default like
    // the other proactive warnings.
    slow_route: true,
    // dump is opt-in: many dev sessions emit hundreds of ray() calls and
    // the user almost always wants to silence them by default.
    dump: false
  }
};

function loadPrefs(): NotifyPrefs {
  if (typeof localStorage === 'undefined') return clonePrefs(DEFAULTS);
  const raw = localStorage.getItem(PREFS_KEY);
  if (!raw) return clonePrefs(DEFAULTS);
  try {
    const p = JSON.parse(raw) as Partial<NotifyPrefs>;
    return {
      enabled: p.enabled ?? DEFAULTS.enabled,
      kinds: { ...DEFAULTS.kinds, ...(p.kinds ?? {}) } as Record<NotifyKind, boolean>
    };
  } catch {
    return clonePrefs(DEFAULTS);
  }
}

function clonePrefs(p: NotifyPrefs): NotifyPrefs {
  return { enabled: p.enabled, kinds: { ...p.kinds } };
}

function savePrefs(p: NotifyPrefs) {
  if (typeof localStorage === 'undefined') return;
  localStorage.setItem(PREFS_KEY, JSON.stringify(p));
}

export const notifyPrefs = writable<NotifyPrefs>(loadPrefs());
export const permissionState = writable<NotificationPermission | 'unsupported'>(
  typeof Notification === 'undefined' ? 'unsupported' : Notification.permission
);
export const dismissed = writable<boolean>(
  typeof localStorage !== 'undefined' && localStorage.getItem(DISMISS_KEY) === '1'
);
export const autoSubscribeDisabled = writable<boolean>(isAutoSubscribeDisabled());

function isAutoSubscribeDisabled(): boolean {
  if (typeof localStorage === 'undefined') return false;
  return localStorage.getItem(AUTO_SUB_KEY) === '0';
}

function setAutoSubscribeDisabled(off: boolean) {
  if (typeof localStorage !== 'undefined') {
    if (off) localStorage.setItem(AUTO_SUB_KEY, '0');
    else localStorage.removeItem(AUTO_SUB_KEY);
  }
  autoSubscribeDisabled.set(off);
}

export function setNotifyPref(kind: NotifyKind, on: boolean) {
  notifyPrefs.update((p) => {
    const next = { enabled: p.enabled, kinds: { ...p.kinds, [kind]: on } };
    savePrefs(next);
    void syncSubscriptionPrefs(next);
    return next;
  });
}

export function setNotifyMaster(on: boolean) {
  notifyPrefs.update((p) => {
    const next = { enabled: on, kinds: { ...p.kinds } };
    savePrefs(next);
    void syncSubscriptionPrefs(next);
    return next;
  });
}

export function dismissNotifyBanner() {
  if (typeof localStorage !== 'undefined') localStorage.setItem(DISMISS_KEY, '1');
  dismissed.set(true);
}

// localizedTitle / localizedBody resolve a Paraglide key with params,
// falling back to the raw English string from the payload when the key is
// missing or Paraglide hasn't compiled a message for it. This is how the
// page achieves localisation while the SW (no DOM, no Paraglide) keeps
// showing the English fallback.
function localize(
  key: string | undefined,
  fallback: string | undefined,
  params: Record<string, string> | undefined
): string {
  if (key) {
    const fn = (m as unknown as Record<string, (p?: Record<string, string>) => string>)[key];
    if (typeof fn === 'function') {
      try {
        return fn(params ?? {});
      } catch {
        /* fall through to fallback */
      }
    }
  }
  return fallback ?? '';
}

// InAppNotification is one entry of the in-page notification stack, the surface
// that carries notifications the desktop never shows.
export interface InAppNotification {
  id: number;
  kind: string;
  title: string;
  body: string;
  url: string;
  failed: boolean;
}

export const inAppNotifications = writable<InAppNotification[]>([]);

// Severity drives how a notification is drawn on the in-page surfaces. The
// diagnostic kinds report a problem lerd found in the user's app, so they read
// as warnings rather than as a completed action.
export type NotifySeverity = 'failure' | 'warning' | 'info';

const WARNING_KINDS = new Set<string>(['nplusone', 'slow_route']);

export function notificationSeverity(kind: string, failed: boolean): NotifySeverity {
  if (failed) return 'failure';
  return WARNING_KINDS.has(kind) ? 'warning' : 'info';
}

// HISTORY_KEY holds the notification centre's list. It is persisted because the
// point of the centre is catching up on what happened while the user was
// elsewhere, including before a reload.
const HISTORY_KEY = 'lerd:notify:history';
const historyLimit = 50;

export interface NotificationRecord extends InAppNotification {
  at: number;
  read: boolean;
}

// Debug notifications used to open the global bridge view, which says nothing
// about the event that was clicked. Stored entries outlive the build that wrote
// them, so an old one is retargeted at the sites list on load.
function retargetStoredURL(url: string): string {
  return url === '#system/dump-bridge' ? '#sites' : (url ?? '');
}

// Stored ids are renumbered on the way in. The list outlives the code that
// wrote it, so a payload from an older build (or a hand-edited one) can carry
// repeats, and two entries under one key throw out of the keyed list and take
// the dashboard down with them.
function loadHistory(): NotificationRecord[] {
  try {
    const raw = localStorage.getItem(HISTORY_KEY);
    const list = raw ? (JSON.parse(raw) as NotificationRecord[]) : [];
    if (!Array.isArray(list)) return [];
    return list
      .filter((r) => r && typeof r === 'object' && typeof r.title === 'string')
      .slice(0, historyLimit)
      .map((r, i) => ({ ...r, id: i + 1, url: retargetStoredURL(r.url) }));
  } catch {
    return [];
  }
}

export const notificationHistory = writable<NotificationRecord[]>(loadHistory());

notificationHistory.subscribe((list) => {
  try {
    localStorage.setItem(HISTORY_KEY, JSON.stringify(list.slice(0, historyLimit)));
  } catch {
    /* storage may be unavailable (private mode, quota) */
  }
});

export const unreadNotifications = derived(notificationHistory, (list) =>
  list.reduce((n, r) => n + (r.read ? 0 : 1), 0)
);

export function markNotificationsRead() {
  notificationHistory.update((list) => list.map((r) => (r.read ? r : { ...r, read: true })));
}

export function clearNotificationHistory() {
  notificationHistory.set([]);
}

// inAppLimit keeps a burst of events from filling the viewport; the oldest
// non-failure entries fall off first.
const inAppLimit = 4;

// Ids are derived from the list they join rather than from a module counter,
// which restarts at zero on every reload and would hand a fresh notification
// the id of a stored one. Two entries under one key throw out of the keyed
// list and take the dashboard down with them.
function nextId(list: { id: number }[]): number {
  return list.reduce((max, n) => Math.max(max, n.id), 0) + 1;
}

function pushInApp(n: Omit<InAppNotification, 'id'>) {
  inAppNotifications.update((list) => [...list, { ...n, id: nextId(list) }].slice(-inAppLimit));
}

// record keeps every delivered notification in the centre, whichever surface
// showed it, so a desktop popup the user missed is still there to be read.
function record(n: Omit<InAppNotification, 'id'>) {
  notificationHistory.update((list) =>
    [{ ...n, id: nextId(list), at: Date.now(), read: false }, ...list].slice(0, historyLimit)
  );
}

export function dismissInApp(id: number) {
  inAppNotifications.update((list) => list.filter((n) => n.id !== id));
}

// notifyLocalFailure raises a failure the page itself detected, rather than one
// the daemon pushed. It takes the same in-app surface and history as a pushed
// event, so a container that will not come back up is still on screen after the
// modal is closed. Failures never auto-dismiss.
export function notifyLocalFailure(kind: string, title: string, body: string) {
  const entry = { kind, title, body, url: '', failed: true };
  record(entry);
  pushInApp(entry);
}

// dedupeWindowMs scopes the tag dedupe to a short window so an immediate
// retry of the same payload collapses but a same-tag send seconds later
// (Send-test double-click, two identical mail webhooks) still fires.
const dedupeWindowMs = 2000;
const recentTags = new Map<string, number>();

// A focused dashboard is already showing whatever the notification announces,
// so the event is left to the page rather than raised on the desktop.
function windowFocused(): boolean {
  if (typeof document === 'undefined') return false;
  return !document.hidden && document.hasFocus();
}

// A failed operation is the one thing the user must not miss: it is reported
// whatever the notification prefs say and it stays on screen until dismissed.
function isFailure(evt: NotificationEvent): boolean {
  return evt.kind.endsWith('_failed') || evt.data?.result === 'failed';
}

async function fireNotification(evt: NotificationEvent) {
  const prefs = get(notifyPrefs);
  const native = get(notifyDelivery) === 'native';
  const failed = isFailure(evt);
  // A failure is always delivered. Otherwise the gate follows the active sink:
  // under native the daemon's resolved per-kind prefs decide, and the browser
  // prefs (which have no UI in native mode) must not act as a phantom filter
  // that keeps the desktop popup but silently empties the bell.
  if (!failed) {
    if (native) {
      if (get(notifyNativeKinds)[evt.kind] === false) return;
    } else {
      if (!prefs.enabled) return;
      if (prefs.kinds[evt.kind as NotifyKind] === false) return;
    }
  }
  if (evt.tag) {
    const key = evt.kind + ' ' + evt.tag;
    const now = Date.now();
    const seen = recentTags.get(key);
    if (seen !== undefined && now - seen < dedupeWindowMs) return;
    recentTags.set(key, now);
    if (recentTags.size > 200) {
      for (const [k, ts] of recentTags) {
        if (now - ts >= dedupeWindowMs) recentTags.delete(k);
      }
    }
  }

  const title = localize(evt.title_key, evt.title, evt.params) || '(notification)';
  const body = localize(evt.body_key, evt.body, evt.params) || '';

  // In-app first: it is the only surface while the window has focus, and a
  // failure is recorded even when the desktop popup also fires, so it is still
  // waiting on screen when the user comes back to the dashboard.
  // The test notification exists to prove the desktop path works, so it skips
  // the in-page surfaces and the focus check that would swallow it.
  const isTest = evt.kind === 'test';
  if (!isTest) {
    const entry = { kind: evt.kind, title, body, url: evt.url ?? '', failed };
    record(entry);
    if (failed || windowFocused()) {
      pushInApp(entry);
    }
    if (windowFocused()) return;
    // Under the native sink the daemon has already posted this to the desktop.
    // A second popup from the page duplicates it and takes the click away from
    // the desktop app, which the daemon's copy opens through lerd://.
    if (get(notifyDelivery) === 'native') return;
  }
  if (typeof Notification === 'undefined') return;
  if (Notification.permission !== 'granted') return;

  const opts: NotificationOptions = {
    body,
    tag: evt.tag,
    icon: evt.icon ?? '/icons/icon-192.png',
    data: { kind: evt.kind, url: evt.url ?? '', ...(evt.data ?? {}) }
  };

  if ('serviceWorker' in navigator) {
    try {
      const reg = await navigator.serviceWorker.ready;
      await reg.showNotification(title, opts);
      return;
    } catch {
      /* fall through to page-level Notification */
    }
  }
  new Notification(title, opts);
}

// notifyDelivery mirrors the server's notification sink (browser | native) so
// browser-only UI (the enable banner, permission prompts) can hide itself when
// the daemon is delivering notifications natively.
export const notifyDelivery = writable<'browser' | 'native'>('browser');

// desktopAppInstalled mirrors whether the daemon sees the Lerd desktop app as
// the lerd:// handler, so the web UI can offer "Open in app".
export const desktopAppInstalled = writable<boolean>(false);

// notifyNativeKinds mirrors the daemon's resolved per-kind native prefs. Under
// the native sink the browser prefs are hidden, so the bell and toasts must
// follow these instead: a kind the daemon posts to the desktop is one the bell
// should keep, and one it suppresses is one the bell should drop.
export const notifyNativeKinds = writable<Record<string, boolean>>({});

export async function loadNotifyDelivery() {
  try {
    const r = await apiFetch('/api/notifications/target');
    if (r.ok) {
      const d = (await r.json()) as {
        target?: string;
        app_installed?: boolean;
        kinds?: Record<string, boolean>;
      };
      notifyDelivery.set(d.target === 'native' ? 'native' : 'browser');
      desktopAppInstalled.set(!!d.app_installed);
      notifyNativeKinds.set(d.kinds ?? {});
    }
  } catch {
    /* keep browser default */
  }
}

// insideDesktopApp is true when the dashboard is running inside the Lerd desktop
// app (its preload exposes window.lerd), so "Open in app" hides there.
export function insideDesktopApp(): boolean {
  return typeof window !== 'undefined' && typeof (window as { lerd?: unknown }).lerd !== 'undefined';
}

// openInDesktopApp hands off to the desktop app at the current route via its
// lerd:// scheme.
export function openInDesktopApp() {
  if (typeof location === 'undefined') return;
  const route = location.hash || '/';
  location.href = 'lerd://open/' + route;
}

// handleProtocolLaunch routes a PWA opened via its web+lerd:// protocol handler.
// The manifest maps the scheme to /?lerd=<full web+lerd:// url>; we extract the
// route, navigate, and strip the query so a refresh doesn't re-trigger it.
export function handleProtocolLaunch() {
  if (typeof location === 'undefined') return;
  const raw = new URLSearchParams(location.search).get('lerd');
  if (!raw) return;
  history.replaceState(null, '', location.pathname + location.hash);
  const m = /^web\+lerd:\/\/open\/?(.*)$/.exec(raw);
  const route = m ? m[1] : '';
  if (route) openOverlayUrl(route.startsWith('#') ? route : '#' + route);
}

let initialized = false;

export function initNotify() {
  if (initialized) return;
  initialized = true;
  handleProtocolLaunch();
  void loadNotifyDelivery();
  wsMessage.subscribe((msg) => {
    if (!msg?.notification) return;
    void fireNotification(msg.notification);
  });
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.addEventListener('message', (e: MessageEvent) => {
      const data = e.data as { kind?: string; url?: string } | undefined;
      if (data?.kind === 'lerd-open' && data.url) {
        openOverlayUrl(data.url);
      }
    });
  }
  // Re-register the push subscription on every mount when permission is
  // already granted so the server's subscription list stays in sync after
  // a browser reset, sub expiry, or pref change made while offline.
  // Skipped when the user has clicked Forget on this browser — otherwise
  // the row they just deleted would reappear on the next page load.
  if (
    typeof Notification !== 'undefined' &&
    Notification.permission === 'granted' &&
    !isAutoSubscribeDisabled()
  ) {
    void ensurePushSubscription();
  }
}

export function shouldShowOptIn(): boolean {
  if (typeof Notification === 'undefined') return false;
  if (Notification.permission !== 'default') return false;
  return !get(dismissed);
}

// BrowserFamily is the smallest classification we need for picking the right
// "how to unblock notifications" copy. Edge / Brave / Opera all collapse into
// chromium because their site-permissions UI is the lock/icon flow.
export type BrowserFamily = 'chromium' | 'firefox' | 'safari' | 'other';

export function detectBrowserFamily(ua: string): BrowserFamily {
  if (!ua) return 'other';
  if (/Firefox\//.test(ua)) return 'firefox';
  if (/Edg\/|OPR\/|Chrome\//.test(ua)) return 'chromium';
  if (/Safari\//.test(ua)) return 'safari';
  return 'other';
}

export async function enableNotifications(): Promise<NotificationPermission | 'unsupported'> {
  if (typeof Notification === 'undefined') return 'unsupported';
  let result: NotificationPermission;
  if (Notification.permission === 'granted' || Notification.permission === 'denied') {
    result = Notification.permission;
  } else {
    result = await Notification.requestPermission();
  }
  permissionState.set(result);
  if (result === 'granted') {
    // Explicit user opt-in clears any prior Forget — without this the
    // user could click "Subscribe this browser" forever and nothing would
    // happen because initNotify already skipped ensurePushSubscription.
    setAutoSubscribeDisabled(false);
    void ensurePushSubscription();
  }
  return result;
}

// forgetCurrentBrowser is called by the settings panel's Forget button when
// the removed device matches the current browser. It revokes the live
// PushSubscription so the browser's push service stops hitting our endpoint,
// then sets the auto-subscribe-disabled flag so initNotify on the next
// mount doesn't silently re-register the same browser.
export async function forgetCurrentBrowser(endpoint: string): Promise<boolean> {
  if (!('serviceWorker' in navigator)) return false;
  try {
    const reg = await navigator.serviceWorker.ready;
    const sub = await reg.pushManager.getSubscription();
    if (!sub || sub.endpoint !== endpoint) return false;
    await sub.unsubscribe();
    setAutoSubscribeDisabled(true);
    return true;
  } catch (err) {
    console.warn('[lerd] forget current browser failed:', err);
    return false;
  }
}

// urlBase64ToArrayBuffer decodes a base64url VAPID public key into the
// ArrayBuffer pushManager.subscribe wants for applicationServerKey.
function urlBase64ToArrayBuffer(b64: string): ArrayBuffer {
  const padding = '='.repeat((4 - (b64.length % 4)) % 4);
  const base64 = (b64 + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(base64);
  const out = new ArrayBuffer(raw.length);
  const view = new Uint8Array(out);
  for (let i = 0; i < raw.length; i++) view[i] = raw.charCodeAt(i);
  return out;
}

async function ensurePushSubscription(): Promise<void> {
  if (!('serviceWorker' in navigator) || !('PushManager' in window)) return;
  try {
    const reg = await navigator.serviceWorker.ready;
    let sub = await reg.pushManager.getSubscription();
    if (!sub) {
      const r = await apiFetch('/api/push/vapid-public-key');
      if (!r.ok) return;
      const { public_key: pubKey } = (await r.json()) as { public_key: string };
      if (!pubKey) return;
      sub = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToArrayBuffer(pubKey)
      });
    }
    await postSubscription(sub);
  } catch (err) {
    console.warn('[lerd] push subscribe failed:', err);
  }
}

async function postSubscription(sub: PushSubscription) {
  const prefs = get(notifyPrefs);
  await apiFetch('/api/push/subscribe', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      ...sub.toJSON(),
      enabled: prefs.enabled,
      enabled_kinds: Object.entries(prefs.kinds)
        .filter(([, on]) => on)
        .map(([k]) => k)
    })
  });
}

// syncSubscriptionPrefs pushes the latest prefs to the backend so closed-PWA
// Web Push respects them. Best-effort; ignored if no subscription exists.
async function syncSubscriptionPrefs(_p: NotifyPrefs) {
  if (!('serviceWorker' in navigator)) return;
  try {
    const reg = await navigator.serviceWorker.ready;
    const sub = await reg.pushManager.getSubscription();
    if (!sub) return;
    await postSubscription(sub);
  } catch {
    /* non-fatal; will re-sync on next page mount */
  }
}

// openOverlayUrl is the SW-message click handler — opens the deep-linked
// overlay when the user clicks an OS notification while the dashboard tab
// is open. Imported lazily so notify.ts has no compile-time dependency on
// the dashboard store (and so the test file can mock it).
function openOverlayUrl(url: string) {
  // Setting the hash kicks initDashboardRoute / hashchange listener which
  // resolves the right service + extraPath; see stores/dashboard.ts.
  if (typeof location !== 'undefined' && url.startsWith('#')) {
    location.hash = url.slice(1);
  }
}

export function _resetNotifyForTest() {
  initialized = false;
  recentTags.clear();
  notifyPrefs.set(clonePrefs(DEFAULTS));
  dismissed.set(false);
  autoSubscribeDisabled.set(false);
  if (typeof Notification !== 'undefined') {
    permissionState.set(Notification.permission);
  }
}
