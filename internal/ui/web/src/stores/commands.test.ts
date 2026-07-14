import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';
import {
  runCommand,
  launchCommand,
  closeRun,
  currentRun,
  runningName,
  runToast,
  runSettled,
  type Command
} from './commands';

// Build a ReadableStream that emits a sequence of UTF-8 chunks. Used to
// simulate the SSE response body for runCommand without hitting the network.
function makeStream(chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  let i = 0;
  return new ReadableStream({
    pull(controller) {
      if (i < chunks.length) {
        controller.enqueue(encoder.encode(chunks[i++]));
      } else {
        controller.close();
      }
    }
  });
}

function mockSSE(chunks: string[]) {
  globalThis.fetch = vi.fn().mockResolvedValue(
    new Response(makeStream(chunks), {
      status: 200,
      headers: { 'Content-Type': 'text/event-stream' }
    })
  ) as unknown as typeof fetch;
}

function mockJSON(body: unknown) {
  globalThis.fetch = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'Content-Type': 'application/json' }
    })
  ) as unknown as typeof fetch;
}

beforeEach(() => {
  closeRun();
  vi.restoreAllMocks();
});

describe('runCommand (SSE parser)', () => {
  it('dispatches stdout lines as they arrive', async () => {
    mockSSE(['event: stdout\ndata: first\n\n', 'event: stdout\ndata: second\n\n', 'event: done\ndata: {"exit":0,"durationMs":42}\n\n']);
    const stdout: string[] = [];
    let exit: number | null = null;
    await runCommand('acme.test', 'echo', {
      onStdout: (l) => stdout.push(l),
      onDone: ({ exit: e }) => (exit = e)
    });
    expect(stdout).toEqual(['first', 'second']);
    expect(exit).toBe(0);
  });

  it('handles a single chunk containing multiple events', async () => {
    mockSSE(['event: stdout\ndata: a\n\nevent: stdout\ndata: b\n\nevent: done\ndata: {"exit":0,"durationMs":1}\n\n']);
    const stdout: string[] = [];
    await runCommand('acme.test', 'echo', { onStdout: (l) => stdout.push(l) });
    expect(stdout).toEqual(['a', 'b']);
  });

  it('handles events split across multiple chunks', async () => {
    mockSSE(['event: stdout\nda', 'ta: partial\n\nevent: done\ndata: {"exit":0,"durationMs":1}\n\n']);
    const stdout: string[] = [];
    await runCommand('acme.test', 'echo', { onStdout: (l) => stdout.push(l) });
    expect(stdout).toEqual(['partial']);
  });

  it('dispatches stderr separately from stdout', async () => {
    mockSSE(['event: stdout\ndata: ok\n\n', 'event: stderr\ndata: warn\n\n', 'event: done\ndata: {"exit":0,"durationMs":1}\n\n']);
    const out: string[] = [];
    const err: string[] = [];
    await runCommand('acme.test', 'echo', { onStdout: (l) => out.push(l), onStderr: (l) => err.push(l) });
    expect(out).toEqual(['ok']);
    expect(err).toEqual(['warn']);
  });

  it('surfaces a URL from the done payload', async () => {
    mockSSE(['event: done\ndata: {"exit":0,"durationMs":12,"url":"https://acme.test/login/x"}\n\n']);
    let url: string | undefined;
    await runCommand('acme.test', 'uli', { onDone: (d) => (url = d.url) });
    expect(url).toBe('https://acme.test/login/x');
  });

  it('falls through to onTerminal when server returns terminal:true JSON instead of SSE', async () => {
    mockJSON({ terminal: true });
    const events = { terminal: 0, done: 0 };
    await runCommand('acme.test', 'shell', {
      onTerminal: () => events.terminal++,
      onDone: () => events.done++
    });
    expect(events.terminal).toBe(1);
    expect(events.done).toBe(0);
  });

  it('reports server errors via onError', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response('forbidden', { status: 403, statusText: 'Forbidden' })
    ) as unknown as typeof fetch;
    let msg: string | undefined;
    await runCommand('acme.test', 'x', { onError: (m) => (msg = m) });
    expect(msg).toContain('403');
  });
});

describe('launchCommand', () => {
  const cmd = (overrides: Partial<Command> = {}): Command => ({
    name: 'echo',
    label: 'Echo',
    command: 'echo hi',
    ...overrides
  });

  it('parks in confirm state for confirm: true commands', () => {
    launchCommand('acme.test', cmd({ confirm: true }));
    const s = get(currentRun);
    expect(s.kind).toBe('confirm');
    if (s.kind !== 'confirm') return;
    expect(s.domain).toBe('acme.test');
    expect(s.cmd.name).toBe('echo');
  });

  // Regression: the confirm state must carry the branch so Run Anyway targets the
  // worktree the command was launched for, not the parent checkout.
  it('carries the worktree branch into the confirm state', () => {
    launchCommand('acme.test', cmd({ confirm: true }), { branch: 'feat-x' });
    const s = get(currentRun);
    expect(s.kind).toBe('confirm');
    if (s.kind !== 'confirm') return;
    expect(s.branch).toBe('feat-x');
  });

  it('skips confirm when skipConfirm: true', async () => {
    mockSSE(['event: done\ndata: {"exit":0,"durationMs":1}\n\n']);
    launchCommand('acme.test', cmd({ confirm: true }), { skipConfirm: true });
    // launchCommand fires execute asynchronously; wait a tick for the state to land.
    await new Promise((r) => setTimeout(r, 0));
    const s = get(currentRun);
    expect(s.kind).not.toBe('confirm');
  });
});

describe('closeRun', () => {
  it('resets state back to idle and clears runningName', () => {
    launchCommand('acme.test', { name: 'noop', label: 'Noop', command: 'true', confirm: true });
    expect(get(currentRun).kind).toBe('confirm');
    closeRun();
    expect(get(currentRun).kind).toBe('idle');
    expect(get(runningName)).toBeNull();
  });
});

describe('history', () => {
  beforeEach(() => localStorage.clear());

  it('persists the last run and exposes it via lastRunFor', async () => {
    const { lastRunFor } = await import('./commands');
    mockSSE(['event: stdout\ndata: hi\n\n', 'event: done\ndata: {"exit":0,"durationMs":12}\n\n']);
    launchCommand('acme.test', { name: 'echo', label: 'Echo', command: 'echo hi' });
    // wait for executeCommand to complete
    await new Promise((r) => setTimeout(r, 50));
    const prev = lastRunFor('acme.test', 'echo');
    expect(prev).not.toBeNull();
    expect(prev!.exit).toBe(0);
    expect(prev!.lines).toEqual([{ stream: 'stdout', text: 'hi' }]);
  });

  it('returns null for unknown (domain, name) pairs', async () => {
    const { lastRunFor } = await import('./commands');
    expect(lastRunFor('nope.test', 'nope')).toBeNull();
  });

  it('survives malformed localStorage payloads', async () => {
    localStorage.setItem('lerd-commands-history-v1', 'not json');
    const { lastRunFor } = await import('./commands');
    expect(lastRunFor('a', 'b')).toBeNull();
  });
});

describe('output: silent', () => {
  beforeEach(() => localStorage.clear());

  it('runs without opening the modal and toasts when it succeeds', async () => {
    mockSSE(['event: stdout\ndata: cache cleared\n\n', 'event: done\ndata: {"exit":0,"durationMs":8}\n\n']);
    launchCommand('acme.test', { name: 'optimize:clear', label: 'Clear all caches', command: 'php artisan optimize:clear', output: 'silent' });
    await new Promise((r) => setTimeout(r, 50));
    expect(get(currentRun).kind).toBe('idle');
    expect(get(runToast)).toBe('Clear all caches finished');
  });

  // The output is the only thing that explains a failure, so a silent command
  // that exits non-zero still surfaces the modal.
  it('opens the modal with the output when it fails', async () => {
    mockSSE(['event: stderr\ndata: boom\n\n', 'event: done\ndata: {"exit":1,"durationMs":8}\n\n']);
    launchCommand('acme.test', { name: 'migrate', label: 'Run migrations', command: 'php artisan migrate', output: 'silent' });
    await new Promise((r) => setTimeout(r, 50));
    const s = get(currentRun);
    expect(s.kind).toBe('done');
    if (s.kind !== 'done') return;
    expect(s.exit).toBe(1);
    expect(s.lines).toEqual([{ stream: 'stderr', text: 'boom' }]);
  });

  it('still records run history', async () => {
    const { lastRunFor } = await import('./commands');
    mockSSE(['event: stdout\ndata: done\n\n', 'event: done\ndata: {"exit":0,"durationMs":9}\n\n']);
    launchCommand('acme.test', { name: 'key:generate', label: 'Generate key', command: 'php artisan key:generate', output: 'silent' });
    await new Promise((r) => setTimeout(r, 50));
    const prev = lastRunFor('acme.test', 'key:generate');
    expect(prev).not.toBeNull();
    expect(prev!.exit).toBe(0);
    expect(prev!.lines).toEqual([{ stream: 'stdout', text: 'done' }]);
  });
});

describe('runSettled', () => {
  it('resolves immediately when nothing is pending', async () => {
    await expect(runSettled()).resolves.toBeUndefined();
  });

  // The doctor's Fix button launches through the confirm gate, so it must not
  // re-check while the prompt is still on screen waiting for the user.
  it('stays pending while a confirmation is waiting, and resolves once the run finishes', async () => {
    mockSSE(['event: done\ndata: {"exit":0,"durationMs":3}\n\n']);
    const cmd: Command = { name: 'migrate:fresh', label: 'Fresh', command: 'php artisan migrate:fresh', confirm: true };
    launchCommand('acme.test', cmd);
    expect(get(currentRun).kind).toBe('confirm');

    let settled = false;
    const pending = runSettled().then(() => (settled = true));
    await new Promise((r) => setTimeout(r, 10));
    expect(settled).toBe(false);

    // The user confirms; the run goes through and settles.
    const { executeCommand } = await import('./commands');
    void executeCommand('acme.test', cmd, '', true);
    await pending;
    expect(settled).toBe(true);
  });

  it('resolves when the user cancels the confirmation', async () => {
    launchCommand('acme.test', { name: 'x', label: 'X', command: 'true', confirm: true });
    expect(get(currentRun).kind).toBe('confirm');
    const pending = runSettled();
    closeRun();
    await expect(pending).resolves.toBeUndefined();
  });
});

describe('launchCommand concurrency guard', () => {
  it('refuses if another run is in flight', async () => {
    mockSSE(['event: done\ndata: {"exit":0,"durationMs":1}\n\n']);
    launchCommand('acme.test', { name: 'first', label: 'First', command: 'echo a' });
    // While the first is running, fire a second.
    launchCommand('acme.test', { name: 'second', label: 'Second', command: 'echo b' });
    await new Promise((r) => setTimeout(r, 10));
    // The first command's state must dominate; second was rejected.
    const s = get(currentRun);
    if (s.kind === 'done' || s.kind === 'running' || s.kind === 'confirm') {
      expect(s.cmd.name).toBe('first');
    }
  });
});
