import { describe, it, expect, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('$lib/api', () => ({ apiJson: () => new Promise(() => {}) }));

import { editors, editorTitle } from './editors';
import { sites, type Site } from './sites';

describe('editorTitle', () => {
  it("names the site's own editor, then the global one, then none", () => {
    editors.set({ editors: [{ id: 'phpstorm', label: 'PhpStorm', installed: true }, { id: 'zed', label: 'Zed', installed: true }], global: 'zed' });
    sites.set([{ name: 'shop', path: '/srv/shop', editor: 'phpstorm' } as Site, { name: 'blog', path: '/srv/blog' } as Site]);
    const title = get(editorTitle);
    expect(title('/srv/shop/app/Cart.php')).toBe('Open in PhpStorm');
    expect(title('/srv/blog/index.php')).toBe('Open in Zed');
    editors.set({ editors: [], global: '' });
    expect(get(editorTitle)('/srv/blog/index.php')).toBe('Open in editor');
  });
});
