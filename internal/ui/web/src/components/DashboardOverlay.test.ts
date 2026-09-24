import { render, screen, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import DashboardOverlay from './DashboardOverlay.svelte';
import { dashboardOpen, openDocs } from '../stores/dashboard';
import { profilerEnabled } from '../stores/profiler';
import { services } from '../stores/services';
import { serviceIcons } from '../stores/serviceIcons';

function openProfiler() {
  dashboardOpen.set({
    name: 'profiler',
    label: 'Profiler',
    dashboard: '/_spx/?SPX_UI_URI=/'
  });
}

describe('DashboardOverlay', () => {
  beforeEach(() => {
    dashboardOpen.set(null);
    profilerEnabled.set(false);
    services.set([]);
    serviceIcons.set({});
  });

  it('hides Back until the embedded iframe has somewhere to go back to', () => {
    openProfiler();
    render(DashboardOverlay);

    // Freshly opened: the SPX iframe has no internal history yet, so Back is a
    // dead end, and a greyed out arrow in the header only reads as clutter.
    expect(screen.queryByTitle('Back')).toBeNull();
  });

  it('shows the profiler toggle as off: muted, not pressed, no live dot', () => {
    profilerEnabled.set(false);
    openProfiler();
    const { container } = render(DashboardOverlay);

    const btn = screen.getByRole('button', { name: /start profiling/i });
    expect(btn.getAttribute('aria-pressed')).toBe('false');
    expect(btn.className).not.toMatch(/emerald/);
    expect(container.querySelector('.animate-pulse')).toBeNull();
  });

  // On the rail-coloured strip an outline alone vanished and grey-500 text fell
  // under AA, so the header buttons take the filled secondary tone pages use.
  it('draws the profiler header buttons in the filled secondary tone', () => {
    openProfiler();
    render(DashboardOverlay);

    for (const name of [/configuration/i, /clear data/i, /start profiling/i]) {
      const btn = screen.getByRole('button', { name });
      expect(btn.className).toContain('dark:bg-white/5');
      expect(btn.className).toContain('text-gray-700');
      expect(btn.className).not.toContain('text-gray-500');
    }
  });

  it('shows the profiler toggle as on: emerald, pressed, live pulsing dot', () => {
    profilerEnabled.set(true);
    openProfiler();
    const { container } = render(DashboardOverlay);

    const btn = screen.getByRole('button', { name: /stop profiling/i });
    expect(btn.getAttribute('aria-pressed')).toBe('true');
    expect(btn.className).toMatch(/emerald/);
    expect(container.querySelector('.animate-pulse')).not.toBeNull();
  });

  it('renders the documentation in place of an iframe', async () => {
    globalThis.fetch = (async () =>
      new Response(JSON.stringify({ pages: [] }), { status: 200 })) as unknown as typeof fetch;
    openDocs();
    const { container } = render(DashboardOverlay);

    expect(container.querySelector('iframe')).toBeNull();
    expect(screen.getByRole('searchbox')).toBeInTheDocument();
    // The header keeps a way out to the same page on the website.
    await waitFor(() =>
      expect(screen.getByTitle('Open in new tab').getAttribute('href')).toBe(
        'https://lerd.sh/getting-started/requirements'
      )
    );
  });

  it('collapses the SPX Configuration form by default on the control panel page', () => {
    openProfiler();
    render(DashboardOverlay);

    // The form starts hidden, so the header offers to show it.
    expect(screen.getByRole('button', { name: /show configuration/i })).toBeTruthy();
  });

  // The header names the service the frame belongs to, so it leads with the
  // mark the preset ships rather than the generic glyph for its category.
  // The strip above the frame is painted like the rail, as on every other page,
  // and the frame curves where the two meet.
  it('paints the header as chrome and rounds the frame corner under it', () => {
    dashboardOpen.set({ name: 'mailpit', label: 'Mailpit', dashboard: 'http://localhost:8025' });
    const { container } = render(DashboardOverlay);

    const iframe = container.querySelector('iframe')!;
    const frame = iframe.parentElement!;
    expect(frame.className).toContain('md:rounded-tl-xl');
    expect(frame.previousElementSibling!.className).toContain('page-header');
  });

  // The address is one click away behind the new-tab button; printed in the
  // header it was only lerd's proxy path.
  it('leaves the address to the new-tab button rather than printing it', () => {
    dashboardOpen.set({ name: 'pgadmin', label: 'pgAdmin', dashboard: '/_svc/pgadmin/' });
    const { container } = render(DashboardOverlay);

    expect(container.textContent).not.toContain('/_svc/pgadmin/');
    expect(screen.getByTitle(/new tab/i).getAttribute('href')).toContain('/_svc/pgadmin/');
  });

  it('heads the frame with the service mark, inked in the declared colour', () => {
    services.set([
      {
        name: 'mailpit',
        status: 'active',
        site_count: 0,
        category: 'mail',
        color: '#0f6cbd',
        dashboard: 'http://localhost:8025'
      }
    ]);
    serviceIcons.set({ mailpit: '<svg viewBox="0 0 24 24"><path d="M3 3h18v18H3z"/></svg>' });
    dashboardOpen.set({ name: 'mailpit', label: 'Mailpit', dashboard: 'http://localhost:8025' });
    const { container } = render(DashboardOverlay);

    const mark = container.querySelector('.mark-glyph path');
    expect(mark?.getAttribute('d')).toBe('M3 3h18v18H3z');
    expect(container.querySelector('.mark-brand')?.getAttribute('style')).toContain('--mark-tint: #0f6cbd');
  });

  // Switching dashboards must replace the frame, not point the old one somewhere
  // new. Navigating it runs the embedded app's beforeunload handler, and an admin
  // UI that registers one (pgAdmin) puts up a native confirm the overlay can't
  // dismiss: the header moves on while the frame stays stuck behind the dialog.
  it('builds a fresh frame for each dashboard instead of navigating the old one', async () => {
    dashboardOpen.set({ name: 'pgadmin', label: 'pgAdmin', dashboard: '/_svc/pgadmin/' });
    const { container } = render(DashboardOverlay);
    const first = container.querySelector('iframe');
    expect(first?.getAttribute('src')).toBe('/_svc/pgadmin/');

    dashboardOpen.set({ name: 'phpmyadmin', label: 'phpMyAdmin', dashboard: '/_svc/phpmyadmin/' });
    await waitFor(() => {
      expect(container.querySelector('iframe')?.getAttribute('src')).toBe('/_svc/phpmyadmin/');
    });
    expect(container.querySelector('iframe')).not.toBe(first);
  });

  // docs and profiler are not services and ship no mark; their built-in glyph
  // has to survive the switch to the shared icon.
  it('keeps the built-in glyph for a dashboard no service backs', () => {
    openProfiler();
    const { container } = render(DashboardOverlay);

    expect(container.querySelector('.mark-glyph')).toBeNull();
    expect(container.querySelector('header svg, .shrink-0 svg')).not.toBeNull();
  });
});
