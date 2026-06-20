// Demo entry: the real lerd UI, fed fixtures instead of a daemon.
import './stubs'; // MUST be first — patches fetch/WebSocket before the app loads
import { mount } from 'svelte';
import App from '$src/App.svelte';
import '$src/app.css';
import { initTheme } from '$stores/theme';

initTheme();

const target = document.getElementById('app');
if (!target) throw new Error('missing #app root');

mount(App, { target });
