import { render } from '@testing-library/svelte';
import { it, expect } from 'vitest';
import PluginFrame from './PluginFrame.svelte';

// The frame is what keeps a plugin page off lerd's origin: scripts yes, the
// dashboard's own standing never.
it('sandboxes the page and hands it its context', () => {
  // Wrapped in props: `context` is also a Svelte mount option.
  const { container } = render(PluginFrame, {
    props: {
      plugin: 'hello',
      url: '/plugins/hello/ui/site.html',
      title: 'Hello',
      context: { site: 'shop', domain: 'shop.test', branch: '' }
    }
  });
  const frame = container.querySelector('iframe')!;
  const sandbox = frame.getAttribute('sandbox') ?? '';
  expect(sandbox).toContain('allow-scripts');
  expect(sandbox).not.toContain('allow-same-origin');
  expect(frame.getAttribute('src')).toBe('/plugins/hello/ui/site.html?site=shop&domain=shop.test&branch=');
});
