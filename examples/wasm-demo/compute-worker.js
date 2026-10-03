/* Heavy Go exports run here; the DOM thread owns controls and rendering. */
"use strict";
importScripts("wasm_exec.js", "runtime.js");

let requestError = null;
const runtime = WasmRuntime.create({
  onError: (message) => {
    requestError = message;
  },
  onTerminal: (message) => postMessage({ terminal: message }),
});
const boot = (async () => {
  const go = new Go();
  const response = await fetch("qmc.wasm");
  if (!response.ok) throw new Error(`fetch qmc.wasm: ${response.status}`);
  const { instance } = await WebAssembly.instantiate(
    await response.arrayBuffer(),
    go.importObject,
  );
  runtime.start(go, instance);
  // A tagged Go test fixture may yield before publishing its namespace.
  await new Promise((resolve) => setTimeout(resolve, 0));
  if (!globalThis.qmc)
    throw new Error("the worker did not publish its Go exports");
  postMessage({ ready: true });
})();

onmessage = async ({ data: { id, name, opts } }) => {
  try {
    await boot;
    requestError = null;
    const result = runtime.call(name, opts);
    const buffers = [
      ...new Set(
        Object.values(result || {})
          .filter(ArrayBuffer.isView)
          .map((view) => view.buffer),
      ),
    ];
    postMessage({ id, result, error: requestError }, buffers);
  } catch (error) {
    postMessage({
      terminal: `The computation worker could not load: ${error.message || error}.`,
    });
  }
};
