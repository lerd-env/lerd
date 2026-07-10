import '@testing-library/jest-dom/vitest';

// jsdom has no Web Animations API, which svelte's animate:flip calls on every
// reordered element. Without it any test that reorders a keyed list throws.
if (typeof Element !== 'undefined' && !Element.prototype.getAnimations) {
  Element.prototype.getAnimations = () => [];
}
