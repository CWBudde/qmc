/* One cancellable worker per independent computation channel. */
(function () {
  "use strict";

  function create({ onError, onTerminal }) {
    let worker = null;
    let pending = null;
    let bootTimer = null;
    let nextId = 0;

    function dispose() {
      if (worker) {
        worker.onmessage = null;
        worker.onerror = null;
        worker.terminate();
      }
      worker = null;
      clearTimeout(bootTimer);
      if (pending) {
        clearTimeout(pending.timer);
        pending.resolve(null);
        pending = null;
      }
    }

    function cancel() {
      if (pending) dispose();
    }

    function fail(message) {
      dispose();
      onTerminal(message);
    }

    function call(name, opts) {
      // Each channel has one outstanding call. Replacing it actually stops
      // the old WASM computation, rather than leaving a stale queue to drain.
      if (pending) cancel();
      return new Promise((resolve) => {
        const id = ++nextId;
        pending = {
          id,
          resolve,
          timer: setTimeout(
            () =>
              fail("The computation worker exceeded its two-minute deadline."),
            120000,
          ),
        };
        try {
          if (!worker) {
            worker = new Worker("compute-worker.js");
            const activeWorker = worker;
            bootTimer = setTimeout(
              () =>
                fail(
                  "The computation worker did not become ready within twenty seconds.",
                ),
              20000,
            );
            worker.onerror = (event) => {
              if (worker !== activeWorker) return;
              event.preventDefault();
              fail(`The computation worker failed: ${event.message}.`);
            };
            worker.onmessage = ({ data }) => {
              if (worker !== activeWorker) return;
              if (data.ready) {
                clearTimeout(bootTimer);
                return;
              }
              if (data.terminal) {
                fail(data.terminal);
                return;
              }
              if (!pending || data.id !== pending.id) return;
              const request = pending;
              pending = null;
              clearTimeout(request.timer);
              if (data.error) onError(data.error);
              request.resolve(data.result || null);
            };
          }
          // Buffers are produced inside the worker and transferred to the UI.
          // A previously displayed buffer must not be detached to reuse it.
          const request = { ...opts };
          delete request.out;
          worker.postMessage({ id, name, opts: request });
        } catch (error) {
          fail(
            `The computation worker could not start: ${error.message || error}.`,
          );
        }
      });
    }

    window.addEventListener("pagehide", dispose);
    return { call, cancel, dispose };
  }

  window.QMCCompute = { create };
})();
