import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

import ServiceDatabasesTab from './ServiceDatabasesTab.svelte';
import { databases, engineLoads, type DatabaseEngine } from '$stores/databases';
import type { Service } from '$stores/services';

function svc(status = 'active'): Service {
  return { name: 'mysql', status, site_count: 0, is_database: true } as Service;
}

function engine(over: Partial<DatabaseEngine> = {}): DatabaseEngine {
  return {
    service: 'mysql',
    family: 'mysql',
    status: 'active',
    supports_create: false,
    supports_drop: false,
    supports_export: false,
    supports_import: false,
    supports_snapshot: false,
    databases: [{ name: 'shop', size_bytes: 2048, snapshots: [] }],
    ...over
  };
}

function stubFetch(impl: () => Promise<Response>) {
  vi.stubGlobal('fetch', vi.fn(impl));
}

describe('ServiceDatabasesTab', () => {
  beforeEach(() => {
    databases.set([]);
    engineLoads.reset();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    databases.set([]);
    engineLoads.reset();
  });

  it('waits on the engine instead of reading a running one as stopped', () => {
    stubFetch(() => new Promise<Response>(() => {}));
    const { getByText, queryByText } = render(ServiceDatabasesTab, { props: { svc: svc() } });
    expect(getByText('Loading...')).toBeInTheDocument();
    expect(queryByText('Start the engine to view its databases.')).toBeNull();
  });

  it('hints at starting the engine when the service is stopped', () => {
    stubFetch(() => new Promise<Response>(() => {}));
    const { getByText, queryByText } = render(ServiceDatabasesTab, {
      props: { svc: svc('inactive') }
    });
    expect(getByText('Start the engine to view its databases.')).toBeInTheDocument();
    expect(queryByText('Loading...')).toBeNull();
  });

  it('lists the databases once the engine lands', async () => {
    stubFetch(async () => new Response(JSON.stringify(engine()), { status: 200 }));
    const { findByText } = render(ServiceDatabasesTab, { props: { svc: svc() } });
    expect(await findByText('shop')).toBeInTheDocument();
  });

  it('reports a failed load and reloads on retry', async () => {
    stubFetch(async () => new Response('boom', { status: 500 }));
    const { findByText, getByText } = render(ServiceDatabasesTab, { props: { svc: svc() } });
    expect(await findByText("Could not read the engine's databases.")).toBeInTheDocument();

    stubFetch(async () => new Response(JSON.stringify(engine()), { status: 200 }));
    await fireEvent.click(getByText('Retry'));
    expect(await findByText('shop')).toBeInTheDocument();
  });

  it('keeps showing the databases while a refresh is in flight', async () => {
    databases.set([engine()]);
    engineLoads.start('mysql');
    stubFetch(() => new Promise<Response>(() => {}));
    const { findByText, queryByText } = render(ServiceDatabasesTab, { props: { svc: svc() } });
    expect(await findByText('shop')).toBeInTheDocument();
    expect(queryByText('Loading...')).toBeNull();
  });

  it('stays on the error while a retry is in flight', async () => {
    engineLoads.settle('mysql', true);
    stubFetch(() => new Promise<Response>(() => {}));
    const { getByText, queryByText } = render(ServiceDatabasesTab, { props: { svc: svc() } });
    expect(getByText("Could not read the engine's databases.")).toBeInTheDocument();
    expect(queryByText('Loading...')).toBeNull();
  });
});
