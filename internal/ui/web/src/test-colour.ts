// Browsers and jsdom serialise a colour read back from a style as rgb(), so the
// theme tests turn each one, alone or inside a shadow, into the #rrggbb their
// palettes are written in.
export function hex(value: string): string {
  return value.replace(/rgb\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)/g, (_, r, g, b) =>
    '#' + [r, g, b].map((n) => Number(n).toString(16).padStart(2, '0')).join(''),
  );
}
