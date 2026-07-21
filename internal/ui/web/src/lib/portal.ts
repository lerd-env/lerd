// Moves a node to <body>. Fixed-position layers are still confined by an
// ancestor that creates a stacking context (the nav rail's z-index), which is
// what buried popovers under the dashboard overlay.
export function portal(node: HTMLElement) {
  document.body.appendChild(node);
  return {
    destroy() {
      node.remove();
    }
  };
}
