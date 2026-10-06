import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import { get } from 'svelte/store';
import { modal } from '$stores/modals';
import BrowserEventsEditor from './BrowserEventsEditor.svelte';

const saved = [{ event: 'inertia:invalid', label: 'Inertia invalid response', message: 'detail.response.status' }];

describe('BrowserEventsEditor', () => {
  it('adds an event from the row at the bottom, and refuses one lerd would reject', async () => {
    const onsave = vi.fn();
    render(BrowserEventsEditor, { props: { saved, saving: false, onsave } });
    const name = screen.getByRole('textbox', { name: 'Event name' });
    const add = screen.getByRole('button', { name: 'Add' });
    await fireEvent.input(name, { target: { value: 'my app error()' } });
    expect(add).toBeDisabled();
    await fireEvent.input(name, { target: { value: 'my-app:error' } });
    await fireEvent.input(screen.getByRole('textbox', { name: 'Message path' }), { target: { value: 'detail.message' } });
    await fireEvent.click(add);
    expect(onsave).toHaveBeenCalledWith([...saved, { event: 'my-app:error', label: '', message: 'detail.message' }]);
  });

  it('edits a row in place', async () => {
    const onsave = vi.fn();
    render(BrowserEventsEditor, { props: { saved, saving: false, onsave } });
    await fireEvent.click(screen.getByRole('button', { name: 'Edit inertia:invalid' }));
    await fireEvent.input(screen.getByDisplayValue('Inertia invalid response'), { target: { value: 'Bad Inertia response' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(onsave).toHaveBeenCalledWith([{ ...saved[0], label: 'Bad Inertia response' }]);
  });

  it('removes a row straight away', async () => {
    const onsave = vi.fn();
    render(BrowserEventsEditor, { props: { saved, saving: false, onsave } });
    await fireEvent.click(screen.getByRole('button', { name: 'Remove inertia:invalid' }));
    expect(onsave).toHaveBeenCalledWith([]);
  });

  it('closes the settings modal when following the examples link to the docs', async () => {
    modal.set({ kind: 'browserLogs', browserLogsSite: 'shop' });
    render(BrowserEventsEditor, { props: { saved, saving: false, onsave: () => {} } });
    await fireEvent.click(screen.getByRole('link', { name: 'Examples' }));
    expect(get(modal).kind).toBeNull();
  });
});

