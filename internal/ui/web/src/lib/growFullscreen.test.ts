import { describe, it, expect, vi, beforeEach } from 'vitest';
import { growFullscreen } from './growFullscreen';

const rect = (top: number, left: number, width: number, height: number) =>
  ({ top, left, width, height, x: left, y: top, right: left + width, bottom: top + height, toJSON() {} }) as DOMRect;

describe('growFullscreen', () => {
  beforeEach(() => {
    vi.stubGlobal('matchMedia', () => ({ matches: false }));
  });

  it('animates the box from where the panel was to where the switch puts it, then lets go', async () => {
    const node = document.createElement('div');
    const rects = [rect(120, 300, 900, 600), rect(0, 0, 1400, 900)];
    node.getBoundingClientRect = () => rects.shift()!;
    const anim = { onfinish: null as null | (() => void), oncancel: null };
    node.animate = vi.fn(() => anim) as unknown as typeof node.animate;
    const apply = vi.fn(() => node.classList.add('bg-white'));

    await growFullscreen(node, apply);

    expect(apply).toHaveBeenCalledOnce();
    expect(node.animate).toHaveBeenCalledWith(
      [
        expect.objectContaining({ top: '120px', left: '300px', width: '900px', height: '600px' }),
        expect.objectContaining({ top: '0px', left: '0px', width: '1400px', height: '900px' })
      ],
      expect.objectContaining({ duration: 200 })
    );
    expect(node.style.position).toBe('fixed');
    anim.onfinish!();
    expect(node.style.position).toBe('');
    expect(node.style.zIndex).toBe('');
  });

  it('moves the padding with the box, so differently padded ends do not jump', async () => {
    const node = document.body.appendChild(document.createElement('div'));
    node.style.padding = '16px';
    node.getBoundingClientRect = () => rect(0, 0, 10, 10);
    node.animate = vi.fn(() => ({})) as unknown as typeof node.animate;
    await growFullscreen(node, () => (node.style.padding = '12px'));
    const [frames] = (node.animate as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(frames.map((f: Keyframe) => f.padding)).toEqual(['16px', '12px']);
  });

  // A second click inside the 200ms starts from the panel's own layout: the
  // first run's fixed positioning is let go before the far end is measured,
  // since a browser reports the cancel only later.
  it('lets go of a run still in flight before measuring the next', async () => {
    const node = document.createElement('div');
    node.getBoundingClientRect = () => rect(0, 0, 10, 10);
    const runs: { cancel: () => void; onfinish: null | (() => void); oncancel: null | (() => void) }[] = [];
    node.animate = vi.fn(() => {
      const run = { onfinish: null as null | (() => void), oncancel: null as null | (() => void), cancel: () => {} };
      runs.push(run);
      return run;
    }) as unknown as typeof node.animate;

    await growFullscreen(node, () => {});
    expect(node.style.position).toBe('fixed');
    let positionAtSwitch = 'unset';
    await growFullscreen(node, () => (positionAtSwitch = node.style.position));
    expect(positionAtSwitch).toBe('');
    expect(node.style.position).toBe('fixed');

    // The first run's late cancel event must not undo the second run.
    runs[0].oncancel?.();
    expect(node.style.position).toBe('fixed');
    runs[1].onfinish!();
    expect(node.style.position).toBe('');
  });

  it('just switches when the user asks for less motion', async () => {
    vi.stubGlobal('matchMedia', () => ({ matches: true }));
    const node = document.createElement('div');
    node.animate = vi.fn() as unknown as typeof node.animate;
    const apply = vi.fn();
    await growFullscreen(node, apply);
    expect(apply).toHaveBeenCalledOnce();
    expect(node.animate).not.toHaveBeenCalled();
  });
});
