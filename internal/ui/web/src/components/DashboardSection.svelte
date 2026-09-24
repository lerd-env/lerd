<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    label?: string; // omitted for a bucket that needs no heading
    sub?: boolean;
    // Drawn before the label when the heading has a mark of its own; the
    // section itself stays unaware of what the mark is.
    icon?: Snippet;
    // Drawn after the label, e.g. a control for the section's workspace.
    trailing?: Snippet;
    children: Snippet;
  }
  let { label, sub = false, icon, trailing, children }: Props = $props();

  const headingClass = $derived(
    (sub ? 'text-[11px]' : 'text-xs') +
      ' group flex items-center gap-1.5 font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500'
  );
</script>

<section class={sub ? 'space-y-2' : 'space-y-2.5'}>
  {#if label && sub}
    <h3 class={headingClass}>{#if icon}{@render icon()}{/if}{label}{#if trailing}{@render trailing()}{/if}</h3>
  {:else if label}
    <h2 class={headingClass}>{#if icon}{@render icon()}{/if}{label}{#if trailing}{@render trailing()}{/if}</h2>
  {/if}
  <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-2">
    {@render children()}
  </div>
</section>
