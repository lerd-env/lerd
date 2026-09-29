<script lang="ts">
  // The logo drawn in the theme's accent, its letter masked out of the brand
  // icon so it keeps the Nunito L while the colours follow the theme.
  let { class: cls = '' }: { class?: string } = $props();
</script>

<span role="img" aria-label="Lerd" class="relative block shrink-0 bg-lerd-red {cls}">
  <span class="absolute inset-0 bg-lerd-onred" style="mask: url(/icons/mark.svg) center / contain no-repeat"></span>
</span>
