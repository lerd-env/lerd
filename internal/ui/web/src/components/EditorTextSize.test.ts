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

  it('stops at both ends of the range', async () => {
    editorFontSize.set(FONT_MAX);
    await open();
    expect(screen.getByRole<HTMLButtonElement>('button', { name: m.tinker_textLarger() }).disabled).toBe(true);
    editorFontSize.set(FONT_MIN);
    await screen.findByText(String(FONT_MIN));
    expect(screen.getByRole<HTMLButtonElement>('button', { name: m.tinker_textSmaller() }).disabled).toBe(true);
  });
});
