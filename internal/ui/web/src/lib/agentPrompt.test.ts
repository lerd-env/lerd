import { describe, it, expect } from 'vitest';
import { requestAgentPrompt } from './agentPrompt';

describe('requestAgentPrompt', () => {
  it('names the site, the request and the rid the request tool reads it by', () => {
    const prompt = requestAgentPrompt('shop.test', { method: 'GET', uri: '/orders?page=2', status: 500 }, 'r-42');
    expect(prompt).toContain('lerd site shop.test');
    expect(prompt).toContain('GET /orders?page=2');
    expect(prompt).toContain('returned 500');
    expect(prompt).toContain('`request` tool (action "lenses", rid "r-42")');
  });
});
