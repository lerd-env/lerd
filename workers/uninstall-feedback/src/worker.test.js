import { test } from 'node:test';
import assert from 'node:assert/strict';
import worker, { validate } from './index.js';

function fakeDB() {
  const rows = [];
  return {
    rows,
    prepare: () => ({ bind: (...args) => ({ run: async () => rows.push(args) }) })
  };
}

const post = (body, path = '/v1/uninstall', ip = '203.0.113.7') =>
  new Request(`https://feedback.lerd.sh${path}`, {
    method: 'POST',
    headers: { 'cf-connecting-ip': ip },
    body: typeof body === 'string' ? body : JSON.stringify(body)
  });

// fakeLimit allows `max` calls per key, the way the rate-limit binding does.
function fakeLimit(max = 5) {
  const seen = new Map();
  return {
    limit: async ({ key }) => {
      seen.set(key, (seen.get(key) ?? 0) + 1);
      return { success: seen.get(key) <= max };
    }
  };
}

const good = { reason: 'heavy', comment: ' too much RAM ', version: '1.38.0', os: 'linux' };

test('stores a valid answer, trimmed', async () => {
  const db = fakeDB();
  const res = await worker.fetch(post(good), { DB: db, LIMIT: fakeLimit() });
  assert.equal(res.status, 204);
  assert.deepEqual(db.rows, [['heavy', 'too much RAM', '1.38.0', 'linux']]);
});

test('a comment is optional', () => {
  assert.equal(validate({ reason: 'trying', version: '1.38.0-beta.1', os: 'darwin' }).comment, '');
});

test('refuses what a lerd binary never sends', () => {
  for (const bad of [
    null,
    [],
    { ...good, reason: 'spam' },
    { ...good, os: 'plan9' },
    { ...good, version: '1.37.0-12-g9d514e93' },
    { ...good, version: 3 },
    { ...good, comment: 'x'.repeat(501) },
    { ...good, comment: {} }
  ]) {
    assert.equal(validate(bad), null, JSON.stringify(bad));
  }
});

test('bad JSON, oversize bodies, wrong method and path are refused', async () => {
  const db = fakeDB();
  assert.equal((await worker.fetch(post('{nope'), { DB: db, LIMIT: fakeLimit() })).status, 400);
  assert.equal((await worker.fetch(post('x'.repeat(5000)), { DB: db, LIMIT: fakeLimit() })).status, 413);
  assert.equal((await worker.fetch(post(good, '/'), { DB: db, LIMIT: fakeLimit() })).status, 404);
  const get = new Request('https://feedback.lerd.sh/v1/uninstall');
  assert.equal((await worker.fetch(get, { DB: db, LIMIT: fakeLimit() })).status, 405);
  assert.equal(db.rows.length, 0);
});

test('a flood from one address is refused and not stored', async () => {
  const db = fakeDB();
  const env = { DB: db, LIMIT: fakeLimit(5) };
  const codes = [];
  for (let i = 0; i < 7; i++) codes.push((await worker.fetch(post(good), env)).status);
  assert.deepEqual(codes, [204, 204, 204, 204, 204, 429, 429]);
  assert.equal(db.rows.length, 5);
  assert.equal((await worker.fetch(post(good, '/v1/uninstall', '198.51.100.9'), env)).status, 204);
});

// A body with no length header still has to be cut off at the cap, not read
// whole first.
test('an oversize streamed body is refused without reading it all', async () => {
  let pulled = 0;
  const stream = new ReadableStream({
    pull(c) {
      pulled++;
      if (pulled > 1000) return c.close();
      c.enqueue(new Uint8Array(1024));
    }
  });
  const req = new Request('https://feedback.lerd.sh/v1/uninstall', {
    method: 'POST', body: stream, duplex: 'half', headers: { 'cf-connecting-ip': '192.0.2.1' }
  });
  const res = await worker.fetch(req, { DB: fakeDB(), LIMIT: fakeLimit() });
  assert.equal(res.status, 413);
  assert.ok(pulled < 20, `read ${pulled} KiB before refusing`);
});
