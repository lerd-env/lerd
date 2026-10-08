import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import CardCloseButton from './CardCloseButton.svelte';

describe('CardCloseButton', () => {
  it('fires onclick under its accessible label', () => {
    const onclick = vi.fn();
    render(CardCloseButton, { props: { label: 'Remove Redis', onclick } });
    screen.getByRole('button', { name: 'Remove Redis' }).click();
    expect(onclick).toHaveBeenCalledOnce();
  });

  it('stays out of sight until its card is hovered or it is focused', () => {
    render(CardCloseButton, { props: { label: 'Remove Redis', onclick: () => {} } });
    const cls = screen.getByRole('button', { name: 'Remove Redis' }).className;
    expect(cls).toMatch(/(^| )opacity-0( |$)/);
    expect(cls).toContain('group-hover:opacity-100');
    expect(cls).toContain('focus-visible:opacity-100');
  });

  it('does not fire while disabled', () => {
    const onclick = vi.fn();
    render(CardCloseButton, { props: { label: 'Dismiss', disabled: true, onclick } });
    screen.getByRole('button', { name: 'Dismiss' }).click();
    expect(onclick).not.toHaveBeenCalled();
  });
});
