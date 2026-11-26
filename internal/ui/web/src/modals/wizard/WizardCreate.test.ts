import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import WizardCreate from './WizardCreate.svelte';

const frameworkCatalogue = vi.fn();

vi.mock('$stores/wizard', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/wizard')>();
  return { ...actual, frameworkCatalogue: (...a: unknown[]) => frameworkCatalogue(...a) };
});

const props = { parent: '/home/u/code', name: 'acme', framework: 'laravel', frameworkVersion: '', starter: 'filament' };

describe('WizardCreate', () => {
  // Coming back to the step remounts it, so the catalogue is still loading when
  // the name is edited; the starter picked earlier must survive that.
  it('keeps the picked starter while the catalogue is still loading', async () => {
    frameworkCatalogue.mockReturnValue(new Promise(() => {}));
    const onchange = vi.fn();
    const { getByDisplayValue } = render(WizardCreate, { ...props, onchange });

    await fireEvent.input(getByDisplayValue('acme'), { target: { value: 'acme2' } });

    expect(onchange).toHaveBeenLastCalledWith(expect.objectContaining({ name: 'acme2', starter: 'filament' }));
  });

  it('drops a starter the newly picked major does not support', async () => {
    frameworkCatalogue.mockResolvedValue([
      {
        name: 'laravel',
        label: 'Laravel',
        versions: ['13', '12'],
        latest: '13',
        starters: [{ name: 'react', label: 'React', versions: ['13'] }]
      }
    ]);
    const onchange = vi.fn();
    const { getAllByRole, getByRole } = render(WizardCreate, { ...props, starter: 'react', onchange });

    const version = await waitFor(() => {
      const menus = getAllByRole('button', { expanded: false });
      expect(menus.length).toBeGreaterThan(1);
      return menus[1];
    });
    await fireEvent.click(version);
    await fireEvent.click(getByRole('option', { name: '12' }));

    expect(onchange).toHaveBeenLastCalledWith(expect.objectContaining({ frameworkVersion: '12', starter: '' }));
  });
});
