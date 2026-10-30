<script lang="ts">
  // The cup that marks anything pinned awake, the counterpart of the sleep moon.
  interface Props {
    class?: string;
  }
  let { class: cls = 'w-4 h-4' }: Props = $props();
</script>

<svg class={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  <path d="M17 8h1a4 4 0 1 1 0 8h-1" />
  <path d="M3 8h14v9a4 4 0 0 1-4 4H7a4 4 0 0 1-4-4Z" />
  <path d="M6 2v2M10 2v2M14 2v2" />
</svg>
