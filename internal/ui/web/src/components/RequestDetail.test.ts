import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import RequestDetail from './RequestDetail.svelte';

vi.mock('$stores/requests', async (orig) => ({
  ...(await orig<object>()),
  loadRequest: () =>
    Promise.resolve({
      rid: 'g1', type: 'fetch', method: 'POST', uri: '/graphql', status: 200, started: '', counts: {}, problems: [], operation: 'query Feed',
      queries: { query_count: 4, total_time_ms: 4, n_plus_one: [{ fingerprint: 'select * from users where id = ?', count: 3, sample_sql: 'select * from users where id = ?', ids: ['q0', 'q1', 'q2'], caller: { file: '/srv/app/vendor/acme/Store.php', line: 9 } }] },
      events: {
        query: ['select * from users where id = ?', 'select * from users where id = ?', 'select * from users where id = ?', 'select * from sessions'].map((sql, i) => ({ id: `q${i}`, ts: '', kind: 'query', ctx: {}, src: { file: '/srv/app/vendor/acme/Store.php', line: 9 }, data: { sql, time_ms: 1, trace: [{ file: '/srv/app/vendor/acme/Store.php', line: 9, func: 'Acme\\Db->select' }, { file: '/srv/app/vendor/acme/Store.php', line: 4, func: 'Acme\\Cache\\Store->get' }] } })),
        view: [0, 1].map((i) => ({ id: `v${i}`, ts: '', kind: 'view', ctx: {}, src: {}, data: { name: 'item' } })),
        span: [5, 70].map((ms, i) => ({ id: `s${i}`, ts: '', kind: 'span', ctx: {}, src: {}, data: { label: 'View', name: 'item', time_ms: ms, status: 'ok' } })),
        cache: Array.from({ length: 450 }, (_, i) => ({ id: `c${i}`, ts: '', kind: 'cache', ctx: {}, src: {}, data: { op: 'hit', key: `key-${i}` } })),
        request: [{ id: 'e1', ts: '', kind: 'request', ctx: {}, src: {}, data: { graphql: [{ type: 'query', name: 'Feed', query: '{ latest: notifications(first: 5) { id } }', fields: [{ alias: 'latest', name: 'notifications', type: '[Notification!]!', file: '/srv/app/app/GraphQL/Queries/NotificationsQuery.php', line: 9, type_file: '/srv/app/app/GraphQL/Types/NotificationType.php', type_line: 7, args: { first: 5 }, data: [{ id: 1 }] }], errors: [{ message: 'Unauthenticated', path: ['me'] }] }], graphql_types: { Notification: { kind: 'object', fields: [{ name: 'id', type: 'ID!' }], file: '/srv/app/app/GraphQL/Types/NotificationType.php', line: 7 } } } }]
      }
    })
}));

describe('RequestDetail', () => {
  it('shows each GraphQL field the request called with its type, input and response', async () => {
    render(RequestDetail, { props: { rid: 'g1', onopen: () => {} } });
    await fireEvent.click(await screen.findByRole('tab', { name: /GraphQL/ }));
    expect(screen.getByText('[Notification!]!').closest('button')).toBeInTheDocument();
    // The field links to its resolver; the query, folded below, names it too.
    expect(screen.getAllByText('notifications').some((el) => el.closest('button'))).toBe(true);
    expect(screen.getByText('Input')).toBeInTheDocument();
    expect(screen.getByText('Response')).toBeInTheDocument();
    expect(screen.getByText('Unauthenticated')).toBeInTheDocument();
  });

  it('pages a long list instead of dropping the rest', async () => {
    render(RequestDetail, { props: { rid: 'g1', onopen: () => {} } });
    await fireEvent.click(await screen.findByRole('tab', { name: /Cache/ }));
    expect(screen.getByText('key-199')).toBeInTheDocument();
    expect(screen.queryByText('key-200')).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Show 200 more of 250 remaining' }));
    expect(screen.getByText('key-399')).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: 'Show 50 more of 50 remaining' }));
    expect(screen.getByText('key-449')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /more of/ })).toBeNull();
  });

  it('narrows the queries to the ones an N+1 repeats', async () => {
    const { container } = render(RequestDetail, { props: { rid: 'g1', onopen: () => {} } });
    await fireEvent.click(await screen.findByRole('tab', { name: /Database/ }));
    const sql = () => [...container.querySelectorAll('code')].map((c) => c.textContent).filter((t) => t?.startsWith('select * from'));
    expect(sql().some((t) => t?.includes('sessions'))).toBe(true);
    await fireEvent.click(screen.getByRole('button', { name: 'Show queries' }));
    expect(sql().some((t) => t?.includes('sessions'))).toBe(false);
    expect(sql().filter((t) => t?.includes('users'))).toHaveLength(3);
  });

  it('gives each rendered view the time of its own render', async () => {
    render(RequestDetail, { props: { rid: 'g1', onopen: () => {} } });
    await fireEvent.click(await screen.findByRole('tab', { name: /Views/ }));
    expect(screen.getByText('5.0 ms')).toBeInTheDocument();
    expect(screen.getByText('70 ms')).toBeInTheDocument();
  });

  it('names an N+1 finding by the class that ran its queries', async () => {
    render(RequestDetail, { props: { rid: 'g1', onopen: () => {} } });
    await fireEvent.click(await screen.findByRole('tab', { name: /Database/ }));
    expect(screen.getAllByText(/^Store(:9)?$/).length).toBeGreaterThan(1);
    expect(screen.queryByText(/vendor\/acme\/Store\.php/)).toBeNull();
  });
});
