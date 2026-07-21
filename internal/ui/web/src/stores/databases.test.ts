import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { dsnFor, importDatabase, type DatabaseEngine, type ImportProgress } from './databases';

function engine(connection_url?: string): DatabaseEngine {
  return {
    service: 'mysql',
    family: 'mysql',
    status: 'active',
    supports_create: true,
    supports_snapshot: true,
    databases: [],
    connection_url
  };
}

describe('dsnFor', () => {
  it('rewrites the database path for a SQL DSN', () => {
    const dsn = dsnFor(engine('mysql://root:lerd@127.0.0.1:3306/lerd'), 'shop');
    expect(dsn).toBe('mysql://root:lerd@127.0.0.1:3306/shop');
  });

  it('keeps query params when swapping the mongo database', () => {
    const dsn = dsnFor(engine('mongodb://root:lerd@127.0.0.1:27017/?authSource=admin'), 'analytics');
    expect(dsn).toBe('mongodb://root:lerd@127.0.0.1:27017/analytics?authSource=admin');
  });

  it('returns empty when the engine has no connection string', () => {
    expect(dsnFor(engine(undefined), 'shop')).toBe('');
  });
});

// FakeXHR stands in for the upload so a test can drive the progress events the
// real browser fires while the dump streams to the daemon.
class FakeXHR {
  static last: FakeXHR | null = null;
  upload = { onprogress: null as ((e: ProgressEvent) => void) | null };
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  status = 200;
  responseText = '{"ok":true}';
  headers: Record<string, string> = {};
  url = '';
  constructor() {
    FakeXHR.last = this;
  }
  open(_method: string, url: string) {
    this.url = url;
  }
  setRequestHeader(key: string, value: string) {
    this.headers[key] = value;
  }
  send(_body: unknown) {}
  progress(loaded: number, total: number) {
    this.upload.onprogress?.({ lengthComputable: true, loaded, total } as ProgressEvent);
  }
  finish(status: number, body: string) {
    this.status = status;
    this.responseText = body;
    this.onload?.();
  }
}

describe('importDatabase', () => {
  beforeEach(() => {
    FakeXHR.last = null;
    vi.stubGlobal('XMLHttpRequest', FakeXHR);
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('{"service":"mysql"}', { status: 200 }))
    );
  });
  afterEach(() => vi.unstubAllGlobals());

  it('reports upload progress while the dump streams', async () => {
    const seen: ImportProgress[] = [];
    const done = importDatabase('mysql', 'shop', new File(['dump'], 'shop.sql'), (p) =>
      seen.push(p)
    );
    FakeXHR.last!.progress(256, 1024);
    FakeXHR.last!.progress(1024, 1024);
    FakeXHR.last!.finish(200, '{"ok":true}');
    await expect(done).resolves.toEqual({ ok: true });
    expect(seen).toEqual([
      { percent: 0.25, uploaded: false },
      { percent: 1, uploaded: true }
    ]);
  });

  it('returns the engine error when the import fails', async () => {
    const done = importDatabase('mysql', 'shop', new File(['dump'], 'shop.sql'));
    FakeXHR.last!.finish(200, '{"ok":false,"error":"import failed: syntax error"}');
    await expect(done).resolves.toEqual({ ok: false, error: 'import failed: syntax error' });
  });

  it('reports a plain-text error body', async () => {
    const done = importDatabase('mysql', 'shop', new File(['dump'], 'shop.sql'));
    FakeXHR.last!.finish(413, 'dump too large');
    await expect(done).resolves.toEqual({ ok: false, error: 'dump too large' });
  });

  it('resolves with an error when the upload itself breaks', async () => {
    const done = importDatabase('mysql', 'shop', new File(['dump'], 'shop.sql'));
    FakeXHR.last!.onerror?.();
    await expect(done).resolves.toMatchObject({ ok: false });
  });
});
