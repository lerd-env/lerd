<script lang="ts">
  // The logo has a palette for light surfaces and one for dark; the app theme picks.
  let { class: cls = '' }: { class?: string } = $props();
  const base = import.meta.env.BASE_URL;
</script>

<img src={`${base}icons/icon.svg`} class="dark:hidden {cls}" alt="Lerd" />
<img src={`${base}icons/icon-dark.svg`} class="hidden dark:block {cls}" alt="Lerd" />
