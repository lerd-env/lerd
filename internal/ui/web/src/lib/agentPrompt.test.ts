import { describe, it, expect } from 'vitest';
import { requestAgentPrompt, doctorAgentPrompt } from './agentPrompt';

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
