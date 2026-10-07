import { describe, it, expect, vi, afterEach } from 'vitest';
import { loadDoctor, type DoctorCheck } from './doctor';

const realFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = realFetch;
});

function mockResponse(body: string, contentType: string) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(body, { status: 200, headers: { 'Content-Type': contentType } }));
  globalThis.fetch = fetchMock;
  return fetchMock;
}

describe('loadDoctor', () => {
  it('hands each check over as it streams in and resolves with the final report', async () => {
    const a = { name: 'app_key', status: 'ok' };
    const b = { name: 'migrations', status: 'fail' };
    const report = { checks: [a, b], failures: 1, warnings: 0 };
    const fetchMock = mockResponse(
      `event: check\ndata: ${JSON.stringify(a)}\n\nevent: check\ndata: ${JSON.stringify(b)}\n\nevent: done\ndata: ${JSON.stringify(report)}\n\n`,
      'text/event-stream'
    );

    const seen: DoctorCheck[] = [];
    const got = await loadDoctor('acme.test', 'feat', (c) => seen.push(c));

    expect(fetchMock.mock.calls[0][0]).toBe('/api/sites/acme.test/doctor?stream=1&branch=feat');
    expect(seen.map((c) => c.name)).toEqual(['app_key', 'migrations']);
    expect(got).toEqual(report);
  });

  it('surfaces the error the route answers with as JSON instead of a stream', async () => {
    mockResponse(JSON.stringify({ error: 'unknown worktree branch' }), 'application/json');
    await expect(loadDoctor('acme.test', 'ghost')).rejects.toThrow('unknown worktree branch');
  });

  it('fails when the stream ends without a report', async () => {
    mockResponse(`event: check\ndata: {"name":"a","status":"ok"}\n\n`, 'text/event-stream');
    await expect(loadDoctor('acme.test')).rejects.toThrow();
  });
});
