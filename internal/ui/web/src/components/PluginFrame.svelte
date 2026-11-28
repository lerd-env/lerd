<script lang="ts">
  import { apiUrl } from '$lib/api';
  import { answerPluginRequest, openPluginTerminal, pluginDiffRequest, pluginMaximizeRequest, pluginEditRequest, pluginTheme, type PluginDiff, type PluginEdit } from '$lib/pluginBridge';
  import PluginDiffModal from './PluginDiffModal.svelte';
  import { locale } from '$stores/locale';

  interface Props {
    plugin: string;
    url: string;
    title: string;
    // Handed to the page as query parameters (site, domain, branch on a site tab).
    context?: Record<string, string>;
    // autoHeight sizes the frame to the page inside, which reports its height
    // through ui/lerd.js; an Overview card has no space of its own to fill.
    autoHeight?: boolean;
  }
  let { plugin, url, title, context = {}, autoHeight = false }: Props = $props();

  let frame = $state<HTMLIFrameElement | null>(null);
  // A page may ask to cover the whole dashboard, a terminal for one; only its
  // own button brings it back, since Escape belongs to what runs inside.
  let maximized = $state(false);
  let height = $state(48);
  let diff = $state<PluginDiff | null>(null);
  // The page that asked is kept with the edit, so the answer goes back to it
  // even if the frame has since loaded another page.
  let editing = $state<(PluginEdit & { source: Window }) | null>(null);

  // Every edit is answered exactly once: the saved text, or null when it is
  // cancelled, replaced by another, or its page is replaced before a save.
  function answerEdit(content: string | null) {
    if (!editing) return;
    editing.source.postMessage({ type: 'lerd:edit:result', id: editing.id, content }, '*');
    editing = null;
  }

  // Terminals the page opened; none outlives the page that holds its port.
  let terminals: (() => void)[] = [];

  // A new src replaces the page in the frame, which can no longer save.
  $effect(() => {
    void src;
    return () => {
      maximized = false;
      answerEdit(null);
      terminals.splice(0).forEach((close) => close());
    };
  });

  // The page learns the dashboard's language with its context, so it can speak
  // it too; a change of language reloads the dashboard and with it the page.
  const src = $derived.by(() => {
    const q = new URLSearchParams({ ...context, lang: $locale }).toString();
    return apiUrl(url) + (q ? '?' + q : '');
  });

  $effect(() => {
    const onMessage = (e: MessageEvent) => {
      if (autoHeight && e.source === frame?.contentWindow && e.data?.type === 'lerd:resize') {
        // Capped, so a page that grows without end cannot push the Overview away.
        height = Math.min(Math.max(Number(e.data.height) || 0, 24), 800);
        return;
      }
      const asked = pluginDiffRequest(e, frame?.contentWindow);
      if (asked) {
        // One dialog at a time: a viewer behind the editor would take its Escape.
        if (!editing) diff = asked;
        return;
      }
      const edit = pluginEditRequest(e, frame?.contentWindow);
      if (edit) {
        answerEdit(null);
        diff = null;
        editing ={ ...edit, source: e.source as Window };
        // Tells the page this lerd has an editor, so it does not fall back to its own.
        frame?.contentWindow?.postMessage({ type: 'lerd:edit:opened', id: edit.id }, '*');
        return;
      }
      const max = pluginMaximizeRequest(e, frame?.contentWindow);
      if (max !== null) {
        maximized = max;
        // Answering tells the page this lerd can, and where it ended up.
        frame?.contentWindow?.postMessage({ type: 'lerd:maximized', on: max }, '*');
        return;
      }
      const terminal = openPluginTerminal(e, plugin, frame?.contentWindow);
      if (terminal) {
        terminals.push(terminal);
        return;
      }
      void answerPluginRequest(e, plugin, frame?.contentWindow);
    };
    window.addEventListener('message', onMessage);
    return () => window.removeEventListener('message', onMessage);
  });

  // The page draws in the dashboard's theme: told on load, and again whenever
  // the theme moves the root's class or palette.
  const sendTheme = () => frame?.contentWindow?.postMessage(pluginTheme(), '*');
  $effect(() => {
    const watch = new MutationObserver(sendTheme);
    watch.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'style'] });
    return () => watch.disconnect();
  });
</script>

<!-- No allow-same-origin: the page must never act with the dashboard's standing.
     It reaches its own endpoints through answerPluginRequest instead. -->
{#key src}
  <iframe
    bind:this={frame}
    {src}
    {title}
    sandbox="allow-scripts allow-forms allow-popups allow-modals"
    onload={sendTheme}
    class="{maximized ? 'fixed inset-0 z-40 h-screen bg-white dark:bg-lerd-card' : autoHeight ? 'block' : 'flex-1'} w-full border-0 {maximized ? '' : 'bg-transparent'}"
    style:height={autoHeight && !maximized ? `${height}px` : undefined}
  ></iframe>
{/key}
{#if diff}
  <PluginDiffModal {diff} onclose={() => (diff = null)} />
{/if}
{#if editing}
  <PluginDiffModal diff={{ path: editing.path, modified: editing.content, after: editing.label }} onclose={() => answerEdit(null)} onsave={(text) => answerEdit(text)} />
{/if}
