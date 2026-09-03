import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import ServiceSnapshotsTab from './ServiceSnapshotsTab.svelte';
import { autoSnapshot, type AutoSnapshotPolicy } from '$stores/autoSnapshot';
import { databases, type DatabaseEngine } from '$stores/databases';
import type { Service } from '$stores/services';

const policy: AutoSnapshotPolicy = {
  enabled: false,
  every: '24h0m0s',
  keep: 7,
  keep_for: '',
  selection: 'opt_out',
  sites: [
    {
      site: 'shop',
      service: 'mysql',
      database: 'shop_db',
      mode: '',
      covered: false,
      last: '2026-07-20T10:00:00Z',
      next: '2026-07-21T10:00:00Z'
    },
    { site: 'blog', service: 'mysql', database: 'blog_db', mode: 'off', covered: false },
    // Another engine's site: this tab must not claim it.
    { site: 'metrics', service: 'postgres', database: 'metrics', mode: '', covered: false }
  ]
};

const engine: DatabaseEngine = {
  service: 'mysql',
  family: 'mysql',
  status: 'active',
  supports_create: true,
  supports_drop: true,
  supports_export: true,
  supports_import: true,
  supports_snapshot: true,
  databases: [
    {
      name: 'shop_db',
      size_bytes: 4096,
      site: 'shop.test',
      snapshots: [
        {
          name: 'auto-20260720-100000',
          created: '2026-07-20T10:00:00Z',
          database: 'shop_db',
          size_bytes: 2048,
          site: 'shop.test',
          auto: true,
          expires_at: '2026-07-27T10:00:00Z',
          estimated: true
        }
      ]
    },
    {
      name: 'blog_db',
      size_bytes: 2048,
      site: 'blog.test',
      snapshots: [
        {
          name: 'by-hand',
          created: '2026-07-18T10:00:00Z',
          database: 'blog_db',
          size_bytes: 1024,
          site: 'blog.test'
        }
      ]
    }
  ]
};

const svc = { name: 'mysql', is_database: true } as Service;

const {
  loadAutoSnapshot,
  saveAutoSnapshot,
  loadEngine,
  keepSnapshot,
  deleteSnapshot,
  restoreSnapshot
} = vi.hoisted(
  () => ({
    loadAutoSnapshot: vi.fn(async () => {}),
    saveAutoSnapshot: vi.fn(async () => ({ ok: true })),
    loadEngine: vi.fn(async () => {}),
    keepSnapshot: vi.fn(async () => ({ ok: true })),
    deleteSnapshot: vi.fn(async () => ({ ok: true })),
    restoreSnapshot: vi.fn(async () => ({ ok: true }))
  })
);
vi.mock('$stores/autoSnapshot', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, loadAutoSnapshot, saveAutoSnapshot };
});
vi.mock('$stores/databases', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, loadEngine, keepSnapshot, deleteSnapshot, restoreSnapshot };
});

// pagedEngine holds one page of snapshots plus one, on a single database.
function pagedEngine(count: number): DatabaseEngine {
  return {
    ...engine,
    databases: [
      {
        name: 'shop_db',
        size_bytes: 4096,
        site: 'shop.test',
        snapshots: Array.from({ length: count }, (_, i) => ({
          name: `auto-2026072${i % 10}-1000${String(i).padStart(2, '0')}`,
          created: new Date(Date.now() - i * 3600_000).toISOString(),
          database: 'shop_db',
          size_bytes: 1024,
          site: 'shop.test',
          auto: true
        }))
      }
    ]
  };
}

describe('ServiceSnapshotsTab', () => {
  beforeEach(() => {
    saveAutoSnapshot.mockClear();
    keepSnapshot.mockClear();
    deleteSnapshot.mockClear();
    restoreSnapshot.mockClear();
    autoSnapshot.set(structuredClone(policy));
    databases.set([engine]);
  });

  it('states how many databases on this engine are scheduled', () => {
    autoSnapshot.update((p) => ({ ...p, enabled: true, sites: p.sites.map((s) => ({ ...s, covered: s.service === 'mysql' && s.database === 'shop_db' })) }));
    const { getByText, queryByText } = render(ServiceSnapshotsTab, { props: { svc } });
    expect(getByText(/1 of this engine/)).toBeTruthy();
    // The per-site opt-in list moved to each database's own dialog; only the
    // summary is left here.
    expect(queryByText(/Follows policy/)).toBeNull();
  });

  it('lists every snapshot on the engine, newest first, with what retention will do', () => {
    const { getByText, getAllByTitle } = render(ServiceSnapshotsTab, { props: { svc } });
    expect(getAllByTitle('auto-20260720-100000').length).toBe(1);
    expect(getByText(/expires around/)).toBeTruthy();
    expect(getByText('by-hand')).toBeTruthy();
  });

  it('filters by site', async () => {
    const { getByRole, getAllByTitle, queryByText } = render(ServiceSnapshotsTab, { props: { svc } });
    await fireEvent.click(getByRole('button', { name: /All sites/ }));
    const menu = await waitFor(() => {
      const el = document.querySelector('[role="listbox"]');
      if (!el) throw new Error('menu did not open');
      return el as HTMLElement;
    });
    const shop = [...menu.querySelectorAll('[role="option"]')].find((o) =>
      o.textContent?.includes('shop')
    );
    await fireEvent.click(shop as HTMLElement);
    await waitFor(() => expect(queryByText('by-hand')).toBeNull());
    // The cell shows the name without its stamp; the full name is its title.
    expect(getAllByTitle('auto-20260720-100000').length).toBe(1);
  });

  // A snapshot taken two months ago is out of every recent window.
  it('filters by when the snapshot was taken', async () => {
    const { getByRole, queryByText } = render(ServiceSnapshotsTab, { props: { svc } });
    await fireEvent.click(getByRole('button', { name: /Any time/ }));
    const menu = await waitFor(() => {
      const el = document.querySelector('[role="listbox"]');
      if (!el) throw new Error('menu did not open');
      return el as HTMLElement;
    });
    const recent = [...menu.querySelectorAll('[role="option"]')].find((o) =>
      o.textContent?.includes('Last 24 hours')
    );
    await fireEvent.click(recent as HTMLElement);
    await waitFor(() => expect(queryByText(/No snapshots match/)).toBeTruthy());
  });

  it('removes one snapshot behind a confirmation', async () => {
    const { getAllByLabelText, getByRole, queryByRole } = render(ServiceSnapshotsTab, {
      props: { svc }
    });
    await fireEvent.click(getAllByLabelText('Delete')[0]);
    expect(getByRole('button', { name: 'Confirm delete' })).toBeTruthy();
    // Nothing is deleted until the confirmation is taken.
    expect(deleteSnapshot).not.toHaveBeenCalled();

    await fireEvent.click(getByRole('button', { name: 'Confirm delete' }));
    await waitFor(() =>
      expect(deleteSnapshot).toHaveBeenCalledWith('mysql', 'shop_db', 'auto-20260720-100000')
    );
    await waitFor(() => expect(queryByRole('button', { name: 'Confirm delete' })).toBeNull());
  });

  it('selects every row from the header and removes them together', async () => {
    const { getByLabelText, getByRole } = render(ServiceSnapshotsTab, { props: { svc } });
    await fireEvent.click(getByLabelText('Select every snapshot shown'));

    await fireEvent.click(getByRole('button', { name: 'Remove 2' }));
    await fireEvent.click(getByRole('button', { name: 'Confirm delete' }));
    await waitFor(() => expect(deleteSnapshot).toHaveBeenCalledTimes(2));
    expect(deleteSnapshot).toHaveBeenCalledWith('mysql', 'shop_db', 'auto-20260720-100000');
    expect(deleteSnapshot).toHaveBeenCalledWith('mysql', 'blog_db', 'by-hand');
  });

  it('pages a long history and keeps selections across the pages', async () => {
    databases.set([pagedEngine(21)]);
    const { getByText, getByLabelText, getByRole } = render(ServiceSnapshotsTab, { props: { svc } });
    expect(getByText('1–20 of 21')).toBeTruthy();

    // Select-all covers the page on screen, not the whole history.
    await fireEvent.click(getByLabelText('Select every snapshot shown'));
    expect(getByRole('button', { name: 'Remove 20' })).toBeTruthy();

    await fireEvent.click(getByLabelText('Next page'));
    await waitFor(() => expect(getByText('21–21 of 21')).toBeTruthy());
    // The 20 picked on the first page are still picked.
    expect(getByRole('button', { name: 'Remove 20' })).toBeTruthy();
  });

  it('shows no pager while everything fits on one page', () => {
    const { queryByLabelText } = render(ServiceSnapshotsTab, { props: { svc } });
    expect(queryByLabelText('Next page')).toBeNull();
  });

  // Select-all covers the filtered view, so a hidden row can never be deleted.
  it('leaves a filtered-out row out of select all', async () => {
    const { getByRole, getByLabelText } = render(ServiceSnapshotsTab, { props: { svc } });
    await fireEvent.click(getByRole('button', { name: /All sites/ }));
    const menu = await waitFor(() => {
      const el = document.querySelector('[role="listbox"]');
      if (!el) throw new Error('menu did not open');
      return el as HTMLElement;
    });
    const shop = [...menu.querySelectorAll('[role="option"]')].find((o) =>
      o.textContent?.includes('shop')
    );
    await fireEvent.click(shop as HTMLElement);

    await fireEvent.click(getByLabelText('Select every snapshot shown'));
    await fireEvent.click(getByRole('button', { name: 'Remove 1' }));
    await fireEvent.click(getByRole('button', { name: 'Confirm delete' }));
    await waitFor(() => expect(deleteSnapshot).toHaveBeenCalledTimes(1));
    expect(deleteSnapshot).toHaveBeenCalledWith('mysql', 'shop_db', 'auto-20260720-100000');
  });

  // The tab states the schedule; changing it stays in the settings dialog.
  it('states the schedule and what the snapshots cost, without editing them', () => {
    const { getByText, queryByTitle } = render(ServiceSnapshotsTab, { props: { svc } });
    expect(getByText('1 day')).toBeTruthy();
    expect(getByText('Opt out')).toBeTruthy();
    expect(getByText(/2 snapshots ·/)).toBeTruthy();
    expect(queryByTitle('Automatic snapshots')).toBeNull();
  });

  // The soonest run among this engine's covered databases, so the card answers
  // "when next" without opening anything.
  it('dates the next run for this engine', async () => {
    autoSnapshot.update((p) => ({
      ...p,
      enabled: true,
      sites: p.sites.map((s) => ({
        ...s,
        covered: s.database === 'shop_db',
        next: s.database === 'shop_db' ? '2026-07-21T10:00:00Z' : undefined
      }))
    }));
    const { getByText } = render(ServiceSnapshotsTab, { props: { svc } });
    expect(getByText(/Next run/)).toBeTruthy();
    expect(getByText(/Jul 21, 2026/)).toBeTruthy();
  });

  it('says the first run lands on the next check when nothing has run yet', () => {
    autoSnapshot.update((p) => ({
      ...p,
      enabled: true,
      sites: p.sites.map((s) => ({ ...s, covered: s.database === 'shop_db', next: undefined }))
    }));
    const { getByText } = render(ServiceSnapshotsTab, { props: { svc } });
    expect(getByText(/at the next check/)).toBeTruthy();
  });

  it('opens the settings dialog to change the schedule', async () => {
    const { getByRole, getByTitle } = render(ServiceSnapshotsTab, { props: { svc } });
    await fireEvent.click(getByRole('button', { name: 'Snapshot settings' }));
    await waitFor(() => expect(getByTitle('Automatic snapshots')).toBeTruthy());
  });

  it('restores a snapshot behind its own confirmation', async () => {
    const { getAllByLabelText, getByRole } = render(ServiceSnapshotsTab, { props: { svc } });
    await fireEvent.click(getAllByLabelText('Restore')[0]);
    expect(restoreSnapshot).not.toHaveBeenCalled();

    await fireEvent.click(getByRole('button', { name: 'Confirm restore' }));
    await waitFor(() =>
      expect(restoreSnapshot).toHaveBeenCalledWith('mysql', 'shop_db', 'auto-20260720-100000')
    );
  });

  it('offers each snapshot as a download', () => {
    const { getAllByLabelText } = render(ServiceSnapshotsTab, { props: { svc } });
    const link = getAllByLabelText('Export')[0] as HTMLAnchorElement;
    expect(link.getAttribute('href')).toContain('database=shop_db');
    expect(link.getAttribute('href')).toContain('name=auto-20260720-100000');
  });

  it('keeps an automatic snapshot, and offers nothing on a manual one', async () => {
    const { getAllByRole, getByRole } = render(ServiceSnapshotsTab, { props: { svc } });
    expect(getAllByRole('button', { name: 'Keep for good' }).length).toBe(1);
    await fireEvent.click(getByRole('button', { name: 'Keep for good' }));
    await waitFor(() =>
      expect(keepSnapshot).toHaveBeenCalledWith('mysql', 'shop_db', 'auto-20260720-100000', true)
    );
  });
});
