import { get } from 'svelte/store';
import { profilerEnabled, setProfiler, captureCount, waitForCapture } from '$stores/profiler';

export type ProfilePhase = 'arming' | 'waiting';

// profileRoute arms the profiler, opens the route and reports whether SPX caught
// it. Arming returns once nginx serves the new config, so the request cannot miss
// the profiler; a profiler this armed is turned back off rather than left on.
export async function profileRoute(host: string, route: string, url: string, onPhase: (p: ProfilePhase) => void): Promise<boolean> {
  const armedHere = !get(profilerEnabled);
  try {
    const before = await captureCount(host, route);
    if (armedHere) {
      onPhase('arming');
      await setProfiler(true);
    }
    // Opened once, here, with the real URL. Holding a blank tab open across the
    // arming wait leaves an about:blank the desktop is asked to find an
    // application for when the dashboard runs as an app window.
    window.open(url, '_blank');
    onPhase('waiting');
    return await waitForCapture(host, route, before);
  } catch {
    return false;
  } finally {
    if (armedHere) {
      try {
        await setProfiler(false);
      } catch {
        /* it stays armed; the toggle is one click away */
      }
    }
  }
}
