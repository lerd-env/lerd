import { describe, it, expect } from 'vitest';
import { formatBytes } from './bytes';

describe('formatBytes', () => {
  it('renders zero and negatives as 0 B', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(-5)).toBe('0 B');
  });

  it('keeps raw bytes below 1 KB', () => {
    expect(formatBytes(512)).toBe('512 B');
  });

  it('shows one decimal under 10 units and rounds above', () => {
    expect(formatBytes(25690112)).toBe('24.5 MB');
    expect(formatBytes(18979224)).toBe('18.1 MB');
    expect(formatBytes(1024 * 1024 * 200)).toBe('200.0 MB');
  });
});
