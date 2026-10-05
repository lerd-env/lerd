import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import CallerSource from './CallerSource.svelte';

const trace = [
  { file: '/srv/app/vendor/laravel/framework/src/Cache/Repository.php', line: 120, func: 'Illuminate\\Cache\\Repository->get' },
  { file: '/srv/app/app/Http/Controllers/CartController.php', line: 31, func: 'Illuminate\\Cache\\CacheManager->__call' },
  { file: '/srv/app/vendor/laravel/framework/src/Routing/Controller.php', line: 54, func: 'App\\Http\\Controllers\\CartController->show' }
];

describe('CallerSource', () => {
  it('unfolds the stack from the app line down, leaving the library frames above it out', async () => {
    render(CallerSource, { props: { file: trace[1].file, line: 31, trace } });
    expect(screen.queryByText(trace[2].func)).not.toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: 'Show stack trace' }));
    expect(screen.getByText(trace[1].func)).toBeInTheDocument();
    expect(screen.getByText(trace[2].func)).toBeInTheDocument();
    expect(screen.queryByText(trace[0].func)).not.toBeInTheDocument();
  });

  it('offers no trace for a line with nothing behind it', () => {
    render(CallerSource, { props: { file: trace[1].file, line: 31, trace: trace.slice(1, 2) } });
    expect(screen.queryByRole('button', { name: 'Show stack trace' })).not.toBeInTheDocument();
  });
});
