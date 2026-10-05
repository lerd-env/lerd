import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import HttpCall from './HttpCall.svelte';

const call = {
  method: 'GET', url: 'https://api.test/stock', status: 200, time_ms: 42,
  timing: { dns: 2, connect: 5, first_byte: 40 },
  request_headers: { Accept: 'application/json' },
  response_headers: { 'Content-Type': 'application/json' }
};

describe('HttpCall', () => {
  it('opens into the phases and both ends of the request', async () => {
    render(HttpCall, { props: { call, tone: '' } });
    expect(screen.queryByText('Request headers')).not.toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button'));
    expect(screen.getByText('Request headers')).toBeInTheDocument();
    expect(screen.getByText('Response headers')).toBeInTheDocument();
    // Each phase is the gap since the one before it: connect 5 - 2, waiting 40 - 5, download 42 - 40.
    for (const [label, took] of [['DNS', '2.0 ms'], ['Connect', '3.0 ms'], ['Waiting', '35 ms'], ['Download', '2.0 ms']]) {
      expect(screen.getByText(label).parentElement).toHaveTextContent(took);
    }
  });

  it('cannot open a request with nothing more to show', () => {
    render(HttpCall, { props: { call: { method: 'GET', url: 'https://api.test/', status: 200 }, tone: '' } });
    expect(screen.getByRole('button')).toBeDisabled();
  });
});
