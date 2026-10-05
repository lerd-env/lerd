import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import TraceBlock from './TraceBlock.svelte';

vi.mock('$lib/editor', async (orig) => ({ ...(await orig<object>()), openInEditor: vi.fn(async () => {}) }));

describe('TraceBlock', () => {
  beforeEach(() => {
    Object.assign(navigator, { clipboard: { writeText: vi.fn(async () => {}) } });
  });

  it('leads with the first application frame, not the innermost vendor one', () => {
    render(TraceBlock, {
      props: {
        trace: [
          { func: 'Builder::get', file: '/home/u/shop/vendor/laravel/Builder.php', line: 100 },
          { func: 'UserController::index', file: '/home/u/shop/app/UserController.php', line: 18 }
        ]
      }
    });
    expect(screen.getAllByText('/home/u/shop/app/UserController.php:18').length).toBeGreaterThan(0);
  });

  it('falls back to the event source when no trace was captured', () => {
    render(TraceBlock, { props: { src: { file: '/home/u/shop/routes/web.php', line: 9 } } });
    expect(screen.getByText('/home/u/shop/routes/web.php:9')).toBeInTheDocument();
  });

  it('offers a copy button for the primary frame', () => {
    render(TraceBlock, { props: { src: { file: '/home/u/shop/routes/web.php', line: 9 } } });
    expect(screen.getByLabelText('Copy path')).toBeInTheDocument();
  });

  it('hides the frame list until the details toggle is pressed', async () => {
    render(TraceBlock, {
      props: {
        trace: [
          { func: 'a', file: '/home/u/shop/app/A.php', line: 1 },
          { func: 'b', file: '/home/u/shop/vendor/b/B.php', line: 2 }
        ]
      }
    });
    expect(screen.queryByText('/home/u/shop/vendor/b/B.php:2')).toBeNull();
    screen.getByText('Details').click();
    await vi.waitFor(() =>
      expect(screen.getByText('/home/u/shop/vendor/b/B.php:2')).toBeInTheDocument()
    );
  });
});
