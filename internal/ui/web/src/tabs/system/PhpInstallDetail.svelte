<script lang="ts">
  import DetailPanel from '$components/DetailPanel.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { apiUrl } from '$lib/api';
  import { phpVersions, loadPhpVersions } from '$stores/phpVersions';
  import { goToTab } from '$stores/route';

  // Lerd supports 5.6 → 8.5. Older minors (7.0/7.1/7.3) exist on Docker
  // Hub but aren't tracked by lerd's xdebug/oci8 pinning, so they're
  // omitted — add an entry when they're promoted.
  interface AvailableVersion {
    version: string;
    label: string;
    note: string;
  }
  const ALL_VERSIONS: AvailableVersion[] = [
    { version: '5.6', label: 'PHP 5.6', note: 'Legacy estendida — base Alpine 3.8 (oci8 2.0.12, xdebug 2.5.5). Sem memcached/amqp/pcov/spx.' },
    { version: '7.4', label: 'PHP 7.4', note: 'Legacy tier — para apps antigos (oci8 2.2.0, xdebug 3.1).' },
    { version: '8.0', label: 'PHP 8.0', note: 'Legacy tier (oci8 3.0.1, xdebug 3.3).' },
    { version: '8.1', label: 'PHP 8.1', note: 'Estável (oci8 3.3.0).' },
    { version: '8.2', label: 'PHP 8.2', note: 'Estável (oci8 3.3.0).' },
    { version: '8.3', label: 'PHP 8.3', note: 'Atual (oci8 3.3.0). Recomendada para Laravel 11.' },
    { version: '8.4', label: 'PHP 8.4', note: 'Latest stable (oci8 3.4.1). Recomendada para Laravel 13.' },
    { version: '8.5', label: 'PHP 8.5', note: 'Bleeding edge (oci8 3.4.1).' }
  ];

  const missing = $derived(
    ALL_VERSIONS.filter((v) => !$phpVersions.includes(v.version))
  );

  let installing = $state(''); // version currently installing (empty when idle)
  let installError = $state('');
  let installSuccess = $state('');
  let logLines = $state<string[]>([]);
  let logEl = $state<HTMLPreElement | undefined>();

  // Auto-scroll the log pane as new lines arrive.
  $effect(() => {
    // Touch logLines.length so the effect re-runs on every new line.
    void logLines.length;
    if (logEl) logEl.scrollTop = logEl.scrollHeight;
  });

  // Block navigation while an install is running so the user doesn't
  // close the tab in the middle of a podman build and end up with a
  // half-built image. The browser's native prompt is the only reliable
  // mechanism — its message text is browser-controlled.
  $effect(() => {
    if (!installing) return;
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = '';
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  });

  async function onInstall(version: string) {
    if (installing) return;
    installing = version;
    installError = '';
    installSuccess = '';
    logLines = [];

    // POST with streaming SSE response. fetch+ReadableStream is the
    // simplest way to consume an SSE endpoint that's keyed by POST
    // (EventSource only does GET).
    let res: Response;
    try {
      res = await fetch(apiUrl('/api/php-versions/' + encodeURIComponent(version) + '/install'), {
        method: 'POST'
      });
    } catch (e) {
      installError = e instanceof Error ? e.message : String(e);
      installing = '';
      return;
    }
    if (!res.ok || !res.body) {
      installError = `request failed (${res.status})`;
      installing = '';
      return;
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buf = '';

    // SSE frame parser: split on blank lines, strip event/data prefixes.
    // The backend emits one frame per stdout line, so frames are tiny.
    try {
      let done = false;
      while (!done) {
        const { value, done: readerDone } = await reader.read();
        done = readerDone;
        if (value) buf += decoder.decode(value, { stream: !done });
        let idx: number;
        while ((idx = buf.indexOf('\n\n')) !== -1) {
          const frame = buf.slice(0, idx);
          buf = buf.slice(idx + 2);
          let event = 'log';
          let data = '';
          for (const line of frame.split('\n')) {
            if (line.startsWith('event: ')) event = line.slice(7);
            else if (line.startsWith('data: ')) data += (data ? '\n' : '') + line.slice(6);
          }
          if (event === 'log') {
            logLines = [...logLines, data];
          } else if (event === 'done') {
            try {
              const payload = JSON.parse(data) as { ok?: boolean; error?: string };
              if (payload.ok) {
                installSuccess = `PHP ${version} instalado e iniciado.`;
                await loadPhpVersions();
                setTimeout(() => goToTab('system', 'php-' + version), 1200);
              } else {
                installError = payload.error || 'falha na instalação';
              }
            } catch {
              installError = 'invalid done frame: ' + data;
            }
          }
        }
      }
    } catch (e) {
      installError = e instanceof Error ? e.message : String(e);
    } finally {
      installing = '';
    }
  }

  function copyLogs() {
    if (logLines.length === 0) return;
    navigator.clipboard.writeText(logLines.join('\n')).catch(() => {});
  }
</script>

<DetailPanel>
  <div class="px-3 sm:px-5 py-4 border-b border-gray-100 dark:border-lerd-border">
    <h2 class="font-semibold text-gray-900 dark:text-white text-base">Instalar versão PHP</h2>
    <p class="text-xs text-gray-400 mt-1 leading-relaxed">
      Provisiona uma nova versão PHP-FPM: build (ou pull) da imagem base, escreve o quadlet systemd e inicia o container. Pode levar 1–3 minutos por versão na primeira vez. Equivalente a <code class="font-mono text-gray-500">lerd php:install &lt;versão&gt;</code>.
    </p>
    {#if installing}
      <p class="text-[11px] text-amber-600 dark:text-amber-400 mt-2 leading-relaxed flex items-center gap-1.5">
        <span class="inline-block w-1.5 h-1.5 bg-amber-500 rounded-full animate-pulse"></span>
        Não feche esta aba durante a instalação — o build seria interrompido e a imagem pode ficar parcial.
      </p>
    {/if}
  </div>

  <div class="px-3 sm:px-5 py-4 space-y-2 shrink-0">
    {#if missing.length === 0}
      <p class="text-sm text-gray-400 italic">Todas as versões suportadas já estão instaladas.</p>
    {:else}
      {#each missing as v (v.version)}
        <div class="flex items-center justify-between gap-3 px-3 py-2.5 bg-gray-50 dark:bg-white/5 border border-gray-200 dark:border-lerd-border rounded">
          <div class="min-w-0">
            <p class="text-sm font-medium text-gray-900 dark:text-gray-100">{v.label}</p>
            <p class="text-[11px] text-gray-400 mt-0.5">{v.note}</p>
          </div>
          <DetailButton
            tone="success"
            onclick={() => onInstall(v.version)}
            disabled={installing !== ''}
            loading={installing === v.version}
            title={`Build + quadlet + start para PHP ${v.version}`}
          >{installing === v.version ? 'Instalando…' : 'Instalar'}</DetailButton>
        </div>
      {/each}
    {/if}

    {#if installSuccess}
      <div class="text-xs font-medium text-emerald-700 dark:text-emerald-400 bg-emerald-50 dark:bg-emerald-500/10 rounded-lg px-3 py-2">
        {installSuccess}
      </div>
    {/if}
    {#if installError}
      <div class="text-xs font-medium text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-500/10 rounded-lg px-3 py-2 break-words">
        {installError}
      </div>
    {/if}
  </div>

  {#if logLines.length > 0}
    <div class="flex-1 flex flex-col min-h-0 border-t border-gray-200 dark:border-lerd-border">
      <div class="flex items-center justify-between px-3 py-1.5 bg-gray-50 dark:bg-white/3 border-b border-gray-200 dark:border-lerd-border shrink-0">
        <span class="text-[10px] font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider">
          Build log {installing ? '— em andamento' : '— finalizado'}
        </span>
        <button
          type="button"
          onclick={copyLogs}
          class="text-[10px] text-gray-500 hover:text-gray-700 dark:hover:text-gray-300 transition-colors"
        >Copiar log</button>
      </div>
      <pre
        bind:this={logEl}
        class="flex-1 overflow-auto px-3 py-2 text-[10px] font-mono text-gray-700 dark:text-gray-300 bg-black/40 whitespace-pre"
      >{logLines.join('\n')}</pre>
    </div>
  {/if}
</DetailPanel>
