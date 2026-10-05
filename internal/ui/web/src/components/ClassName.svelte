<script lang="ts">
  import { shortClass } from '$lib/sourceLabel';

  // A class, or a text holding classes, by its own name, the namespace in the
  // title for whoever needs it.
  let { value, class: cls = '' }: { value: string; class?: string } = $props();
  const short = $derived(shortClass(value));
</script>

<span class={cls} title={short !== value ? value : undefined}>{short}</span>
