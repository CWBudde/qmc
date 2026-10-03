/* Shared request/terminal-failure handling for both demo controllers. */
(function () {
  "use strict";

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

  window.WasmRuntime = { create };
})();
