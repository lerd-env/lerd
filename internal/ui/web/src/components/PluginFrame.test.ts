import { render } from '@testing-library/svelte';
import { it, expect } from 'vitest';
import PluginFrame from './PluginFrame.svelte';

// The frame is what keeps a plugin page off lerd's origin: scripts yes, the
// dashboard's own standing never.
it('sandboxes the page and hands it its context and the dashboard language', () => {
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
  expect(frame.getAttribute('src')).toBe('/plugins/hello/ui/site.html?site=shop&domain=shop.test&branch=&lang=en');
});

// A page can ask the dashboard to show two versions of a file in its own diff.
it('opens the dashboard diff when its page asks for one', async () => {
  const { container, findByText } = render(PluginFrame, {
    props: { plugin: 'hello', url: '/plugins/hello/ui/site.html', title: 'Hello' }
  });
  const frame = container.querySelector('iframe')!;
  window.dispatchEvent(
    new MessageEvent('message', {
      source: frame.contentWindow,
      data: { type: 'lerd:diff', path: 'wp-content/uploads/a.txt', original: 'one', modified: 'two', before: 'production', after: 'This site' }
    })
  );
  expect(await findByText('wp-content/uploads/a.txt')).toBeTruthy();
  expect(await findByText('production')).toBeTruthy();
});

it('opens one file read only when its page asks to view it', async () => {
  const { container, findByText } = render(PluginFrame, {
    props: { plugin: 'hello', url: '/plugins/hello/ui/site.html', title: 'Hello' }
  });
  const frame = container.querySelector('iframe')!;
  window.dispatchEvent(
    new MessageEvent('message', { source: frame.contentWindow, data: { type: 'lerd:view', path: 'notes/only-here.txt', content: 'hello', label: 'production' } })
  );
  expect(await findByText('notes/only-here.txt')).toBeTruthy();
  expect(await findByText('production')).toBeTruthy();
  expect(container.ownerDocument.body.textContent).not.toContain('→');
});
