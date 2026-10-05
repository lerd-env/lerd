import { describe, it, expect } from 'vitest';
import { funcLabel, packageOf, traceRows, traceRuns, traceText } from './traceFrames';

const trace = [
  { file: '/app/vendor/laravel/framework/src/Facade.php', line: 364, func: 'Illuminate\\Cache\\CacheManager->__call' },
  { file: '/app/routes/web.php', line: 24, func: 'Illuminate\\Support\\Facades\\Facade::__callStatic' },
  { file: '/app/vendor/laravel/framework/src/CallableDispatcher.php', line: 39, func: 'Illuminate\\Routing\\RouteFileRegistrar->{closure:/app/routes/web.php:19}' },
  { file: '/app/vendor/laravel/framework/src/Route.php', line: 254, func: 'Illuminate\\Routing\\CallableDispatcher->dispatch' },
  { file: '/app/web/core/lib/Kernel.php', line: 9, func: 'Drupal\\Core\\Kernel->handle', pkg: 'drupal/core' }
];

describe('traceRows', () => {
  it('names each line by the function it sits in, the next frame down', () => {
    const rows = traceRows(trace);
    expect(rows[1].inside).toBe(trace[2].func);
    expect(rows[1].called).toBe(trace[1].func);
    expect(rows[4].inside).toBe('');
  });
  it('tells app frames from package frames, a package outside vendor/ included', () => {
    expect(traceRows(trace).map((r) => r.app)).toEqual([false, true, false, false, false]);
    expect(packageOf(trace[4])).toBe('drupal/core');
    expect(packageOf(trace[0])).toBe('laravel/framework');
  });
});

describe('traceRuns', () => {
  it('folds consecutive vendor frames and keeps app frames apart', () => {
    expect(traceRuns(traceRows(trace)).map((r) => [r.app, r.rows.length])).toEqual([[false, 1], [true, 1], [false, 3]]);
  });
});

describe('funcLabel', () => {
  it('reads methods, closures and functions the way they were written', () => {
    expect(funcLabel('Illuminate\\Routing\\Route->runCallable')).toBe('Route->runCallable()');
    expect(funcLabel('Illuminate\\Support\\Facades\\Facade::__callStatic')).toBe('Facade::__callStatic()');
    expect(funcLabel('Illuminate\\Pipeline\\Pipeline->{closure:{closure:Illuminate\\Pipeline\\Pipeline::carry():194}:195}')).toBe('closure in Pipeline::carry()');
    expect(funcLabel('Illuminate\\Routing\\RouteFileRegistrar->{closure:/app/routes/web.php:19}')).toBe('closure in web.php:19');
    expect(funcLabel('array_map')).toBe('array_map()');
    expect(funcLabel('')).toBe('{main}');
  });
});

describe('traceText', () => {
  it('writes the trace the way PHP prints one', () => {
    expect(traceText(trace.slice(0, 2))).toBe('#0 /app/vendor/laravel/framework/src/Facade.php(364): Illuminate\\Cache\\CacheManager->__call()\n#1 /app/routes/web.php(24): Illuminate\\Support\\Facades\\Facade::__callStatic()');
  });
});
