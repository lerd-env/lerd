// Moves a node to <body>, or to the layer set with setLayerRoot. Fixed-position
// layers are still confined by an ancestor that creates a stacking context (the
// nav rail's z-index), which is what buried popovers under the dashboard
// overlay. The debug bar sets its own layer, so its popovers stay inside the
// shadow root it lives in.
let layer: HTMLElement | null = null;

export function setLayerRoot(el: HTMLElement | null) {
  layer = el;
}

export function layerRoot(): HTMLElement {
  return layer ?? document.body;
}

export function portal(node: HTMLElement) {
  layerRoot().appendChild(node);
  return {
    destroy() {
      node.remove();
    }
  };
}
