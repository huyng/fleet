// Mounts the app for tests.
//
// This file has JSX, so node cannot load it directly: build-test.mjs bundles
// it into test/.build/harness.js first, and the test file imports that.

import { act } from "react";
import { createRoot } from "react-dom/client";

import { App } from "../src/App.jsx";

/**
 * Mount the app into a DOM node, flushing the first render.
 *
 * @param {Element} container Where to mount, usually an empty div.
 * @returns {Promise<{unmount: function}>} A handle that tears the app down.
 */
export async function mount(container) {
  const root = createRoot(container);
  await act(async () => {
    root.render(<App />);
  });

  return {
    unmount: async () => {
      await act(async () => {
        root.unmount();
      });
    },
  };
}

/**
 * Let queued state updates flush, inside act so React can account for them.
 *
 * @param {function} update A function that changes state, such as a click.
 */
export async function interact(update) {
  await act(async () => {
    await update();
  });
}
