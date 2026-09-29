<script lang="ts">
  import Icon from '$components/Icon.svelte';

  interface Props {
    label: string;
  }
  let { label }: Props = $props();
</script>

<div class="px-5 py-10 flex flex-col items-center gap-2 text-gray-500 dark:text-gray-400">
  <Icon name="spinner" class="w-5 h-5 animate-spin" />
  <span class="text-xs">{label}</span>
</div>
