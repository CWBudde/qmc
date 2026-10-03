/* Shared request/terminal-failure handling for both demo controllers. */
(function () {
  "use strict";

  // Both pages use the same progress reader and non-reader/reduced-motion
  // fallback. Runtime startup and terminal handling remain separate below.
  async function load(onProgress, reducedMotion = false) {
    const go = new Go();
    const response = await fetch("qmc.wasm");

    if (!response.ok) {
      throw new Error(`fetch qmc.wasm: ${response.status}`);
    }

    if (!response.body || !response.body.getReader || reducedMotion) {
      onProgress(1);

      return {
        go,
        result: WebAssembly.instantiateStreaming
          ? await WebAssembly.instantiateStreaming(response, go.importObject)
          : await WebAssembly.instantiate(
              await response.arrayBuffer(),
              go.importObject,
            ),
      };
    }

    const total = Number(response.headers.get("content-length")) || 0;
    const reader = response.body.getReader();
    const chunks = [];
    let received = 0;

    for (;;) {
      const { done, value } = await reader.read();

      if (done) {
        break;
      }

      chunks.push(value);
      received += value.length;

      if (total > 0) {
        onProgress(Math.min(0.98, received / total));
      }
    }

    onProgress(1);

    const bytes = new Uint8Array(received);
    let offset = 0;

    for (const chunk of chunks) {
      bytes.set(chunk, offset);
      offset += chunk.length;
    }

    return {
      go,
      result: await WebAssembly.instantiate(bytes, go.importObject),
    };
  }

  function create({ onError, onTerminal }) {
    let go = null;
    let dead = false;

    function terminate(message) {
      if (dead) return;
      dead = true;
      onTerminal(`${message} Reload WebAssembly to start a fresh instance.`);
    }

    function start(instanceGo, instance) {
      go = instanceGo;
      // main deliberately stays alive. Resolution, rejection, or a trap means
      // that this instance is no longer available, rather than a bad request.
      Promise.resolve(go.run(instance)).then(
        () => terminate("The WebAssembly runtime terminated."),
        (error) => terminate(`WebAssembly failed: ${error.message || error}.`),
      );
    }

    function call(name, opts, { silent = false } = {}) {
      if (dead) return null;
      if (go && go.exited) {
        terminate("The WebAssembly runtime terminated.");
        return null;
      }
      const fn = globalThis.qmc && globalThis.qmc[name];
      if (typeof fn !== "function") {
        if (!silent) onError(`export "${name}" is unavailable`);
        return null;
      }
      try {
        const result = fn(opts);
        if (result && result.error) {
          if (!silent) onError(result.error);
          return null;
        }
        return result;
      } catch (error) {
        if ((go && go.exited) || error instanceof WebAssembly.RuntimeError) {
          terminate(`WebAssembly failed: ${error.message || error}.`);
        } else if (!silent) {
          onError(`${name} failed: ${error.message || error}`);
        }
        return null;
      }
    }

    return { start, call, terminate };
  }

  globalThis.WasmRuntime = { load, create };
})();
