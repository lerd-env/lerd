import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import CleanupModal from './CleanupModal.svelte';
import type { DiskImage } from '$stores/disk';

function img(over: Partial<DiskImage> = {}): DiskImage {
  return { id: 'docker.io/library/mysql:8.0', desc: 'unused service image', owner: 'lerd', bytes: 100, ...over };
}

function props(images: DiskImage[]) {
  return {
    open: true,
    images,
    reclaimableBytes: images.reduce((n, i) => n + i.bytes, 0),
    onconfirm: () => {},
    onclose: () => {}
  };
}

describe('CleanupModal', () => {
  // The whole point of the owner split: a user deciding whether to press the
  // button needs to see what is lerd's own mess and what is their own.
  it('groups the images by owner with a count each', () => {
    const { getByText } = render(CleanupModal, {
      props: props([
        img(),
        img({ id: 'docker.io/library/golang:1.25', desc: 'unused image', owner: 'other', bytes: 900 }),
        img({ id: 'docker.io/library/php:8.4-cli', desc: 'unused image', owner: 'other', bytes: 500 })
      ])
    });
    expect(getByText('lerd images (1)')).toBeTruthy();
    expect(getByText('Other images (2)')).toBeTruthy();
  });

  it('omits a group with nothing in it', () => {
    const { queryByText } = render(CleanupModal, { props: props([img()]) });
    expect(queryByText('Other images (0)')).toBeNull();
  });

  // Two stranded build bases both read "unused image", so the ref is the only
  // thing that tells the user which image is about to go.
  it('lists each image by its ref, not only its description', () => {
    const { getByText } = render(CleanupModal, {
      props: props([img({ id: 'docker.io/library/golang:1.25', desc: 'unused image', owner: 'other' })])
    });
    expect(getByText('docker.io/library/golang:1.25')).toBeTruthy();
  });
});
