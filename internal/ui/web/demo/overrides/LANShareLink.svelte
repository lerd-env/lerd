<script lang="ts">
  // Demo override of the real LANShareLink: the QR image is served by the
  // daemon (/api/lan-qr/...) which doesn't exist here, so we ship a static QR
  // that encodes https://lerd.sh instead. The hover tooltip behaviour matches
  // the original component.
  import qrSrc from './lerd-qr.svg';
  import { m } from '$src/paraglide/messages.js';

  interface Props {
    domain: string;
    url: string;
    siteDomain?: string;
    branch?: string;
  }
  let { url }: Props = $props();

  let show = $state(false);
  let x = $state(0);
  let y = $state(0);

  function onEnter(e: Event) {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    x = r.left;
    y = r.bottom + 4;
    show = true;
  }
  function onLeave() {
    show = false;
  }
</script>

<div
  role="tooltip"
  onmouseenter={onEnter}
  onmouseleave={onLeave}
  onfocusin={onEnter}
  onfocusout={onLeave}
>
  <a
    href="https://lerd.sh"
    target="_blank"
    rel="noopener"
    class="text-[10px] text-teal-600 dark:text-teal-400 font-mono hover:underline">{url}</a
  >
  {#if show}
    <div
      style="position:fixed; left:{x}px; top:{y}px; z-index:9999"
      class="p-1.5 bg-white dark:bg-lerd-card rounded-sm shadow-lg border border-gray-200 dark:border-lerd-border"
    >
      <img src={qrSrc} width="160" height="160" alt={m.lanShare_qrAlt()} />
    </div>
  {/if}
</div>
