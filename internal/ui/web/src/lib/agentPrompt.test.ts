import { describe, it, expect } from 'vitest';
import { requestAgentPrompt, doctorAgentPrompt, logAgentPrompt, logEntryReachable } from './agentPrompt';

describe('requestAgentPrompt', () => {
  it('names the site, the request and the rid the request tool reads it by', () => {
    const prompt = requestAgentPrompt('shop.test', { method: 'GET', uri: '/orders?page=2', status: 500 }, 'r-42');
    expect(prompt).toContain('lerd site shop.test');
    expect(prompt).toContain('GET /orders?page=2');
    expect(prompt).toContain('returned 500');
    expect(prompt).toContain('`request` tool (action "lenses", rid "r-42")');
  });
});

describe('doctorAgentPrompt', () => {
  it('names the failing checks by id and points the agent at site_doctor for the site', () => {
    const prompt = doctorAgentPrompt('shop.test', 'site "shop.test"', ['composer_deps', 'app_debug']);
    expect(prompt).toContain('lerd site shop.test');
    expect(prompt).toContain('composer_deps, app_debug');
    expect(prompt).toContain('`diag` tool (action "site_doctor", site "shop.test")');
  });
});

describe('logAgentPrompt', () => {
  it('quotes the entry and points the agent at the file through the logs tool', () => {
    const prompt = logAgentPrompt('shop.test', 'site "shop.test"', 'laravel.log', {
      level: 'ERROR',
      date: '2026-10-08 12:00:01',
      message: 'Undefined variable $order\nmore lines'
    });
    expect(prompt).toContain('lerd site shop.test');
    expect(prompt).toContain('(ERROR at 2026-10-08 12:00:01): "Undefined variable $order"');
    expect(prompt).not.toContain('more lines');
    expect(prompt).toContain('`logs` tool (action "fetch", source "app:laravel.log", site "shop.test", until "2026-10-08 12:00:01")');
  });

  it('cuts a long message so the prompt stays a pointer, not a paste', () => {
    const prompt = logAgentPrompt('shop.test', 'site "shop.test"', 'laravel.log', { message: 'x'.repeat(500) });
    expect(prompt).toContain('x'.repeat(200) + '…');
    expect(prompt).not.toContain('x'.repeat(201));
  });
});

describe('logEntryReachable', () => {
  it('only offers a dated entry inside the tail the logs tool reads', () => {
    expect(logEntryReachable({ date: '2026-10-08 12:00:01', message: 'boom' })).toBe(true);
    expect(logEntryReachable({ message: 'raw line' })).toBe(false);
    expect(logEntryReachable({ date: '2026-01-02 09:00:00', message: 'old', past_tail: true })).toBe(false);
  });
});
