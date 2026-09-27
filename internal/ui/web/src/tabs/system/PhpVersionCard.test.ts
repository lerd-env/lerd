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

  it('marks a prerelease version so it is not set as a default by mistake', () => {
    render(PhpVersionCard, { props: { ...base, version: '8.6', patch: '8.6.0beta2', prerelease: true } });
    expect(screen.getByText('beta')).toBeInTheDocument();
    // The build string is too wide to sit beside the mark, so only the tooltip carries it.
    expect(screen.getByText('8.6')).toBeInTheDocument();
    expect(screen.getByRole('button').title).toContain('8.6.0beta2');
    expect(screen.getByRole('button').title).toContain('beta');
  });

  it('stays quiet when the image is on the current base', () => {
    render(PhpVersionCard, { props: base });
    expect(screen.queryByLabelText('Base image updated')).not.toBeInTheDocument();
  });

  // The page is already titled PHP, so a chip carries its state in a dot rather
  // than a logo and a pill.
  it('shows running or stopped as a dot, without repeating the PHP mark', () => {
    const { rerender } = render(PhpVersionCard, { props: base });
    expect(screen.getByLabelText('Running')).toBeInTheDocument();
    expect(screen.queryByText('PHP')).not.toBeInTheDocument();
    rerender({ ...base, running: false });
    expect(screen.getByLabelText('Stopped')).toBeInTheDocument();
  });
});
