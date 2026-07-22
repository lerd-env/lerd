import { describe, it, expect } from 'vitest';
import { escapeHtml } from './html';

describe('escapeHtml', () => {
  it('neutralises tags that would otherwise execute through {@html}', () => {
    expect(escapeHtml('<img src=x onerror=alert(1)>')).toBe(
      '&lt;img src=x onerror=alert(1)&gt;'
    );
    expect(escapeHtml('</code><script>alert(1)</script>')).toBe(
      '&lt;/code&gt;&lt;script&gt;alert(1)&lt;/script&gt;'
    );
  });

  it('escapes the quote characters that break out of an attribute', () => {
    expect(escapeHtml(`" onmouseover="alert(1)`)).toBe(
      '&quot; onmouseover=&quot;alert(1)'
    );
    expect(escapeHtml("' onfocus='alert(1)")).toBe('&#39; onfocus=&#39;alert(1)');
  });

  it('escapes ampersands first so entities are not double-decoded', () => {
    expect(escapeHtml('&lt;script&gt;')).toBe('&amp;lt;script&amp;gt;');
  });

  it('leaves ordinary values untouched', () => {
    expect(escapeHtml('192.168.0.200')).toBe('192.168.0.200');
    expect(escapeHtml('alice')).toBe('alice');
    expect(escapeHtml('*.test')).toBe('*.test');
  });

  it('renders null and undefined as an empty string rather than the literal word', () => {
    expect(escapeHtml(null)).toBe('');
    expect(escapeHtml(undefined)).toBe('');
  });
});
