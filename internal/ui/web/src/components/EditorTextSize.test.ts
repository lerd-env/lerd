import { fireEvent, render, screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { beforeEach, describe, expect, it } from 'vitest';
import EditorTextSize from './EditorTextSize.svelte';
import { editorFontSize, FONT_MAX, FONT_MIN } from '$stores/editorFont';
import { m } from '../paraglide/messages.js';

const open = async () => {
  render(EditorTextSize);
  await fireEvent.click(screen.getByRole('button', { name: m.tinker_textSize() }));
};

describe('EditorTextSize', () => {
  beforeEach(() => editorFontSize.set(12));

  it('moves the shared size', async () => {
    await open();
    await fireEvent.click(screen.getByRole('button', { name: m.tinker_textLarger() }));
    expect(get(editorFontSize)).toBe(13);
    await fireEvent.click(screen.getByRole('button', { name: m.tinker_textSmaller() }));
    await fireEvent.click(screen.getByRole('button', { name: m.tinker_textSmaller() }));
    expect(get(editorFontSize)).toBe(11);
  });

  it('stops at both ends of the range without dropping keyboard focus', async () => {
    editorFontSize.set(FONT_MAX - 1);
    await open();
    const larger = screen.getByRole('button', { name: m.tinker_textLarger() });
    larger.focus();
    await fireEvent.click(larger);
    await fireEvent.click(larger);
    expect(get(editorFontSize)).toBe(FONT_MAX);
    expect(larger.getAttribute('aria-disabled')).toBe('true');
    expect(document.activeElement).toBe(larger);

    editorFontSize.set(FONT_MIN);
    const smaller = await screen.findByRole('button', { name: m.tinker_textSmaller() });
    await fireEvent.click(smaller);
    expect(get(editorFontSize)).toBe(FONT_MIN);
    expect(smaller.getAttribute('aria-disabled')).toBe('true');
  });
});
