import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import MobileHeader from './MobileHeader.svelte';

describe('MobileHeader', () => {
  it('keeps the bell beside the theme switcher on the right', () => {
    const { container } = render(MobileHeader);
    // justify-between spreads direct children, so a third one lands mid-bar.
    const bar = container.firstElementChild!;
    expect(bar.children).toHaveLength(2);
    expect(bar.lastElementChild!.querySelectorAll('button').length).toBeGreaterThanOrEqual(2);
  });
});
