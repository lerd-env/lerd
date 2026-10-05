import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';

const calls: { path: string; init?: RequestInit }[] = [];
vi.mock('$lib/api', () => ({
  apiJson: vi.fn(() => Promise.resolve([])),
  apiFetch: vi.fn((path: string, init?: RequestInit) => {
    calls.push({ path, init });
    return Promise.resolve(new Response('{}', { status: 200 }));
  })
}));

import Annotator from './Annotator.svelte';

beforeEach(() => {
  calls.length = 0;
  document.body.innerHTML = '<main><button id="pay">Pay now</button></main>';
});

describe('Annotator', () => {
  it('picks the element under the pointer without the page seeing the click, and saves a note on it', async () => {
    const pay = document.getElementById('pay')!;
    const pageClick = vi.fn();
    pay.addEventListener('click', pageClick);
    document.elementFromPoint = () => pay;
    render(Annotator, { props: { rid: 'r1', picking: true } });

    window.dispatchEvent(new MouseEvent('pointermove', { clientX: 5, clientY: 5 }));
    pay.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }));
    expect(pageClick).not.toHaveBeenCalled();

    const box = await screen.findByRole('textbox');
    await fireEvent.input(box, { target: { value: 'Button should be red' } });
    await fireEvent.keyDown(box, { key: 'Enter' });

    await waitFor(() => expect(calls.some((c) => c.init?.method === 'POST')).toBe(true));
    const post = calls.find((c) => c.init?.method === 'POST')!;
    expect(post.path).toBe('/api/annotations');
    const body = JSON.parse(String(post.init!.body));
    expect(body).toMatchObject({ selector: '#pay', tag: 'button', text: 'Pay now', comment: 'Button should be red', rid: 'r1' });
  });
});
