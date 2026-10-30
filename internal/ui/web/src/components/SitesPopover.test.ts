import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import SitesPopover from './SitesPopover.svelte';
import { sites } from '$stores/sites';
import { frameworkMarks } from '$stores/frameworkMarks';

describe('SitesPopover', () => {
  beforeEach(() => {
    sites.set([
      { domain: 'running.test', fpm_running: true, framework: 'laravel' },
      { domain: 'paused.test', fpm_running: true, paused: true },
      { domain: 'stopped.test' }
    ]);
    frameworkMarks.set({
      laravel: { svg: '<svg viewBox="0 0 24 24"><path d="M3 3h18v18H3z"/></svg>', color: '#f05340' }
    });
    location.hash = '';
  });

  it('renders nothing when no site uses the thing', () => {
    const { container } = render(SitesPopover, { props: { domains: [] } });
    expect(container.textContent).toBe('');
  });

  it('collapses the sites into one control carrying the count', () => {
    render(SitesPopover, { props: { domains: ['running.test', 'stopped.test'] } });
    expect(screen.getByLabelText('2 sites')).toBeInTheDocument();
    expect(screen.queryByText('running.test')).not.toBeInTheDocument();
  });

  it('gives every row the state the sites store reports', async () => {
    render(SitesPopover, {
      props: { domains: ['running.test', 'paused.test', 'stopped.test', 'gone.test'] }
    });
    await fireEvent.click(screen.getByLabelText('4 sites'));

    const dot = (domain: string) =>
      screen.getByText(domain).previousElementSibling!.className;
    expect(dot('running.test')).toContain('bg-emerald-500');
    expect(dot('paused.test')).toContain('bg-amber-400');
    expect(dot('stopped.test')).toContain('bg-gray-300');
    expect(dot('gone.test')).toContain('bg-gray-300');
  });

  it('closes each row with the mark of the framework that site runs', async () => {
    render(SitesPopover, { props: { domains: ['running.test', 'stopped.test'] } });
    await fireEvent.click(screen.getByLabelText('2 sites'));

    const row = (domain: string) => screen.getByText(domain).closest('button')!;
    expect(row('running.test').querySelector('.mark-ink path')).not.toBeNull();
    expect(row('stopped.test').querySelector('.mark-ink')).toBeNull();
  });

  it('opens the site a row names and closes behind itself', async () => {
    render(SitesPopover, { props: { domains: ['running.test'] } });
    await fireEvent.click(screen.getByLabelText('1 sites'));
    await fireEvent.click(screen.getByText('running.test'));

    expect(location.hash).toBe('#sites/running.test');
    expect(screen.queryByText('running.test')).not.toBeInTheDocument();
  });

  // A pinned site keeps the service awake, so the control says so and the
  // list points at which site does it.
  it('marks the pinned sites when asked to', async () => {
    sites.set([{ domain: 'pinned.test', pinned: true }, { domain: 'plain.test' }]);
    const { container } = render(SitesPopover, { props: { domains: ['pinned.test', 'plain.test'], showPinned: true } });
    expect(container.querySelector('[data-testid="sites-coffee"]')).toBeTruthy();
    await fireEvent.click(screen.getByLabelText('2 sites'));
    expect(screen.getAllByTitle('Pinned, never idle-suspended')).toHaveLength(1);
    expect(screen.getByText('pinned.test').parentElement!.querySelector('[title="Pinned, never idle-suspended"]')).toBeTruthy();
  });

  it('leaves pins unmarked by default', () => {
    sites.set([{ domain: 'pinned.test', pinned: true }]);
    const { container } = render(SitesPopover, { props: { domains: ['pinned.test'] } });
    expect(container.querySelector('[data-testid="sites-coffee"]')).toBeNull();
  });
});
