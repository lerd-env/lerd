import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import BrowserEventsEditor from './BrowserEventsEditor.svelte';

const saved = [{ event: 'inertia:invalid', label: 'Inertia invalid response', message: 'detail.response.status' }];
const presets = [{ name: 'inertia', label: 'Inertia', events: saved, detected: true, applied: true }];

describe('BrowserEventsEditor', () => {
  it('marks a row that came from an added preset', () => {
    render(BrowserEventsEditor, { props: { saved, presets, saving: false, onsave: () => {} } });
    expect(screen.getByText('Inertia')).toBeInTheDocument();
  });

  it('saves a new event, and refuses one lerd would reject', async () => {
    const onsave = vi.fn();
    render(BrowserEventsEditor, { props: { saved, presets, saving: false, onsave } });
    await fireEvent.click(screen.getByRole('button', { name: /Add event/ }));
    const name = screen.getByRole('textbox', { name: 'Event name' });
    await fireEvent.input(name, { target: { value: 'my app error()' } });
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
    await fireEvent.input(name, { target: { value: 'my-app:error' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(onsave).toHaveBeenCalledWith([...saved, { event: 'my-app:error', label: '', message: '' }]);
  });

  it('removes a row straight away', async () => {
    const onsave = vi.fn();
    render(BrowserEventsEditor, { props: { saved, presets, saving: false, onsave } });
    await fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
    expect(onsave).toHaveBeenCalledWith([]);
  });
});
