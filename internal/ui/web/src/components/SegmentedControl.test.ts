import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import Harness from './SegmentedControl.test.svelte';

const options = [
  { value: 'app', label: 'App' },
  { value: 'testing', label: 'Testing' }
];

describe('SegmentedControl', () => {
  it('renders one button per option', () => {
    render(Harness, { props: { options, value: 'app' } });
    expect(screen.getByRole('button', { name: 'App' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Testing' })).toBeInTheDocument();
  });

  it('marks only the selected option as pressed', () => {
    render(Harness, { props: { options, value: 'testing' } });
    expect(screen.getByRole('button', { name: 'App' })).toHaveAttribute('aria-pressed', 'false');
    expect(screen.getByRole('button', { name: 'Testing' })).toHaveAttribute('aria-pressed', 'true');
  });

  it('reports the value when another option is clicked', () => {
    const onchange = vi.fn();
    render(Harness, { props: { options, value: 'app', onchange } });
    screen.getByRole('button', { name: 'Testing' }).click();
    expect(onchange).toHaveBeenCalledWith('testing');
  });

  it('stays quiet when the selected option is clicked again', () => {
    const onchange = vi.fn();
    render(Harness, { props: { options, value: 'app', onchange } });
    screen.getByRole('button', { name: 'App' }).click();
    expect(onchange).not.toHaveBeenCalled();
  });

  it('labels the group for assistive tech', () => {
    render(Harness, { props: { options, value: 'app', label: 'Target database' } });
    expect(screen.getByRole('group', { name: 'Target database' })).toBeInTheDocument();
  });
});
