import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import vm from "node:vm";

const demo = new URL("../examples/wasm-demo/", import.meta.url);
const code = await readFile(new URL("runtime.js", demo), "utf8");

async function loadCase({
  reader = true,
  streaming = true,
  reduced = false,
  length = 4,
  failure,
} = {}) {
  let reads = 0;
  const progress = [],
    calls = [];
  const bytes = new Uint8Array([0, 97, 115, 109]);
  const response = {
    ok: failure !== "http",
    status: 503,
    headers: { get: () => String(length) },
    body: reader
      ? {
          getReader: () => ({
            read: async () => {
              if (failure === "read") throw new Error("read failed");
              return reads++ < 2
                ? {
                    done: false,
                    value: bytes.slice((reads - 1) * 2, reads * 2),
                  }
                : { done: true };
            },
          }),
        }
      : null,
    arrayBuffer: async () => bytes.buffer,
  };
  const instantiate = async (value, imports) => {
    calls.push("bytes");
    assert.deepEqual(
      Array.from(new Uint8Array(value.buffer || value)),
      Array.from(bytes),
    );
    assert.equal(imports.token, "imports");
    if (failure === "instantiate") throw new Error("instantiate failed");
    return { instance: "instance" };
  };
  const webAssembly = { instantiate };
  if (streaming)
    webAssembly.instantiateStreaming = async (value, imports) => {
      calls.push("streaming");
      assert.equal(value, response);
      assert.equal(imports.token, "imports");
      if (failure === "instantiate") throw new Error("instantiate failed");
      return { instance: "instance" };
    };
  const context = vm.createContext({
    Uint8Array,
    WebAssembly: webAssembly,
    Go: class {
      importObject = { token: "imports" };
    },
    fetch: async (url) => {
      assert.equal(url, "qmc.wasm");
      if (failure === "fetch") throw new Error("fetch failed");
      return response;
    },
  });
  vm.runInContext(code, context);
  const operation = context.WasmRuntime.load(
    (value) => progress.push(value),
    reduced,
  );
  if (failure) {
    await assert.rejects(
      operation,
      failure === "http" ? /503/ : new RegExp(failure + " failed"),
    );
    return;
  }
  const result = await operation;
  assert.equal(result.result.instance, "instance");
  assert.deepEqual(calls, [
    (!reader || reduced) && streaming ? "streaming" : "bytes",
  ]);
  assert.deepEqual(
    progress,
    reader && !reduced && length ? [0.5, 0.98, 1] : [1],
  );
  assert.equal(
    webAssembly.instantiateStreaming === undefined,
    !streaming,
    "loader changed browser globals",
  );
}

for (const opts of [
  {},
  { length: 0 },
  { reader: false },
  { reduced: true },
  { streaming: false },
  { reader: false, streaming: false },
  { reduced: true, streaming: false },
  ...["fetch", "http", "read", "instantiate"].map((failure) => ({ failure })),
  { reader: false, failure: "instantiate" },
])
  await loadCase(opts);

for (const [page, controller] of [
  ["index.html", "app.js"],
  ["analysis.html", "analysis.js"],
]) {
  const html = await readFile(new URL(page, demo), "utf8");
  const js = await readFile(new URL(controller, demo), "utf8");
  const ids = new Set(
    Array.from(html.matchAll(/id="([^"]+)"/g), (match) => match[1]),
  );
  const contract = html.split("DOM CONTRACT")[1].split("-->")[0];
  const described = new Set(
    Array.from(
      contract.matchAll(/#([A-Za-z][A-Za-z0-9]*)/g),
      (match) => match[1],
    ),
  );
  assert.deepEqual(
    Array.from(ids).sort(),
    Array.from(described).sort(),
    page + " DOM contract drift",
  );
  for (const match of js.matchAll(/\bel\("([^"]+)"\)/g))
    assert(ids.has(match[1]), controller + " selector missing " + match[1]);
  assert(
    js.includes("WasmRuntime.load("),
    controller + " bypasses shared loader",
  );
}
console.log(
  "Shared loader and DOM contracts passed: progress, fallbacks, fetch/read/instantiate errors, exact IDs, and controller selectors.",
);
