import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import Harness from './WorkerControl.test.svelte';

const queueOptions = [
  { name: 'queue', value: 'high,default,low', default: 'default' },
  { name: 'tries', value: '', default: '3' }
];

describe('WorkerControl', () => {
  it('renders a plain toggle for a worker with nothing to tune', () => {
    const { container } = render(Harness, { props: {} });
    expect(screen.getByRole('button', { name: 'Queue' })).toBeInTheDocument();
    expect(container.querySelectorAll('button')).toHaveLength(1);
  });

  it('adds the gear when the definition declares options', () => {
    const { container } = render(Harness, { props: { options: queueOptions } });
    expect(container.querySelectorAll('button')).toHaveLength(2);
  });

  it('adds a logs shortcut for a worker whose journal is readable', async () => {
    const onLogs = vi.fn();
    render(Harness, { props: { onLogs } });
    await fireEvent.click(screen.getByRole('button', { name: 'Queue logs' }));
    expect(onLogs).toHaveBeenCalled();
  });

  it('keeps the gear and the logs shortcut side by side', () => {
    const { container } = render(Harness, { props: { options: queueOptions, onLogs: () => {} } });
    expect(container.querySelectorAll('button')).toHaveLength(3);
    expect(screen.getByRole('button', { name: 'Worker options' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Queue logs' })).toBeInTheDocument();
  });

  it('prefills the committed value and offers the definition default', async () => {
    render(Harness, { props: { options: queueOptions } });
    await fireEvent.click(screen.getByRole('button', { name: 'Worker options' }));
    const queue = screen.getByLabelText('queue') as HTMLInputElement;
    const tries = screen.getByLabelText('tries') as HTMLInputElement;
    expect(queue.value).toBe('high,default,low');
    expect(tries.value).toBe('');
    expect(tries.placeholder).toBe('3');
  });

  it('forwards every field on save, so a cleared one goes back to the default', async () => {
    const onSaveOptions = vi.fn();
    render(Harness, { props: { options: queueOptions, onSaveOptions } });
    await fireEvent.click(screen.getByRole('button', { name: 'Worker options' }));
    await fireEvent.input(screen.getByLabelText('queue'), { target: { value: ' emails ' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(onSaveOptions).toHaveBeenCalledWith({ queue: 'emails', tries: '' });
  });
});
