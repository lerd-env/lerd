import { render, screen, cleanup } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { currentRun, closeRun } from '$stores/commands';
import Harness from './CommandRunModal.test.svelte';

beforeEach(() => {
  closeRun();
});
afterEach(() => {
  closeRun();
  cleanup();
});

describe('CommandRunModal', () => {
  it('renders nothing when state is idle', () => {
    const { container } = render(Harness);
    // Modal only renders when currentRun.kind is not idle.
    expect(container.querySelector('[aria-label="Close"]')).toBeNull();
    expect(container.querySelector('[aria-label="Cancel"]')).toBeNull();
  });

  it('renders the confirm view with command label and shell', async () => {
    render(Harness);
    currentRun.set({
      kind: 'confirm',
      domain: 'acme.test',
      cmd: { name: 'migrate:fresh', label: 'Fresh migrate', command: 'php artisan migrate:fresh --force', confirm: true },
      branch: ''
    });
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.getByText(/Run Fresh migrate/)).toBeInTheDocument();
    expect(screen.getByText(/acme\.test/)).toBeInTheDocument();
    expect(screen.getByText(/php artisan migrate:fresh --force/)).toBeInTheDocument();
    // Two buttons match Cancel (backdrop aria-label + footer text content).
    expect(screen.getAllByRole('button', { name: 'Cancel' }).length).toBeGreaterThanOrEqual(1);
    expect(screen.getByRole('button', { name: 'Run anyway' })).toBeInTheDocument();
  });

  it('renders the running view with running pill and waiting message', async () => {
    render(Harness);
    currentRun.set({
      kind: 'running',
      domain: 'acme.test',
      cmd: { name: 'echo', label: 'Echo', command: 'echo hi' },
      lines: [],
      started: Date.now()
    });
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.getByText('running')).toBeInTheDocument();
    expect(screen.getByText(/waiting for output/)).toBeInTheDocument();
  });

  it('renders the done view with exit 0 pill and duration', async () => {
    render(Harness);
    currentRun.set({
      kind: 'done',
      domain: 'acme.test',
      cmd: { name: 'echo', label: 'Echo', command: 'echo hi' },
      lines: [{stream: 'stdout', text: 'hello'}],
      exit: 0,
      durationMs: 42
    });
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.getByText(/exit 0/)).toBeInTheDocument();
    expect(screen.getByText(/42ms/)).toBeInTheDocument();
    expect(screen.getByText(/hello/)).toBeInTheDocument();
  });

  it('renders red exit pill for non-zero exit', async () => {
    const { container } = render(Harness);
    currentRun.set({
      kind: 'done',
      domain: 'acme.test',
      cmd: { name: 'fail', label: 'Fail', command: 'false-cmd' },
      lines: [{stream: 'stdout', text: 'boom'}],
      exit: 7,
      durationMs: 12
    });
    await new Promise((r) => setTimeout(r, 0));
    // Pick the red exit pill specifically (it has bg-red-100).
    const pill = container.querySelector('span.bg-red-100, span.bg-red-900\\/40');
    expect(pill?.textContent?.trim()).toContain('exit 7');
  });

  it('renders the URL panel for output: url commands', async () => {
    render(Harness);
    currentRun.set({
      kind: 'done',
      domain: 'acme.test',
      cmd: { name: 'uli', label: 'Login', command: 'drush uli', output: 'url' },
      lines: [{stream: 'stdout', text: 'https://acme.test/login/abc'}],
      exit: 0,
      durationMs: 100,
      url: 'https://acme.test/login/abc'
    });
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.getByText('One-time URL')).toBeInTheDocument();
    expect(screen.getAllByText(/https:\/\/acme\.test\/login\/abc/).length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: 'Copy' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open' })).toBeInTheDocument();
  });

  it('closes on Cancel click in confirm view', async () => {
    const { container } = render(Harness);
    currentRun.set({
      kind: 'confirm',
      domain: 'acme.test',
      cmd: { name: 'x', label: 'X', command: 'true', confirm: true },
      branch: ''
    });
    await new Promise((r) => setTimeout(r, 0));
    // Two buttons match "Cancel" (backdrop has aria-label and the footer Cancel
    // button has text content). Pick the footer one.
    const cancels = screen.getAllByRole('button', { name: 'Cancel' });
    const footer = cancels.find((b) => b.textContent?.includes('Cancel'))!;
    footer.click();
    await new Promise((r) => setTimeout(r, 0));
    // The modal markup should be gone once state is idle.
    expect(container.querySelector('[aria-label="Cancel"]')).toBeNull();
  });

  it('closes on Escape keypress when modal is open', async () => {
    const { container } = render(Harness);
    currentRun.set({
      kind: 'done',
      domain: 'acme.test',
      cmd: { name: 'x', label: 'X', command: 'true' },
      lines: [{stream: 'stdout', text: 'ok'}],
      exit: 0,
      durationMs: 10
    });
    await new Promise((r) => setTimeout(r, 0));
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await new Promise((r) => setTimeout(r, 0));
    expect(container.querySelector('[aria-label="Close"]')).toBeNull();
  });
});
