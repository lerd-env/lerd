import { describe, it, expect } from 'vitest';
import { logHighlight } from './logHighlight';

const RED = 'text-red-500';
const YELLOW = 'text-yellow-600 dark:text-yellow-400';
const BLUE = 'text-blue-600 dark:text-blue-400';
const GREEN = 'text-emerald-600 dark:text-emerald-500';

describe('logHighlight', () => {
  it('paints php-fpm levels', () => {
    expect(logHighlight('[15-Sep-2026 10:06:05] NOTICE: fpm is running, pid 1')).toBe(BLUE);
    expect(logHighlight('[15-Sep-2026 10:06:05] WARNING: child 12 exited')).toBe(YELLOW);
    expect(logHighlight('[15-Sep-2026 10:06:05] ERROR: unable to bind listening socket')).toBe(RED);
  });

  it('ranks PHP diagnostics above the surrounding level word', () => {
    expect(logHighlight('PHP Warning:  Undefined variable $x in /app/index.php')).toBe(RED);
    expect(logHighlight('PHP Fatal error: Uncaught TypeError')).toBe(RED);
    expect(logHighlight('PHP Deprecated:  Passing null to parameter')).toBe(YELLOW);
  });

  it('paints nginx levels whatever their case', () => {
    expect(logHighlight('2026/09/15 15:23:35 [error] 7#7: *1 open() failed')).toBe(RED);
    expect(logHighlight('2026/09/15 15:23:35 [warn] 7#7: conflicting server name')).toBe(YELLOW);
    expect(logHighlight('2026/09/15 15:23:35 [crit] 7#7: SSL_do_handshake() failed')).toBe(RED);
  });

  it('paints access lines by status', () => {
    const fpm = (code: number) => `10.89.0.157 -  15/Sep/2026:15:23:35 +0000 "GET /index.php" ${code}`;
    expect(logHighlight(fpm(200))).toBe(GREEN);
    expect(logHighlight(fpm(404))).toBe(YELLOW);
    expect(logHighlight(fpm(500))).toBe(RED);
    expect(logHighlight('127.0.0.1 - - [15/Sep/2026:15:23:35] "GET / HTTP/1.1" 301 178')).toBe(GREEN);
  });

  it('leaves anything else to the default colour', () => {
    expect(logHighlight('Xdebug: [Step Debug] Could not connect to debugging client.')).toBeNull();
    expect(logHighlight('query[A] app.test from 127.0.0.1')).toBeNull();
  });
});
