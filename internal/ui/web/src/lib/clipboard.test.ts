import { describe, it, expect, vi, beforeEach } from 'vitest';
import { copyText } from './clipboard';

describe('copyText', () => {
  beforeEach(() => {
    Object.assign(navigator, { clipboard: undefined });
  });

  it('uses the clipboard API when it works', async () => {
    const writeText = vi.fn(async () => {});
    Object.assign(navigator, { clipboard: { writeText } });
    expect(await copyText('A=1')).toBe(true);
    expect(writeText).toHaveBeenCalledWith('A=1');
  });

  it('falls back to a selection copy when the API refuses', async () => {
    Object.assign(navigator, {
      clipboard: {
        writeText: async () => {
          throw new Error('document is not focused');
        }
      }
    });
    let copied = '';
    document.execCommand = vi.fn(() => {
      copied = document.querySelector('textarea')?.value ?? '';
      return true;
    });
    expect(await copyText('A=1')).toBe(true);
    expect(copied).toBe('A=1');
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('reports failure when neither path copies', async () => {
    document.execCommand = vi.fn(() => false);
    expect(await copyText('A=1')).toBe(false);
  });
});
