// Minimal SGR (Select Graphic Rendition) parser. Converts ANSI escape
// sequences from CLI tools into safe HTML span tags. Handles:
//   - foreground 30-37 / bright 90-97 / default 39
//   - background 40-47 / bright 100-107 / default 49
//   - bold (1), dim (2), italic (3), underline (4) and their resets
//   - 256-color and 24-bit true color, foreground and background
//
// Skips cursor positioning, line clear, and other non-SGR sequences (they're
// noise in a non-interactive replay). Strips them rather than letting them
// reach the DOM.

const FG: Record<number, string> = {
  30: 'var(--ansi-black, #555)',
  31: 'var(--ansi-red, #ff5555)',
  32: 'var(--ansi-green, #50fa7b)',
  33: 'var(--ansi-yellow, #f1fa8c)',
  34: 'var(--ansi-blue, #6272ff)',
  35: 'var(--ansi-magenta, #ff79c6)',
  36: 'var(--ansi-cyan, #8be9fd)',
  37: 'var(--ansi-white, #f8f8f2)',
  90: 'var(--ansi-bright-black, #6272a4)',
  91: 'var(--ansi-bright-red, #ff6e6e)',
  92: 'var(--ansi-bright-green, #69ff94)',
  93: 'var(--ansi-bright-yellow, #ffffa5)',
  94: 'var(--ansi-bright-blue, #d6acff)',
  95: 'var(--ansi-bright-magenta, #ff92df)',
  96: 'var(--ansi-bright-cyan, #a4ffff)',
  97: 'var(--ansi-bright-white, #ffffff)'
};

// The xterm 256-color cube and grayscale ramp. Indexes 0-15 reuse the named
// palette above so a theme override still applies to them.
const CUBE = [0, 95, 135, 175, 215, 255];

function color256(idx: number): string | undefined {
  if (idx < 8) return FG[30 + idx];
  if (idx < 16) return FG[90 + (idx - 8)];
  if (idx < 232) {
    const n = idx - 16;
    return `rgb(${CUBE[Math.floor(n / 36) % 6]},${CUBE[Math.floor(n / 6) % 6]},${CUBE[n % 6]})`;
  }
  if (idx < 256) {
    const v = 8 + (idx - 232) * 10;
    return `rgb(${v},${v},${v})`;
  }
  return undefined;
}

function htmlEscape(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

interface SpanState {
  fg?: string;
  bg?: string;
  bold?: boolean;
  dim?: boolean;
  italic?: boolean;
  underline?: boolean;
}

function spanStyle(s: SpanState): string {
  const parts: string[] = [];
  if (s.fg) parts.push('color:' + s.fg);
  if (s.bg) parts.push('background-color:' + s.bg);
  if (s.bold) parts.push('font-weight:600');
  if (s.dim) parts.push('opacity:0.65');
  if (s.italic) parts.push('font-style:italic');
  if (s.underline) parts.push('text-decoration:underline');
  return parts.join(';');
}

export function ansiToHtml(text: string): string {
  // ESC[...m for SGR. We ignore non-m escapes (cursor moves, etc.) by
  // matching ESC[ followed by any param chars then a terminator letter.
  const sgrRe = /\x1b\[([\d;]*)m/g;
  const otherEscRe = /\x1b\[[\d;]*[A-Za-z]/g;
  // First scrub non-SGR escapes by replacing them with empty string.
  // Then walk SGR matches.
  let cleaned = text.replace(otherEscRe, (m) => (m.endsWith('m') ? m : ''));
  // Also drop other common ESC sequences we don't model: ESC] OSC, ESC( charset.
  cleaned = cleaned.replace(/\x1b\][^\x07\x1b]*(\x07|\x1b\\)/g, '').replace(/\x1b[()][\dA-Za-z]/g, '');
  // Carriage returns rewrite the line in a terminal, which is how progress
  // bars (composer, npm) animate. Keep only what a terminal would be left
  // showing instead of concatenating every frame.
  cleaned = cleaned
    .split('\n')
    .map((l) => {
      const trimmed = l.replace(/\r+$/, '');
      const cr = trimmed.lastIndexOf('\r');
      return cr === -1 ? trimmed : trimmed.slice(cr + 1);
    })
    .join('\n');

  let out = '';
  let last = 0;
  const state: SpanState = {};
  let openSpan = false;

  const writeChunk = (chunk: string) => {
    if (!chunk) return;
    const escaped = htmlEscape(chunk);
    if (openSpan) {
      out += escaped;
    } else {
      const style = spanStyle(state);
      if (style) {
        out += '<span style="' + style + '">' + escaped;
        openSpan = true;
      } else {
        out += escaped;
      }
    }
  };

  const closeSpan = () => {
    if (openSpan) {
      out += '</span>';
      openSpan = false;
    }
  };

  // Reads an extended color at params[i] (38 or 48) and returns the CSS color
  // plus how many extra params it consumed.
  const extended = (params: number[], i: number): [string | undefined, number] => {
    if (params[i + 1] === 5) return [color256(params[i + 2] ?? 0), 2];
    if (params[i + 1] === 2) {
      const [r, g, b] = [params[i + 2] ?? 0, params[i + 3] ?? 0, params[i + 4] ?? 0];
      return [`rgb(${r},${g},${b})`, 4];
    }
    return [undefined, 1];
  };

  let m: RegExpExecArray | null;
  while ((m = sgrRe.exec(cleaned)) !== null) {
    writeChunk(cleaned.slice(last, m.index));
    last = m.index + m[0].length;

    const params = m[1].split(';').filter(Boolean).map(Number);
    if (params.length === 0) params.push(0);

    for (let i = 0; i < params.length; i++) {
      const p = params[i];
      closeSpan();
      if (p === 0) {
        state.fg = undefined;
        state.bg = undefined;
        state.bold = false;
        state.dim = false;
        state.italic = false;
        state.underline = false;
      } else if (p === 1) state.bold = true;
      else if (p === 2) state.dim = true;
      else if (p === 3) state.italic = true;
      else if (p === 4) state.underline = true;
      else if (p === 22) {
        state.bold = false;
        state.dim = false;
      } else if (p === 23) state.italic = false;
      else if (p === 24) state.underline = false;
      else if (p === 39) state.fg = undefined;
      else if (p === 49) state.bg = undefined;
      else if (FG[p]) state.fg = FG[p];
      else if (p >= 40 && p <= 47) state.bg = FG[p - 10];
      else if (p >= 100 && p <= 107) state.bg = FG[p - 10];
      else if (p === 38 || p === 48) {
        const [color, used] = extended(params, i);
        if (color) {
          if (p === 38) state.fg = color;
          else state.bg = color;
        }
        i += used;
      }
      // Unrecognized codes (inverse, blink, …) are dropped silently.
    }
  }
  writeChunk(cleaned.slice(last));
  closeSpan();
  return out;
}
