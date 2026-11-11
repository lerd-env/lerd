import { render } from '@testing-library/svelte';
import { it, expect, vi } from 'vitest';
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

// A page can hand a file to the dashboard's editor and get the saved text back.
it('edits a file for its page and answers with the saved text, or null on cancel', async () => {
  const { container, findByText, queryByText } = render(PluginFrame, {
    props: { plugin: 'hello', url: '/plugins/hello/ui/site.html', title: 'Hello' }
  });
  const frame = container.querySelector('iframe')!;
  const sent: any[] = [];
  frame.contentWindow!.postMessage = ((m: unknown) => sent.push(m)) as Window['postMessage'];
  const ask = (id: number) =>
    window.dispatchEvent(new MessageEvent('message', { source: frame.contentWindow, data: { type: 'lerd:edit', id, path: '.lerd/scrub.sql', content: 'DELETE FROM x;', label: 'after a pull' } }));

  ask(7);
  expect(await findByText('.lerd/scrub.sql')).toBeTruthy();
  expect(sent).toContainEqual({ type: 'lerd:edit:opened', id: 7 });
  (await findByText('Save')).click();
  await vi.waitFor(() => expect(sent).toContainEqual({ type: 'lerd:edit:result', id: 7, content: 'DELETE FROM x;' }));
  await vi.waitFor(() => expect(queryByText('.lerd/scrub.sql')).toBeNull());

  ask(8);
  (await findByText('Cancel')).click();
  await vi.waitFor(() => expect(sent).toContainEqual({ type: 'lerd:edit:result', id: 8, content: null }));
});
