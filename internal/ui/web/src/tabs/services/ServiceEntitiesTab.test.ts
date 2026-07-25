import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';

import ServiceEntitiesTab from './ServiceEntitiesTab.svelte';
import { entities, type EntityKind } from '$stores/entities';
import type { Service } from '$stores/services';

function svc(status = 'active'): Service {
  return { name: 'rustfs', status, site_count: 0, entity_kinds: ['buckets'] } as Service;
}

function buckets(): EntityKind {
  return {
    kind: 'buckets',
    columns: [
      { key: 'objects', label: 'Objects', format: 'number' },
      { key: 'size', label: 'Size', format: 'bytes' }
    ],
    actions: [
      { name: 'create' },
      { name: 'export' },
      { name: 'import' },
      { name: 'delete', destructive: true }
    ],
    rows: [{ name: 'lerd', values: ['3', '2048'], site: 'shop.test' }]
  };
}

describe('ServiceEntitiesTab', () => {
  beforeEach(() => {
    entities.set({ rustfs: [buckets()] });
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('[]', { status: 200 }))
    );
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    entities.set({});
  });

  it('renders rows with their declared columns formatted', () => {
    const { getByText } = render(ServiceEntitiesTab, { props: { svc: svc() } });
    expect(getByText('lerd')).toBeInTheDocument();
    expect(getByText('3')).toBeInTheDocument();
    expect(getByText('2.0 KB')).toBeInTheDocument();
  });

  it('links the owning site on the card', () => {
    const { getByText } = render(ServiceEntitiesTab, { props: { svc: svc() } });
    expect(getByText('shop.test')).toBeInTheDocument();
  });

  it('renders export as a download link and import as an upload button', () => {
    const { getByLabelText } = render(ServiceEntitiesTab, { props: { svc: svc() } });
    expect(getByLabelText('Export')).toHaveAttribute(
      'href',
      expect.stringContaining('/api/entities/rustfs/buckets/export?name=lerd')
    );
    expect(getByLabelText('Import')).toBeInTheDocument();
    // Streaming actions never double as generic row buttons.
    expect(() => getByLabelText('export')).toThrow();
  });

  it('asks before running a destructive action', async () => {
    const { getByLabelText, getByText } = render(ServiceEntitiesTab, { props: { svc: svc() } });
    await fireEvent.click(getByLabelText('delete'));
    // Nothing ran yet: the confirmation stands between the click and the data.
    expect(vi.mocked(fetch).mock.calls.some(([u]) => String(u).includes('/delete'))).toBe(false);
    expect(getByText('This cannot be undone.')).toBeInTheDocument();
  });

  it('hints at starting the service when it is stopped', () => {
    const { getByText } = render(ServiceEntitiesTab, { props: { svc: svc('inactive') } });
    expect(getByText('Start the service to browse its contents.')).toBeInTheDocument();
  });
});
