// What a note pinned from the debug bar says about its element: a selector
// that finds it again, and enough of it to recognise on the page.

const TEST_ATTRS = ['data-testid', 'data-test', 'data-cy', 'data-qa'];

const unique = (sel: string, root: ParentNode = document) => {
  try {
    return root.querySelectorAll(sel).length === 1;
  } catch {
    return false;
  }
};

// step names one element among its siblings: tag, its stable classes when
// they single it out, otherwise its place among the siblings of its tag.
function step(el: Element): string {
  const tag = el.tagName.toLowerCase();
  const parent = el.parentElement;
  if (!parent) return tag;
  const classes = [...el.classList].filter((c) => /^[A-Za-z_-][\w-]*$/.test(c) && !/^(is-|has-|hover|focus|active)/.test(c)).slice(0, 3);
  if (classes.length) {
    const sel = `${tag}.${classes.map((c) => CSS.escape(c)).join('.')}`;
    if ([...parent.children].filter((c) => c.matches(sel)).length === 1) return sel;
  }
  const same = [...parent.children].filter((c) => c.tagName === el.tagName);
  return same.length > 1 ? `${tag}:nth-of-type(${same.indexOf(el) + 1})` : tag;
}

// selectorFor returns a selector that matches the element and nothing else on
// the page: its id or a test attribute when unique, otherwise a path up to the
// nearest ancestor that has one, or to the body.
export function selectorFor(el: Element): string {
  const own = anchor(el);
  if (own) return own;
  const parts: string[] = [];
  let cur: Element | null = el;
  while (cur && cur !== document.body && cur !== document.documentElement) {
    parts.unshift(step(cur));
    const sel = parts.join(' > ');
    const at = cur.parentElement ? anchor(cur.parentElement) : null;
    if (at && unique(`${at} > ${sel}`)) return `${at} > ${sel}`;
    if (unique(sel) && document.querySelector(sel) === el) return sel;
    cur = cur.parentElement;
  }
  return ['body', ...parts].join(' > ');
}

function anchor(el: Element): string | null {
  if (el.id) {
    const sel = `#${CSS.escape(el.id)}`;
    if (unique(sel)) return sel;
  }
  for (const a of TEST_ATTRS) {
    const v = el.getAttribute(a);
    if (v) {
      const sel = `[${a}="${CSS.escape(v)}"]`;
      if (unique(sel)) return sel;
    }
  }
  return null;
}

export interface ElementInfo {
  tag: string;
  text: string;
  rect: { x: number; y: number; w: number; h: number };
}

// describeElement is the element's tag, the start of its text, and where it
// sits on the page, scrolled or not.
export function describeElement(el: Element): ElementInfo {
  const r = el.getBoundingClientRect();
  const text = (el.textContent ?? '').replace(/\s+/g, ' ').trim().slice(0, 120);
  return { tag: el.tagName.toLowerCase(), text, rect: { x: Math.round(r.left + scrollX), y: Math.round(r.top + scrollY), w: Math.round(r.width), h: Math.round(r.height) } };
}

// notePlace puts the comment box under the element, or above it when there is
// no room below, kept inside the viewport.
export function notePlace(r: { left: number; top: number; bottom: number; right: number }, vw: number, vh: number, w = 320, h = 200, margin = 8): { left: number; top: number } {
  const left = Math.max(margin, Math.min(r.left, vw - w - margin));
  const below = r.bottom + margin;
  const top = below + h <= vh - margin ? below : Math.max(margin, Math.min(r.top - h - margin, vh - h - margin));
  return { left, top };
}

