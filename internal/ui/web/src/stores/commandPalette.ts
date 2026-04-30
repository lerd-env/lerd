import { writable } from 'svelte/store';

export const paletteOpen = writable<boolean>(false);

export function openCommandPalette() {
  paletteOpen.set(true);
}

export function closeCommandPalette() {
  paletteOpen.set(false);
}

export function toggleCommandPalette() {
  paletteOpen.update((v) => !v);
}
