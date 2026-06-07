import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import Harness from './StripeControl.test.svelte';

describe('StripeControl', () => {
  it('renders the Stripe toggle and the config gear', () => {
    const { container } = render(Harness, { props: {} });
    expect(screen.getByRole('button', { name: 'Stripe' })).toBeInTheDocument();
    // Toggle + gear, even while stopped, so the path can be set before enabling.
    expect(container.querySelectorAll('button')).toHaveLength(2);
  });

  it('forwards the toggle click', () => {
    const onToggle = vi.fn();
    render(Harness, { props: { onToggle } });
    screen.getByRole('button', { name: 'Stripe' }).click();
    expect(onToggle).toHaveBeenCalledOnce();
  });

  it('opens the modal prefilled with the current webhook path', async () => {
    render(Harness, { props: { webhookPath: '/webhooks/stripe' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Configure webhook path' }));
    const input = screen.getByLabelText('Webhook path') as HTMLInputElement;
    expect(input.value).toBe('/webhooks/stripe');
  });

  it('normalises a leading slash and forwards the saved path', async () => {
    const onSaveConfig = vi.fn();
    render(Harness, { props: { onSaveConfig } });
    await fireEvent.click(screen.getByRole('button', { name: 'Configure webhook path' }));
    const input = screen.getByLabelText('Webhook path') as HTMLInputElement;
    await fireEvent.input(input, { target: { value: 'nest/stripe' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(onSaveConfig).toHaveBeenCalledWith('/nest/stripe');
  });
});
