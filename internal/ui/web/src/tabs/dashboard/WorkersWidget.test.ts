import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import WorkersWidget from './WorkersWidget.svelte';
import { services } from '$stores/services';
import { sites } from '$stores/sites';
import { unhealthyWorkers } from '$stores/workerHealth';
import { workerMarks } from '$stores/workerMarks';
import { frameworkMarks } from '$stores/frameworkMarks';

beforeEach(() => {
  workerMarks.set({ workers: { 'laravel/queue': { icon: 'queue', color: '#ff2d20' } }, marks: {} });
  sites.set([
    { domain: 'shop.test', name: 'shop', framework: 'laravel', framework_label: 'Laravel' }
  ] as never);
  services.set([
    { name: 'queue-shop', status: 'active', queue_site: 'shop' }
  ] as never);
  localStorage.clear();
  frameworkMarks.set({});
  unhealthyWorkers.set([]);
});

describe('WorkersWidget', () => {
  // Group headings, told apart from the rows by the heading's own truncate span.
  function groupLabels(container: HTMLElement): string[] {
    return [...container.querySelectorAll('.flex-1.truncate.text-left')].map(
      (n) => n.textContent?.trim() ?? ''
    );
  }

  it('leads with the group running the most workers', () => {
    sites.set([
      { domain: 'shop.test', name: 'shop' },
      { domain: 'blog.test', name: 'blog' }
    ] as never);
    services.set([
      { name: 'horizon-shop', status: 'active', horizon_site: 'shop' },
      { name: 'queue-shop', status: 'active', queue_site: 'shop' },
      { name: 'queue-blog', status: 'active', queue_site: 'blog' }
    ] as never);
    const { container } = render(WorkersWidget);
    expect(groupLabels(container)).toEqual(['Queues', 'Horizon']);
  });

  it('breaks a tie on active workers with the size of the group', () => {
    sites.set([
      { domain: 'shop.test', name: 'shop' },
      { domain: 'blog.test', name: 'blog' }
    ] as never);
    services.set([
      { name: 'horizon-shop', status: 'active', horizon_site: 'shop' },
      { name: 'queue-shop', status: 'active', queue_site: 'shop' },
      { name: 'queue-blog', status: 'inactive', queue_site: 'blog' }
    ] as never);
    const { container } = render(WorkersWidget);
    expect(groupLabels(container)).toEqual(['Queues', 'Horizon']);
  });

  it('names the site once and leaves the worker type to the heading', () => {
    render(WorkersWidget);
    expect(screen.getByText('shop')).toBeInTheDocument();
    expect(screen.queryByText(/Queue Worker/)).toBeNull();
    expect(screen.queryByText(/Laravel/)).toBeNull();
  });

  it('marks each row when one group spans two frameworks', () => {
    workerMarks.set({
      workers: { 'laravel/queue': { icon: 'queue', color: '#ff2d20' } },
      marks: {}
    });
    frameworkMarks.set({
      laravel: { svg: '<svg viewBox="0 0 24 24"><path d="M5 5h2v2H5z"/></svg>', color: '#ff2d20' },
      symfony: { svg: '<svg viewBox="0 0 24 24"><path d="M8 8h2v2H8z"/></svg>', color: '#000000' }
    });
    sites.set([
      { domain: 'shop.test', name: 'shop', framework: 'laravel', framework_label: 'Laravel' },
      { domain: 'intranet.test', name: 'intranet', framework: 'symfony', framework_label: 'Symfony' }
    ] as never);
    services.set([
      { name: 'queue-shop', status: 'active', queue_site: 'shop' },
      { name: 'queue-intranet', status: 'active', queue_site: 'intranet' }
    ] as never);

    const { container } = render(WorkersWidget);
    expect(container.innerHTML).toContain('M5 5h2v2H5z');
    expect(container.innerHTML).toContain('M8 8h2v2H8z');
  });

  it('leaves the rows unmarked while a group holds one framework', () => {
    frameworkMarks.set({
      laravel: { svg: '<svg viewBox="0 0 24 24"><path d="M5 5h2v2H5z"/></svg>', color: '#ff2d20' }
    });
    const { container } = render(WorkersWidget);
    // The heading's own mark carries the framework through its tone, so the
    // rows say nothing about it.
    expect(container.innerHTML).not.toContain('M5 5h2v2H5z');
  });

  it('summarises a group by state rather than by fraction', () => {
    render(WorkersWidget);
    expect(screen.getByText('Queues')).toBeInTheDocument();
    expect(screen.getByText('1 up')).toBeInTheDocument();
  });

  it('starts a healthy group collapsed', () => {
    render(WorkersWidget);
    expect(screen.getByRole('button', { name: /Queues/ }).getAttribute('aria-expanded')).toBe(
      'false'
    );
  });

  it('opens a group holding a failing worker without being asked', () => {
    unhealthyWorkers.set([
      { site: 'shop', worker: 'queue', unit: 'lerd-queue-shop', state: 'failed' }
    ]);
    render(WorkersWidget);
    expect(screen.getByRole('button', { name: /Queues/ }).getAttribute('aria-expanded')).toBe(
      'true'
    );
  });

  it('keeps a group the user closed closed, failing or not', async () => {
    unhealthyWorkers.set([
      { site: 'shop', worker: 'queue', unit: 'lerd-queue-shop', state: 'failed' }
    ]);
    render(WorkersWidget);
    const head = screen.getByRole('button', { name: /Queues/ });
    head.click();
    await vi.waitFor(() => expect(head.getAttribute('aria-expanded')).toBe('false'));
  });

  it('puts a failing unit’s last error on the tile itself', () => {
    unhealthyWorkers.set([
      {
        site: 'shop',
        worker: 'queue',
        unit: 'lerd-queue-shop',
        state: 'failed',
        last_error: 'exit 255 connection refused'
      }
    ]);
    render(WorkersWidget);
    expect(screen.getByText('exit 255 connection refused')).toBeInTheDocument();
  });

  it('shows a suspended worker as asleep rather than dropping it', () => {
    services.set([] as never);
    sites.set([
      {
        domain: 'shop.test',
        name: 'shop',
        framework: 'laravel',
        framework_label: 'Laravel',
        idle_suspended_workers: ['queue']
      }
    ] as never);
    render(WorkersWidget);
    expect(screen.getByText('shop')).toBeInTheDocument();
    expect(screen.getByText('idle')).toBeInTheDocument();
  });
});
