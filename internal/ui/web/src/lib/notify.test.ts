import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';

// Mock dashboard store so tests don't pull in services/site fetching.
const openOverlay = vi.fn();
vi.mock('$stores/dashboard', () => ({
  openOverlayUrl: (url: string) => openOverlay(url)
}));

// MockNotification stands in for the global Notification constructor.
class MockNotification {
  static permission: NotificationPermission = 'granted';
  static instances: MockNotification[] = [];
  static requestPermission = vi.fn(async () => MockNotification.permission);
  title: string;
  body?: string;
  tag?: string;
  constructor(title: string, opts?: NotificationOptions) {
    this.title = title;
    this.body = opts?.body;
    this.tag = opts?.tag;
    MockNotification.instances.push(this);
  }
  close() {
    /* noop */
  }
  onclick: (() => void) | null = null;
}

const swShows: Array<{ title: string; opts?: NotificationOptions }> = [];

function installSWMock() {
  const reg = {
    showNotification: vi.fn(async (title: string, opts?: NotificationOptions) => {
      swShows.push({ title, opts });
    })
  };
  Object.defineProperty(globalThis.navigator, 'serviceWorker', {
    configurable: true,
    value: {
      ready: Promise.resolve(reg),
      addEventListener: vi.fn()
    }
  });
}
function removeSWMock() {
  delete (globalThis.navigator as unknown as { serviceWorker?: unknown }).serviceWorker;
}

interface Notification {
  kind: string;
  title?: string;
  title_key?: string;
  body?: string;
  body_key?: string;
  params?: Record<string, string>;
  tag?: string;
  url?: string;
  data?: Record<string, string>;
}

describe('notify dispatcher', () => {
  beforeEach(() => {
    MockNotification.instances = [];
    MockNotification.permission = 'granted';
    swShows.length = 0;
    openOverlay.mockClear();
    // @ts-expect-error test double
    globalThis.Notification = MockNotification;
    localStorage.clear();
    installSWMock();
    vi.resetModules();
  });

  afterEach(() => {
    // @ts-expect-error reset
    delete globalThis.Notification;
    removeSWMock();
  });

  it('fires a notification via SW.showNotification when a WS notification arrives', async () => {
    const { initNotify } = await import('./notify');
    const { wsMessage } = await import('./ws');

    initNotify();
    const evt: Notification = {
      kind: 'mail',
      title: 'New email: Welcome',
      body: 'From: alice@x.com',
      tag: 'lerd-mail-abc',
      url: '#service/mailpit/view/abc',
      data: { id: 'abc' }
    };
    wsMessage.set({ type: 'notification', notification: evt });
    await Promise.resolve();
    await Promise.resolve();

    expect(swShows).toHaveLength(1);
    expect(swShows[0].title).toBe('New email: Welcome');
    expect(swShows[0].opts?.body).toBe('From: alice@x.com');
    expect(swShows[0].opts?.tag).toBe('lerd-mail-abc');
    expect((swShows[0].opts?.data as { kind?: string })?.kind).toBe('mail');
  });

  it('raises nothing on the desktop while the dashboard window has focus', async () => {
    const hasFocus = vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const { initNotify } = await import('./notify');
    const { wsMessage } = await import('./ws');

    initNotify();
    wsMessage.set({
      type: 'notification',
      notification: { kind: 'mail', title: 'already on screen' }
    });
    await Promise.resolve();
    await Promise.resolve();

    expect(swShows).toHaveLength(0);
    hasFocus.mockRestore();
  });

  it('suppresses notifications for kinds the user has disabled', async () => {
    const { initNotify, setNotifyPref } = await import('./notify');
    const { wsMessage } = await import('./ws');

    initNotify();
    setNotifyPref('mail', false);

    wsMessage.set({
      type: 'notification',
      notification: { kind: 'mail', title: 'should not fire' }
    });
    await Promise.resolve();
    await Promise.resolve();

    expect(swShows).toHaveLength(0);
  });

  it('suppresses notifications when the master toggle is off', async () => {
    const { initNotify, setNotifyMaster } = await import('./notify');
    const { wsMessage } = await import('./ws');

    initNotify();
    setNotifyMaster(false);

    wsMessage.set({
      type: 'notification',
      notification: { kind: 'mail', title: 'masked' }
    });
    await Promise.resolve();
    await Promise.resolve();

    expect(swShows).toHaveLength(0);
  });

  it('fires N+1 notifications by default and suppresses them once toggled off', async () => {
    const { initNotify, setNotifyPref } = await import('./notify');
    const { wsMessage } = await import('./ws');

    initNotify();
    // nplusone defaults on, so the first warning fires.
    wsMessage.set({
      type: 'notification',
      notification: { kind: 'nplusone', title: 'N+1 on acme', tag: 'lerd-nplusone-a' }
    });
    await Promise.resolve();
    await Promise.resolve();
    expect(swShows).toHaveLength(1);

    // Turning the category off suppresses subsequent N+1 warnings.
    setNotifyPref('nplusone', false);
    wsMessage.set({
      type: 'notification',
      notification: { kind: 'nplusone', title: 'N+1 on beta', tag: 'lerd-nplusone-b' }
    });
    await Promise.resolve();
    await Promise.resolve();
    expect(swShows).toHaveLength(1);
  });

  it('deduplicates back-to-back notifications with the same tag', async () => {
    const { initNotify } = await import('./notify');
    const { wsMessage } = await import('./ws');

    initNotify();
    const evt: Notification = { kind: 'mail', title: 'x', tag: 'lerd-mail-dup' };
    wsMessage.set({ type: 'notification', notification: evt });
    wsMessage.set({ type: 'notification', notification: { ...evt } });
    await Promise.resolve();
    await Promise.resolve();

    expect(swShows).toHaveLength(1);
  });

  it('allows a same-tag notification once the dedupe window has passed', async () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(new Date('2026-05-18T10:00:00Z'));
      const { initNotify } = await import('./notify');
      const { wsMessage } = await import('./ws');

      initNotify();
      const evt: Notification = { kind: 'mail', title: 'first', tag: 'lerd-test' };
      wsMessage.set({ type: 'notification', notification: evt });
      await Promise.resolve();
      await Promise.resolve();

      vi.setSystemTime(new Date('2026-05-18T10:00:05Z'));
      wsMessage.set({
        type: 'notification',
        notification: { ...evt, title: 'second' }
      });
      await Promise.resolve();
      await Promise.resolve();

      expect(swShows).toHaveLength(2);
    } finally {
      vi.useRealTimers();
    }
  });

  it('does not dedupe across kinds even with the same tag', async () => {
    const { initNotify } = await import('./notify');
    const { wsMessage } = await import('./ws');

    initNotify();
    wsMessage.set({
      type: 'notification',
      notification: { kind: 'mail', title: 'M', tag: 'shared' }
    });
    wsMessage.set({
      type: 'notification',
      notification: { kind: 'worker_failed', title: 'W', tag: 'shared' }
    });
    await Promise.resolve();
    await Promise.resolve();

    expect(swShows).toHaveLength(2);
  });

  it('persists and exposes preferences', async () => {
    const { setNotifyPref, setNotifyMaster, notifyPrefs } = await import('./notify');

    setNotifyPref('dump', true);
    setNotifyMaster(false);

    const cur = get(notifyPrefs);
    expect(cur.kinds.dump).toBe(true);
    expect(cur.enabled).toBe(false);

    const stored = localStorage.getItem('lerd:notify:prefs');
    expect(stored).toBeTruthy();
    const parsed = JSON.parse(stored!);
    expect(parsed.kinds.dump).toBe(true);
    expect(parsed.enabled).toBe(false);
  });

  it('default prefs enable mail/worker/op/update and disable dump', async () => {
    const { notifyPrefs } = await import('./notify');
    const cur = get(notifyPrefs);
    expect(cur.enabled).toBe(true);
    expect(cur.kinds.mail).toBe(true);
    expect(cur.kinds.worker_failed).toBe(true);
    expect(cur.kinds.op_done).toBe(true);
    expect(cur.kinds.update_available).toBe(true);
    expect(cur.kinds.dump).toBe(false);
  });
});

describe('forgetCurrentBrowser', () => {
  interface FakeSub {
    endpoint: string;
    unsubscribe: ReturnType<typeof vi.fn>;
  }
  let fakeSub: FakeSub | null = null;
  let subscribeMock: ReturnType<typeof vi.fn>;

  function installPushMock(endpoint: string | null) {
    fakeSub = endpoint
      ? { endpoint, unsubscribe: vi.fn(async () => true) }
      : null;
    subscribeMock = vi.fn(async () => ({
      endpoint: 'https://push.example/new',
      toJSON: () => ({
        endpoint: 'https://push.example/new',
        keys: { p256dh: 'p', auth: 'a' }
      })
    }));
    const reg = {
      showNotification: vi.fn(),
      pushManager: {
        getSubscription: vi.fn(async () => fakeSub),
        subscribe: subscribeMock
      }
    };
    Object.defineProperty(globalThis.navigator, 'serviceWorker', {
      configurable: true,
      value: { ready: Promise.resolve(reg), addEventListener: vi.fn() }
    });
    // PushManager presence is the gate ensurePushSubscription checks.
    (globalThis as unknown as { PushManager?: object }).PushManager = function PushManager() {};
  }

  beforeEach(() => {
    MockNotification.permission = 'granted';
    // @ts-expect-error test double
    globalThis.Notification = MockNotification;
    localStorage.clear();
    vi.resetModules();
  });

  afterEach(() => {
    // @ts-expect-error reset
    delete globalThis.Notification;
    delete (globalThis as unknown as { PushManager?: object }).PushManager;
    delete (globalThis.navigator as unknown as { serviceWorker?: unknown }).serviceWorker;
  });

  it('unsubscribes and sets the flag when endpoint matches the current sub', async () => {
    installPushMock('https://push.example/mine');
    const { forgetCurrentBrowser, autoSubscribeDisabled } = await import('./notify');

    const result = await forgetCurrentBrowser('https://push.example/mine');

    expect(result).toBe(true);
    expect(fakeSub?.unsubscribe).toHaveBeenCalledTimes(1);
    expect(localStorage.getItem('lerd:notify:auto-subscribe')).toBe('0');
    expect(get(autoSubscribeDisabled)).toBe(true);
  });

  it('is a no-op when endpoint does not match', async () => {
    installPushMock('https://push.example/mine');
    const { forgetCurrentBrowser, autoSubscribeDisabled } = await import('./notify');

    const result = await forgetCurrentBrowser('https://push.example/somebody-else');

    expect(result).toBe(false);
    expect(fakeSub?.unsubscribe).not.toHaveBeenCalled();
    expect(localStorage.getItem('lerd:notify:auto-subscribe')).toBeNull();
    expect(get(autoSubscribeDisabled)).toBe(false);
  });

  it('is a no-op when the browser has no subscription', async () => {
    installPushMock(null);
    const { forgetCurrentBrowser, autoSubscribeDisabled } = await import('./notify');

    const result = await forgetCurrentBrowser('https://push.example/anything');

    expect(result).toBe(false);
    expect(get(autoSubscribeDisabled)).toBe(false);
  });

  it('initNotify skips ensurePushSubscription when the flag is set', async () => {
    installPushMock('https://push.example/mine');
    localStorage.setItem('lerd:notify:auto-subscribe', '0');

    const reg = await navigator.serviceWorker!.ready;
    const getSub = (reg as unknown as { pushManager: { getSubscription: ReturnType<typeof vi.fn> } })
      .pushManager.getSubscription;

    const { initNotify } = await import('./notify');
    initNotify();
    await Promise.resolve();
    await Promise.resolve();

    expect(getSub).not.toHaveBeenCalled();
  });

  it('enableNotifications clears the flag and triggers a re-subscribe', async () => {
    installPushMock(null);
    localStorage.setItem('lerd:notify:auto-subscribe', '0');

    const { enableNotifications, autoSubscribeDisabled } = await import('./notify');
    // apiFetch will try to hit /api/push/vapid-public-key — stub fetch so
    // ensurePushSubscription's branch returns silently instead of throwing.
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(null, { status: 404 })
    );

    const res = await enableNotifications();
    await Promise.resolve();

    expect(res).toBe('granted');
    expect(localStorage.getItem('lerd:notify:auto-subscribe')).toBeNull();
    expect(get(autoSubscribeDisabled)).toBe(false);

    fetchSpy.mockRestore();
  });
});

describe('detectBrowserFamily', () => {
  it('classifies Chrome as chromium', async () => {
    const { detectBrowserFamily } = await import('./notify');
    expect(
      detectBrowserFamily(
        'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36'
      )
    ).toBe('chromium');
  });

  it('classifies Edge as chromium', async () => {
    const { detectBrowserFamily } = await import('./notify');
    expect(
      detectBrowserFamily(
        'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0'
      )
    ).toBe('chromium');
  });

  it('classifies Opera as chromium', async () => {
    const { detectBrowserFamily } = await import('./notify');
    expect(
      detectBrowserFamily(
        'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 OPR/115.0.0.0'
      )
    ).toBe('chromium');
  });

  it('classifies Firefox as firefox', async () => {
    const { detectBrowserFamily } = await import('./notify');
    expect(
      detectBrowserFamily(
        'Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0'
      )
    ).toBe('firefox');
  });

  it('classifies Safari (without Chrome) as safari', async () => {
    const { detectBrowserFamily } = await import('./notify');
    expect(
      detectBrowserFamily(
        'Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15'
      )
    ).toBe('safari');
  });

  it('falls back to other for empty or unknown UAs', async () => {
    const { detectBrowserFamily } = await import('./notify');
    expect(detectBrowserFamily('')).toBe('other');
    expect(detectBrowserFamily('SomeCustomBot/1.0')).toBe('other');
  });
});

describe('in-app notifications', () => {
  beforeEach(() => {
    MockNotification.instances = [];
    MockNotification.permission = 'granted';
    swShows.length = 0;
    // @ts-expect-error test double
    globalThis.Notification = MockNotification;
    localStorage.clear();
    installSWMock();
    vi.resetModules();
  });

  afterEach(() => {
    // @ts-expect-error reset
    delete globalThis.Notification;
    removeSWMock();
  });

  it('shows the event in the page while the window has focus', async () => {
    const hasFocus = vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const { initNotify, inAppNotifications, dismissInApp } = await import('./notify');
    const { wsMessage } = await import('./ws');

    initNotify();
    wsMessage.set({
      type: 'notification',
      notification: { kind: 'mail', title: 'New email', body: 'From alice' }
    });
    await Promise.resolve();
    await Promise.resolve();

    const list = get(inAppNotifications);
    expect(list).toHaveLength(1);
    expect(list[0].title).toBe('New email');
    expect(list[0].failed).toBe(false);
    expect(swShows).toHaveLength(0);

    dismissInApp(list[0].id);
    expect(get(inAppNotifications)).toHaveLength(0);
    hasFocus.mockRestore();
  });

  it('records a failure in the page even when the desktop popup also fires', async () => {
    const hasFocus = vi.spyOn(document, 'hasFocus').mockReturnValue(false);
    const { initNotify, inAppNotifications } = await import('./notify');
    const { wsMessage } = await import('./ws');

    initNotify();
    wsMessage.set({
      type: 'notification',
      notification: { kind: 'op_failed', title: 'Migrate failed: mariadb', body: 'exit 127' }
    });
    await Promise.resolve();
    await Promise.resolve();

    expect(get(inAppNotifications)[0].failed).toBe(true);
    expect(swShows).toHaveLength(1);
    hasFocus.mockRestore();
  });

  it('reports a failure even with notifications muted', async () => {
    const hasFocus = vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const { initNotify, setNotifyMaster, inAppNotifications } = await import('./notify');
    const { wsMessage } = await import('./ws');

    initNotify();
    setNotifyMaster(false);
    wsMessage.set({
      type: 'notification',
      notification: { kind: 'op_failed', title: 'Install failed: PHP 8.5', body: 'boom' }
    });
    await Promise.resolve();
    await Promise.resolve();

    expect(get(inAppNotifications)).toHaveLength(1);
    hasFocus.mockRestore();
  });
});

describe('notification centre', () => {
  beforeEach(() => {
    MockNotification.instances = [];
    MockNotification.permission = 'granted';
    swShows.length = 0;
    // @ts-expect-error test double
    globalThis.Notification = MockNotification;
    localStorage.clear();
    installSWMock();
    vi.resetModules();
  });

  afterEach(() => {
    // @ts-expect-error reset
    delete globalThis.Notification;
    removeSWMock();
  });

  it('keeps every delivered notification, unread, and survives a reload', async () => {
    const hasFocus = vi.spyOn(document, 'hasFocus').mockReturnValue(false);
    const first = await import('./notify');
    const { wsMessage } = await import('./ws');

    first.initNotify();
    wsMessage.set({
      type: 'notification',
      notification: { kind: 'op_failed', title: 'Migrate failed: mariadb', body: 'exit 127' }
    });
    await Promise.resolve();
    await Promise.resolve();

    expect(get(first.unreadNotifications)).toBe(1);

    // A reload re-reads the persisted list rather than starting empty.
    vi.resetModules();
    const reloaded = await import('./notify');
    expect(get(reloaded.notificationHistory)).toHaveLength(1);
    expect(get(reloaded.unreadNotifications)).toBe(1);

    reloaded.markNotificationsRead();
    expect(get(reloaded.unreadNotifications)).toBe(0);

    reloaded.clearNotificationHistory();
    expect(get(reloaded.notificationHistory)).toHaveLength(0);
    hasFocus.mockRestore();
  });
});

describe('test notification', () => {
  beforeEach(() => {
    MockNotification.instances = [];
    MockNotification.permission = 'granted';
    swShows.length = 0;
    // @ts-expect-error test double
    globalThis.Notification = MockNotification;
    localStorage.clear();
    installSWMock();
    vi.resetModules();
  });

  afterEach(() => {
    // @ts-expect-error reset
    delete globalThis.Notification;
    removeSWMock();
  });

  // Send test is pressed from the settings panel, so the window always has
  // focus; suppressing it there would make the button look broken.
  it('reaches the desktop even with the dashboard focused, and is not filed in the centre', async () => {
    const hasFocus = vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const { initNotify, notificationHistory, inAppNotifications } = await import('./notify');
    const { wsMessage } = await import('./ws');

    initNotify();
    wsMessage.set({
      type: 'notification',
      notification: { kind: 'test', title: 'lerd notifications test' }
    });
    await Promise.resolve();
    await Promise.resolve();

    expect(swShows).toHaveLength(1);
    expect(get(notificationHistory)).toHaveLength(0);
    expect(get(inAppNotifications)).toHaveLength(0);
    hasFocus.mockRestore();
  });
});

describe('notification ids', () => {
  beforeEach(() => {
    MockNotification.instances = [];
    MockNotification.permission = 'granted';
    swShows.length = 0;
    // @ts-expect-error test double
    globalThis.Notification = MockNotification;
    localStorage.clear();
    installSWMock();
    vi.resetModules();
  });

  afterEach(() => {
    // @ts-expect-error reset
    delete globalThis.Notification;
    removeSWMock();
  });

  // A repeated id makes the keyed list throw each_key_duplicate, which takes
  // the whole dashboard down, so ids have to outlive the page that made them.
  // The stored list is renumbered on load for the same reason: a payload
  // written by an older build can already carry repeats.
  it('stay unique across a reload', async () => {
    const hasFocus = vi.spyOn(document, 'hasFocus').mockReturnValue(false);
    const first = await import('./notify');
    const { wsMessage } = await import('./ws');
    first.initNotify();
    wsMessage.set({ type: 'notification', notification: { kind: 'op_done', title: 'one' } });
    await Promise.resolve();
    await Promise.resolve();

    vi.resetModules();
    const reloaded = await import('./notify');
    const ws2 = await import('./ws');
    reloaded.initNotify();
    ws2.wsMessage.set({ type: 'notification', notification: { kind: 'op_done', title: 'two' } });
    await Promise.resolve();
    await Promise.resolve();

    const ids = get(reloaded.notificationHistory).map((r) => r.id);
    expect(ids).toHaveLength(2);
    expect(new Set(ids).size).toBe(2);
    hasFocus.mockRestore();
  });
});

describe('stored notification history', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetModules();
  });

  it('renumbers a stored list that repeats an id', async () => {
    localStorage.setItem(
      'lerd:notify:history',
      JSON.stringify([
        { id: 1, kind: 'op_failed', title: 'Migrate failed', body: '', url: '', failed: true, at: 1, read: false },
        { id: 1, kind: 'op_done', title: 'Update finished', body: '', url: '', failed: false, at: 2, read: true }
      ])
    );

    const { notificationHistory } = await import('./notify');
    const ids = get(notificationHistory).map((r) => r.id);
    expect(ids).toHaveLength(2);
    expect(new Set(ids).size).toBe(2);
  });

  it('drops junk rather than rendering it', async () => {
    localStorage.setItem('lerd:notify:history', JSON.stringify([null, 7, { id: 3 }]));
    const { notificationHistory } = await import('./notify');
    expect(get(notificationHistory)).toHaveLength(0);
  });

  // Entries written before debug notifications learned to open the site they
  // came from still point at the global bridge view, and the list outlives the
  // build that wrote it, so the stale route is rewritten on load (#1005).
  it('retargets a stored debug notification away from the bridge view', async () => {
    localStorage.setItem(
      'lerd:notify:history',
      JSON.stringify([
        { id: 1, kind: 'nplusone', title: 'Possible N+1 query on acme', body: '', url: '#system/dump-bridge', failed: false, at: 1, read: false },
        { id: 2, kind: 'op_done', title: 'Update finished', body: '', url: '#services/mysql', failed: false, at: 2, read: true }
      ])
    );

    const { notificationHistory } = await import('./notify');
    const urls = get(notificationHistory).map((r) => r.url);
    expect(urls).toEqual(['#sites', '#services/mysql']);
  });
});

describe('notification severity', () => {
  it('reads a detected problem as a warning, not a completed action', async () => {
    const { notificationSeverity } = await import('./notify');
    expect(notificationSeverity('nplusone', false)).toBe('warning');
    expect(notificationSeverity('slow_route', false)).toBe('warning');
    expect(notificationSeverity('op_done', false)).toBe('info');
    expect(notificationSeverity('mail', false)).toBe('info');
    expect(notificationSeverity('op_failed', true)).toBe('failure');
    // A failed operation outranks its category.
    expect(notificationSeverity('nplusone', true)).toBe('failure');
  });
});
