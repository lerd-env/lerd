<script lang="ts">
  import Popover from './Popover.svelte';
</script>

<aside class="z-20" style="isolation: isolate">
  <Popover label="Open">
    {#snippet trigger()}
      <span>bell</span>
    {/snippet}
    {#snippet children(close: () => void)}
      <button type="button" onclick={close}>panel item</button>
    {/snippet}
  </Popover>
</aside>
