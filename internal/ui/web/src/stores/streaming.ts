import { get } from 'svelte/store';
import { wsMessage } from '$lib/ws';
import { sites } from './sites';
import { status } from './status';

// The server sends streaming_on before it rebuilds the snapshots, so a share
// that just started never waits on that rebuild to hide anything.
wsMessage.subscribe((msg) => {
  if (msg?.type !== 'streaming_on') return;
  const st = get(status);
  const priv = st.private_workspaces ?? [];
  sites.set(get(sites).filter((s) => !s.hidden_while_streaming));
  status.set({ ...st, streaming_mode: true, workspaces: (st.workspaces ?? []).filter((w) => !priv.includes(w)) });
});
