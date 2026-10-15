<script lang="ts">
  import { apiUrl } from '$lib/api';
  import { answerPluginRequest } from '$lib/pluginBridge';

  interface Props {
    plugin: string;
    url: string;
    title: string;
    // Handed to the page as query parameters (site, domain, branch on a site tab).
    context?: Record<string, string>;
  }
  let { plugin, url, title, context = {} }: Props = $props();

  let frame = $state<HTMLIFrameElement | null>(null);

  const src = $derived.by(() => {
    const q = new URLSearchParams(context).toString();
    return apiUrl(url) + (q ? '?' + q : '');
  });

  $effect(() => {
    const onMessage = (e: MessageEvent) => void answerPluginRequest(e, plugin, frame?.contentWindow);
    window.addEventListener('message', onMessage);
    return () => window.removeEventListener('message', onMessage);
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
    class="flex-1 w-full border-0 bg-transparent"
  ></iframe>
{/key}
