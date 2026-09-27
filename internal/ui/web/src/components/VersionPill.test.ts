import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import VersionPill from './VersionPill.svelte';
import { version } from '$stores/version';

describe('VersionPill', () => {
  // Side by side, a badge in the rail's tiny size looked like a leftover next to
  // the version beside it.
  it('draws the channel badge the same size as the version', () => {
    version.set({ current: '1.35.0-48-ga75062ab', latest: '', hasUpdate: false, checked: true, checking: false, changelog: '' });
    const { getByText } = render(VersionPill);
    const sizing = (el: HTMLElement) =>
      el.className.split(/\s+/).filter((c) => /^(px|py|text-(xs|\[))/.test(c)).sort();
    expect(sizing(getByText('dev'))).toEqual(sizing(getByText('v1.35.0')));
  });
});
