import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';

import PhpVersionCard from './PhpVersionCard.svelte';

const base = {
  version: '8.4',
  patch: '8.4.12',
  running: true,
  isDefault: false,
  selected: false,
  onselect: () => {}
};

describe('PhpVersionCard', () => {
  it('flags a version whose base image was republished', () => {
    render(PhpVersionCard, { props: { ...base, updateAvailable: true } });
    expect(screen.getByLabelText('Base image updated')).toBeInTheDocument();
    expect(screen.getByRole('button').title).toContain('Rebuild to pick it up');
  });

  it('stays quiet when the image is on the current base', () => {
    render(PhpVersionCard, { props: base });
    expect(screen.queryByLabelText('Base image updated')).not.toBeInTheDocument();
  });
});
