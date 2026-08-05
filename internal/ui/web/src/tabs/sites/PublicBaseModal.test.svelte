<script lang="ts">
  import PublicBaseModal from './PublicBaseModal.svelte';

  // One signal per prop: testing-library's rerender swaps the whole props
  // object, which invalidates every read and would hide the untracked seeding
  // this harness exists to exercise.
  let base = $state('');
</script>

<button data-testid="refresh-base" onclick={() => (base = 'dev.example.com')}>refresh</button>

<PublicBaseModal open={true} {base} onclose={() => {}} onsubmit={() => {}} />
