<script lang="ts">
  import type { Snippet } from 'svelte';

  // A single settings section rendered as a card. The section supplies its own
  // header (title + status pill) and controls as children; this just provides the
  // consistent card shell shared across the System > lerd page.
  // class lets a column stretch its cards so a short one does not leave the
  // page background showing through next to a taller neighbour.
  let { children, class: klass = '' }: { children: Snippet; class?: string } = $props();
</script>

<div class="rounded-xl border border-gray-100 dark:border-lerd-border bg-white dark:bg-lerd-card p-4 {klass}">
  {@render children()}
</div>
