import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import ToolCard from './ToolCard.svelte';
import type { ToolStatus } from '$stores/status';

const tool = (over: Partial<ToolStatus>): ToolStatus => ({
  name: 'mkcert',
  installed: 'v1.4.4',
  pinned: 'v1.4.4',
  present: true,
  update_available: false,
  ...over
});

describe('ToolCard', () => {
  it('shows the installed version for an up-to-date tool', () => {
    render(ToolCard, { props: { tool: tool({}) } });
    expect(screen.getByText('mkcert')).toBeInTheDocument();
    expect(screen.getByText('v1.4.4')).toBeInTheDocument();
  });

  it('flags an available update with the pinned version', () => {
    render(ToolCard, {
      props: { tool: tool({ name: 'composer', installed: '2.10.2', pinned: '2.10.3', update_available: true }) }
    });
    expect(screen.getByText('2.10.2')).toBeInTheDocument();
    expect(screen.getByText(/2\.10\.3/)).toBeInTheDocument();
  });

  it('dims the pin for a missing tool', () => {
    render(ToolCard, { props: { tool: tool({ present: false, installed: '' }) } });
    const version = screen.getByText('v1.4.4');
    expect(version.className).toContain('text-gray-400');
  });
});
