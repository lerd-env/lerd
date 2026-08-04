import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import TinkerSnippetSaveModal from './TinkerSnippetSaveModal.svelte';
import type { TinkerSnippet } from '$stores/sites';

const existing: TinkerSnippet = {
  name: 'count-users.php',
  label: 'count-users',
  source: 'project',
  content: '1;'
};

function renderModal(onsave = vi.fn(), onclose = vi.fn()) {
  const utils = render(TinkerSnippetSaveModal, {
    props: {
      open: true,
      snippets: [existing],
      initialName: '',
      initialSource: 'project' as const,
      onsave,
      onclose
    }
  });
  return { ...utils, onsave, onclose };
}

describe('TinkerSnippetSaveModal', () => {
  it('saves a fresh name straight away, appending nothing itself', async () => {
    const { getByText, onsave } = renderModal();
    const input = document.getElementById('snippet-name') as HTMLInputElement;
    await fireEvent.input(input, { target: { value: 'fresh' } });
    await fireEvent.click(getByText('Save'));
    expect(onsave).toHaveBeenCalledWith('fresh', 'project');
  });

  it('turns into an overwrite confirmation when the name already exists', async () => {
    const { getByText, queryByText, onsave } = renderModal();
    const input = document.getElementById('snippet-name') as HTMLInputElement;
    await fireEvent.input(input, { target: { value: 'count-users' } });
    await fireEvent.click(getByText('Save'));
    // First click only arms the confirmation; nothing is saved yet.
    expect(onsave).not.toHaveBeenCalled();
    expect(queryByText(/already exists there/)).toBeInTheDocument();
    await fireEvent.click(getByText('Overwrite'));
    expect(onsave).toHaveBeenCalledWith('count-users', 'project');
  });

  it('detects the collision case-insensitively for case-insensitive filesystems', async () => {
    const { getByText, onsave } = renderModal();
    const input = document.getElementById('snippet-name') as HTMLInputElement;
    await fireEvent.input(input, { target: { value: 'Count-Users' } });
    await fireEvent.click(getByText('Save'));
    expect(onsave).not.toHaveBeenCalled();
    expect(getByText(/already exists there/)).toBeInTheDocument();
  });

  it('disarms the overwrite confirmation when the name changes', async () => {
    const { getByText, queryByText } = renderModal();
    const input = document.getElementById('snippet-name') as HTMLInputElement;
    await fireEvent.input(input, { target: { value: 'count-users' } });
    await fireEvent.click(getByText('Save'));
    expect(queryByText(/already exists there/)).toBeInTheDocument();
    await fireEvent.input(input, { target: { value: 'count-users-2' } });
    expect(queryByText(/already exists there/)).not.toBeInTheDocument();
    expect(getByText('Save')).toBeInTheDocument();
  });
});
