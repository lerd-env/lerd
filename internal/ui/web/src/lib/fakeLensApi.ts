import { vi } from 'vitest';
import type { DumpEvent } from '$lib/dumpEvent';
import { fakeLensAnswer } from '$lib/fakeLensAnswer';

export { fakeLensAnswer };

// A test double for lerd-ui's Debug lens endpoints and its nudge stream, for
// component tests, answering through fakeLensAnswer.

export interface FakeLensApi {
  events: DumpEvent[];
  requests: string[];
  // enabled is what /api/dumps/status reports for the debug bridge.
  enabled: boolean;
  // nudge tells every open lens an event of kind arrived for site.
  nudge(kind: string, site: string): void;
  restore(): void;
}

export function installFakeLensApi(events: DumpEvent[] = []): FakeLensApi {
  const api: FakeLensApi = { events, requests: [], enabled: true, nudge: () => {}, restore: () => {} };
  const sources = new Set<{ url: string; fire: (data: string) => void }>();

  const realFetch = globalThis.fetch;
  const realES = globalThis.EventSource;
  globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input), 'http://lerd.test');
    api.requests.push(url.pathname + url.search);
    return new Response(JSON.stringify(fakeLensAnswer(api.events, url.pathname + url.search, api.enabled)), { status: 200 });
  }) as unknown as typeof fetch;

  class FakeSource {
    listeners: Array<(e: { data: string }) => void> = [];
    entry: { url: string; fire: (data: string) => void };
    constructor(url: string) {
      this.entry = { url, fire: (data) => this.listeners.forEach((l) => l({ data })) };
      sources.add(this.entry);
    }
    addEventListener(type: string, fn: (e: { data: string }) => void) {
      if (type === 'message') this.listeners.push(fn);
    }
    close() {
      sources.delete(this.entry);
    }
  }
  // @ts-expect-error test double
  globalThis.EventSource = FakeSource;

  api.nudge = (kind, site) => {
    for (const s of sources) s.fire(JSON.stringify({ kind, site }));
  };
  api.restore = () => {
    globalThis.fetch = realFetch;
    globalThis.EventSource = realES;
  };
  return api;
}
