import { describe, it, expect } from 'vitest';
import { ansiToHtml } from './ansi';

describe('ansiToHtml', () => {
  it('passes plain text through with HTML escaping', () => {
    expect(ansiToHtml('hello world')).toBe('hello world');
    expect(ansiToHtml('<script>')).toBe('&lt;script&gt;');
    expect(ansiToHtml('a & b > c')).toBe('a &amp; b &gt; c');
  });

  it('strips reset and renders nothing for default colors', () => {
    expect(ansiToHtml('\x1b[0mtext')).toBe('text');
  });

  it('wraps red foreground in a span', () => {
    const html = ansiToHtml('\x1b[31mboom\x1b[0m');
    expect(html).toContain('color:');
    expect(html).toContain('boom');
    expect(html).toContain('</span>');
  });

  it('closes the open span at end of input even without explicit reset', () => {
    const html = ansiToHtml('\x1b[32mgreen-no-reset');
    expect(html).toContain('green-no-reset');
    expect(html.match(/<\/span>/g)).toHaveLength(1);
  });

  it('handles bold + color together', () => {
    const html = ansiToHtml('\x1b[1;31mERROR\x1b[0m: details');
    expect(html).toContain('font-weight:600');
    expect(html).toContain('color:');
    expect(html).toContain('ERROR');
    expect(html).toContain('details');
  });

  it('strips cursor-positioning escapes without rendering them', () => {
    // \x1b[2K clears the line, \x1b[1A moves cursor up — both should vanish.
    const html = ansiToHtml('\x1b[2K\x1b[1Akept');
    expect(html).toBe('kept');
  });

  it('handles 256-color foreground by mapping to nearest 8-color', () => {
    const html = ansiToHtml('\x1b[38;5;9mbright-red\x1b[0m');
    expect(html).toContain('bright-red');
    expect(html).toContain('color:');
  });

  it('preserves newlines', () => {
    expect(ansiToHtml('one\ntwo')).toBe('one\ntwo');
  });

  it('renders a real `artisan about` line', () => {
    const html = ansiToHtml('  \x1b[32;1mEnvironment\x1b[39;22m \x1b[90m......\x1b[39m');
    expect(html).toContain('font-weight:600');
    expect(html).toContain('Environment</span>');
    expect(html).toContain('......');
  });

  it('renders background colors', () => {
    const html = ansiToHtml('\x1b[41mred-bg\x1b[49m plain');
    expect(html).toContain('background-color:');
    expect(html).toContain('red-bg');
    expect(html).toContain('plain');
  });

  it('renders italic, underline and dim', () => {
    expect(ansiToHtml('\x1b[3mi\x1b[23m')).toContain('font-style:italic');
    expect(ansiToHtml('\x1b[4mu\x1b[24m')).toContain('text-decoration:underline');
    expect(ansiToHtml('\x1b[2md\x1b[22m')).toContain('opacity:');
  });

  it('clears an attribute when its reset code arrives', () => {
    const html = ansiToHtml('\x1b[4munder\x1b[24mplain');
    expect(html).toContain('>under');
    expect(html.endsWith('plain')).toBe(true);
  });

  it('renders the 256-color cube and grayscale ramp as rgb', () => {
    expect(ansiToHtml('\x1b[38;5;196mred\x1b[0m')).toContain('color:rgb(255,0,0)');
    expect(ansiToHtml('\x1b[38;5;244mgrey\x1b[0m')).toContain('color:rgb(128,128,128)');
  });

  it('renders 24-bit true color for foreground and background', () => {
    expect(ansiToHtml('\x1b[38;2;10;20;30mfg\x1b[0m')).toContain('color:rgb(10,20,30)');
    expect(ansiToHtml('\x1b[48;2;10;20;30mbg\x1b[0m')).toContain('background-color:rgb(10,20,30)');
  });

  it('does not let true-color params leak in as separate codes', () => {
    // 38;2;1;31;7 must consume all five params — a naive parser would read the
    // trailing 31 as "red" and 7 as inverse.
    const html = ansiToHtml('\x1b[38;2;1;31;7mtext');
    expect(html).toContain('color:rgb(1,31,7)');
    expect(html.match(/<span/g)).toHaveLength(1);
  });

  it('collapses carriage-return progress frames to the final one', () => {
    expect(ansiToHtml('10%\r50%\r100%')).toBe('100%');
    expect(ansiToHtml('done\r')).toBe('done');
    expect(ansiToHtml('a\nb\rc')).toBe('a\nc');
  });

  it('carries color across collapsed progress frames', () => {
    // The green was set on a frame that got painted over, but a terminal had
    // already processed it, so the surviving frame stays green.
    const html = ansiToHtml('\x1b[32m10%\r50%');
    expect(html).toContain('color:');
    expect(html).toContain('50%');
    expect(html).not.toContain('10%');
  });

  it('handles back-to-back color changes without nesting spans', () => {
    const html = ansiToHtml('\x1b[31mred\x1b[32mgreen\x1b[0m');
    // We should never have a <span> inside a <span> — closing the previous
    // span happens before opening the next.
    const opens = html.match(/<span/g)?.length ?? 0;
    const closes = html.match(/<\/span>/g)?.length ?? 0;
    expect(opens).toBe(closes);
  });
});
