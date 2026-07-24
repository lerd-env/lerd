import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import ImportIssuesModal from './ImportIssuesModal.svelte';

function issues(n: number) {
  return Array.from({ length: n }, (_, i) => ({ message: `ERROR:  relation "table_${i}" already exists`, count: 1 }));
}

describe('ImportIssuesModal', () => {
  it('lists every issue the report carried, however many', () => {
    const { container } = render(ImportIssuesModal, {
      props: { title: 'Imported, but the engine reported 320 errors', issues: issues(243), onclose: () => {} }
    });
    expect(container.querySelectorAll('li')).toHaveLength(243);
  });

  it('keeps the long list inside its own scroll area', () => {
    const { container } = render(ImportIssuesModal, {
      props: { title: 'Imported', issues: issues(120), onclose: () => {} }
    });
    expect(container.querySelector('ul')?.className).toContain('overflow-y-auto');
  });

  it('lists what lerd held back separately from what the engine rejected', () => {
    const { queryByText, getByText } = render(ImportIssuesModal, {
      props: {
        title: 'Imported',
        issues: issues(2),
        skipped: [{ message: 'DEFINER clauses naming users the local engine does not have', count: 4 }],
        onclose: () => {}
      }
    });
    expect(getByText(/lerd left these out on the way in/)).toBeInTheDocument();
    expect(getByText('4×')).toBeInTheDocument();
    expect(queryByText(/more distinct errors not shown/)).toBeNull();
  });

  it('notes what was dropped only when something was', () => {
    const { queryByText, rerender } = render(ImportIssuesModal, {
      props: { title: 'Imported', issues: issues(3), onclose: () => {} }
    });
    expect(queryByText(/more distinct errors not shown/)).toBeNull();
    rerender({ title: 'Imported', issues: issues(3), omitted: 7, onclose: () => {} });
    expect(queryByText(/7 more distinct errors not shown/)).not.toBeNull();
  });
});
