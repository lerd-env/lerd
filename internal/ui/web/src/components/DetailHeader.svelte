<script lang="ts">
  import type { Snippet } from 'svelte';
  import BackButton from '$components/BackButton.svelte';
  import { isDesktop } from '$lib/media';

  interface Props {
    title: string;
    // back shows the phone's way back to the list; a page stacking two
    // headers turns it off on the second.
    back?: boolean;
    // beside sits next to the title (badges); trailing at the far end (actions).
    beside?: Snippet;
    trailing?: Snippet;
  }
  let { title, back = true, beside, trailing }: Props = $props();
</script>

<div
  class="flex flex-wrap items-center justify-between gap-y-2 px-3 py-1.5 page-header shrink-0"
>
  <div class="flex items-center gap-3 min-w-0">
    <!-- A phone has no side panel, so the way back to the list sits here. -->
    {#if back && !$isDesktop}<span class="-ml-1"><BackButton /></span>{/if}
    <span class="font-semibold text-gray-900 dark:text-white text-base">{title}</span>
    {#if beside}{@render beside()}{/if}
  </div>
  {#if trailing}{@render trailing()}{/if}
</div>
