<script lang="ts">
  let { original, modified, language, ignoreSpace }: { original: string; modified: string; language: string; ignoreSpace: boolean } =
    $props();
</script>

<div data-testid="diff" data-original={original} data-modified={modified} data-language={language} data-ignore-space={String(ignoreSpace)}></div>
