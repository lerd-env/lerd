import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import RequestDetail from './RequestDetail.svelte';

vi.mock('$stores/requests', async (orig) => ({
  ...(await orig<object>()),
  loadRequest: () =>
    Promise.resolve({
      rid: 'g1', type: 'fetch', method: 'POST', uri: '/graphql', status: 200, started: '', counts: {}, problems: [], operation: 'query Feed',
      events: {
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
});
