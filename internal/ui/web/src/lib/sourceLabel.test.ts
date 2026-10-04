import { describe, it, expect } from 'vitest';
import { relativeTo, callerClass } from './sourceLabel';

describe('source labels', () => {
  it('shows a path from the deepest project root that holds it', () => {
    expect(relativeTo('/home/u/work/app/routes/web.php', ['/home/u/work', '/home/u/work/app'])).toBe('routes/web.php');
    expect(relativeTo('/usr/local/etc/lerd/x.php', ['/home/u/work'])).toBe('/usr/local/etc/lerd/x.php');
  });

  it('names the class a call was made in from the frame after it', () => {
    const trace = [
      { file: '/app/vendor/Builder.php', line: 9, func: 'Illuminate\\Database\\Connection->select' },
      { file: '/app/app/Models/Order.php', line: 42, func: 'Illuminate\\Database\\Eloquent\\Builder->get' },
      { file: '/app/app/Http/Controllers/OrderController.php', line: 12, func: 'App\\Models\\Order->latestPaid' }
    ];
    expect(callerClass('/app/app/Models/Order.php', 42, trace)).toBe('App\\Models\\Order');
    expect(callerClass('/app/routes/web.php', 3, trace)).toBe('');
    const route = [
      { file: '/app/routes/web.php', line: 22, func: 'Lerd\\Debug\\Event->run' },
      { file: '/app/vendor/Router.php', line: 9, func: 'Illuminate\\Routing\\RouteFileRegistrar->{closure:/app/routes/web.php:9}' }
    ];
    expect(callerClass('/app/routes/web.php', 22, route)).toBe('');
  });
});
