import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import LerdMark from './LerdMark.svelte';

describe('LerdMark', () => {
  it('paints the mark in the theme accent with the letter in its on-accent tone', () => {
    render(LerdMark, { class: 'w-7 h-7 rounded-lg' });
    const mark = screen.getByRole('img', { name: 'Lerd' });
    expect(mark.className).toContain('bg-lerd-red');
    expect(mark.className).toContain('w-7');
    const letter = mark.firstElementChild as HTMLElement;
    expect(letter.className).toContain('bg-lerd-onred');
    expect(letter.getAttribute('style')).toContain('/icons/mark.svg');
  });
});
