import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import CallerSource from './CallerSource.svelte';

vi.mock('$lib/sourceCode', async (orig) => ({ ...(await orig<object>()), loadSource: () => Promise.reject(new Error('none')) }));

const trace = [
  { file: '/srv/app/vendor/laravel/framework/src/Cache/Repository.php', line: 120, func: 'Illuminate\\Cache\\Repository->get' },
  { file: '/srv/app/app/Http/Controllers/CartController.php', line: 31, func: 'Illuminate\\Cache\\CacheManager->__call' },
  { file: '/srv/app/vendor/laravel/framework/src/Routing/Controller.php', line: 54, func: 'App\\Http\\Controllers\\CartController->show' }
];

describe('CallerSource', () => {
  it('opens the call path with the app line named by the method it sits in', async () => {
    render(CallerSource, { props: { file: trace[1].file, line: 31, trace } });
    await fireEvent.click(screen.getByRole('button', { name: 'Show stack trace' }));
    expect(screen.getByRole('option', { selected: true })).toHaveTextContent('CartController->show()');
    expect(screen.getAllByText('1 vendor frame')).toHaveLength(2);
  });

  it('offers no trace for a line with nothing behind it', () => {
    render(CallerSource, { props: { file: trace[1].file, line: 31, trace: trace.slice(1, 2) } });
    expect(screen.queryByRole('button', { name: 'Show stack trace' })).not.toBeInTheDocument();
  });
});
