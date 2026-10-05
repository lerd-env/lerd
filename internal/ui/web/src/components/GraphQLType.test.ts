import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import GraphQLType from './GraphQLType.svelte';

const types = {
  DayAgenda: { kind: 'object' as const, fields: [{ name: 'date', type: 'String!' }, { name: 'items', type: '[AgendaItem!]!', args: [{ name: 'first', type: 'Int' }] }] },
  AgendaItem: { kind: 'object' as const, fields: [{ name: 'status', type: 'Status' }] },
  Status: { kind: 'enum' as const, values: ['OPEN', 'CLOSED'] }
};

describe('GraphQLType', () => {
  it('opens a type into its fields, and a field type in turn', async () => {
    render(GraphQLType, { props: { type: '[DayAgenda!]!', types } });
    expect(screen.queryByText('items')).not.toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button'));
    expect(screen.getByText('items')).toBeInTheDocument();
    expect(screen.getByText('(first: Int)')).toBeInTheDocument();
    await fireEvent.click(screen.getAllByRole('button', { expanded: false })[0]);
    expect(screen.getByText('status')).toBeInTheDocument();
  });

  it('marks a type the schema circles back to instead of opening it again', async () => {
    const cyclic = { Case: { kind: 'object' as const, fields: [{ name: 'parent', type: 'Case' }] } };
    render(GraphQLType, { props: { type: 'Case', types: cyclic } });
    await fireEvent.click(screen.getByRole('button'));
    expect(screen.getAllByRole('button')).toHaveLength(1);
    expect(screen.getByText('↻')).toBeInTheDocument();
  });

  it('shows a type it knows nothing more about as its name', () => {
    render(GraphQLType, { props: { type: 'String!', types } });
    expect(screen.getByText('String!')).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});
