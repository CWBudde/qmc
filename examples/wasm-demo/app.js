/*
 * app.js — the Point Lab controller.
 *
 * Its only jobs are: collect control values, ask Go for points, and draw what
 * comes back. It never computes a coordinate, a radical inverse, or a digit
 * expansion. If you find yourself about to write `while (i > 0) { d.push(i %
 * base); i = (i / base) | 0; }` in this file, that is the signal to add an
 * export in the Go bridge instead — a demo that reimplements the library in
 * JavaScript is demonstrating the JavaScript.
 *
 * No modules; this file is an IIFE and depends only on window.Render.
 */
(function () {
  "use strict";

  // --- DOM ---------------------------------------------------------------

  const el = (id) => document.getElementById(id);

  const rack = el("rack");
  const statusEl = el("status");
  const bootRing = el("bootRing");
  const liveRegion = el("liveRegion");
  const buildInfo = el("buildInfo");

  const haltonCanvas = el("haltonCanvas");
  const randomCanvas = el("randomCanvas");
  const haltonMeta = el("haltonMeta");
  const randomMeta = el("randomMeta");

  const playButton = el("play");
  const scrub = el("scrub");
  const revealReadout = el("revealReadout");

  const dimsInput = el("dims");
  const dimsOut = el("dimsOut");
  const countInput = el("count");
  const countOut = el("countOut");
  const skipInput = el("skip");
  const skipOut = el("skipOut");
  const axisXSelect = el("axisX");
  const axisYSelect = el("axisY");
  const seedInput = el("seed");
  const newSeedButton = el("newSeed");
  const resetButton = el("resetView");
  const scrambleButton = el("scramble");
  const scrambleState = el("scrambleState");
  const moneyNote = el("moneyNote");

  const telemetry = {
    baseX: el("tBaseX"),
    baseY: el("tBaseY"),
    points: el("tPoints"),
    skip: el("tSkip"),
    seed: el("tSeed"),
    scramble: el("tScramble"),
  };

  const digitIndexInput = el("digitIndex");
  const digitDimSelect = el("digitDim");
  const digitPrev = el("digitPrev");
  const digitNext = el("digitNext");
  const mirror = el("mirror");
  const dBase = el("dBase");
  const dRawIndex = el("dRawIndex");
  const dValue = el("dValue");
  const dPlainValue = el("dPlainValue");
  const permNote = el("permNote");

  const reducedMotion =
    window.matchMedia &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  const LIVE_THROTTLE_MS = 700;

  // --- state -------------------------------------------------------------

  const state = {
    ready: false,
    dead: false,
    info: null,
    halton: null,
    random: null,
    reveal: 0,
    playing: false,
    lastTick: 0,
    selected: -1,
    lastAnnounce: 0,
    pending: false,
  };

  // Reusable views over JS-owned ArrayBuffers, one set per point cloud so the
  // two calls never write into each other's memory.
  const sinks = { halton: {}, random: {} };

  function setStatus(message, tone) {
    statusEl.textContent = message;
    statusEl.dataset.state = tone || "";
  }

  // The scatter redraws on every slider tick; an unthrottled live region would
  // read out hundreds of updates a second and drown the page in speech.
  function announce(message) {
    const now = Date.now();

    if (now - state.lastAnnounce < LIVE_THROTTLE_MS) {
      return;
    }

    state.lastAnnounce = now;
    liveRegion.textContent = message;
  }

  // --- the wasm call wrapper ---------------------------------------------

  // Every call into Go goes through here. A missing export, a thrown error and
  // an {error} result all become the same thing: a message on the status line
  // and a null return. Nothing from the wasm side is ever allowed to throw into
  // the render loop, because a half-drawn frame is much harder to diagnose than
  // a status line that says what went wrong.
  function call(name, opts, callOpts) {
    const silent = callOpts && callOpts.silent;

    // A panic aborts the whole wasm instance. Once one has been reported the
    // module is rubble, and calling into it again produces noise rather than
    // information, so the gate stays shut until the page is reloaded.
    if (state.dead) {
      return null;
    }

    const api = globalThis.qmc;

    if (!api || typeof api[name] !== "function") {
      if (!silent) {
        setStatus(`export "${name}" is unavailable`, "error");
      }

      return null;
    }

    let result;

    try {
      result = api[name](opts);
    } catch (err) {
      console.error(err);

      if (!silent) {
        setStatus(`${name} failed: ${err && err.message}`, "error");
      }

      return null;
    }

    if (result && result.error) {
      console.error(result.error);

      if (result.panic) {
        state.dead = true;
        setStatus(
          `${name} panicked: ${result.error} — the WebAssembly instance is dead. Reload the page.`,
          "error",
        );

        return null;
      }

      if (!silent) {
        setStatus(result.error, "error");
      }

      return null;
    }

    return result;
  }

  // cacheSinks remembers the views Go handed back so the next call can reuse
  // their buffers. A returned view may be a subarray of the buffer, so the
  // whole ArrayBuffer is re-wrapped rather than the view stored directly —
  // caching `result.xy` itself would hand Go a short window into a long buffer
  // and the next, larger request would look like it needed a reallocation.
  function cacheSinks(store, result, keys) {
    for (const key of keys) {
      const view = result[key];

      if (view && view.buffer) {
        store[key] = {
          f32: new Float32Array(view.buffer),
          u8: new Uint8Array(view.buffer),
        };
      }
    }
  }

  // --- control readers ---------------------------------------------------

  function intValue(input, fallback) {
    const value = parseInt(input.value, 10);

    return Number.isFinite(value) ? value : fallback;
  }

  function scrambleOn() {
    return scrambleButton.getAttribute("aria-pressed") === "true";
  }

  function baseRequest() {
    return {
      dims: intValue(dimsInput, 39),
      count: intValue(countInput, 600),
      skip: intValue(skipInput, 64),
      scramble: scrambleOn(),
      seed: intValue(seedInput, 1),
      axisX: intValue(axisXSelect, 0),
      axisY: intValue(axisYSelect, 1),
    };
  }

  // --- controls built from info() ----------------------------------------

  function fillDimensionSelect(select, dims, preferred) {
    const wanted = preferred === undefined ? intValue(select, 0) : preferred;

    select.innerHTML = "";

    for (let i = 0; i < dims; i += 1) {
      const option = document.createElement("option");
      option.value = String(i);
      option.textContent = `dimension ${i}`;
      select.append(option);
    }

    // Shrinking the sequence can strand a selected axis past the last
    // dimension. Clamp rather than silently reset to zero, so nudging
    // dimensions down and back up lands where it started.
    select.value = String(Math.max(0, Math.min(dims - 1, wanted)));
  }

  function rebuildDimensionSelects() {
    const dims = intValue(dimsInput, 39);

    fillDimensionSelect(axisXSelect, dims);
    fillDimensionSelect(axisYSelect, dims);
    fillDimensionSelect(digitDimSelect, dims);
  }

  // The <select>s in the HTML ship empty and every slider range is overwritten
  // here. Limits belong to the Go side's capability table, so raising maxPoints
  // in the library raises it in the UI without anyone editing markup.
  function populateControls() {
    const info = state.info;
    const defaults = info.defaults || {};

    dimsInput.min = "2";
    dimsInput.max = String(info.maxDims);
    countInput.min = "1";
    countInput.max = String(info.maxPoints);
    skipInput.min = "0";
    skipInput.max = String(info.maxSkip);
    digitIndexInput.min = "0";
    digitIndexInput.max = String(info.maxIndex);

    dimsInput.value = String(defaults.dims);
    countInput.value = String(defaults.count);
    skipInput.value = String(defaults.skip);
    seedInput.value = String(defaults.seed);

    fillDimensionSelect(axisXSelect, defaults.dims, defaults.axisX);
    fillDimensionSelect(axisYSelect, defaults.dims, defaults.axisY);
    fillDimensionSelect(digitDimSelect, defaults.dims, defaults.axisX);

    // Scrambling starts OFF on purpose. The default view is the defect the
    // library's README documents; the toggle is the demonstration.
    setScramble(false);
    syncOutputs();

    buildInfo.textContent = `${info.goVersion} · ${info.goos}/${info.goarch}`;
  }

  function syncOutputs() {
    dimsOut.textContent = dimsInput.value;
    countOut.textContent = Number(countInput.value).toLocaleString("en-US");
    skipOut.textContent = skipInput.value;
  }

  function setScramble(on) {
    scrambleButton.setAttribute("aria-pressed", String(on));
    scrambleState.textContent = on ? "on" : "off";

    moneyNote.innerHTML = on
      ? "<b>On.</b> Each digit position of each dimension is rewritten by an independent uniform permutation of its digit alphabet. The elementary-interval structure survives — a digit permutation maps each interval onto another of the same size — but the ramps are gone, and the diagonal with them."
      : "<b>Off.</b> On dimensions 37 and 38 of a 39-dimensional sequence the bases are 163 and 167, so with only a few hundred points both coordinates are still walking up their ramp in lockstep. That is the diagonal on the left. Press this switch.";
  }

  // --- drawing -----------------------------------------------------------

  function scheduleRefresh() {
    if (state.pending || !state.ready) {
      return;
    }

    state.pending = true;
    requestAnimationFrame(() => {
      state.pending = false;
      refresh();
    });
  }

  function refresh() {
    if (!state.ready) {
      return;
    }

    const request = baseRequest();

    const halton = call(
      "points",
      Object.assign({}, request, { source: "halton", out: sinks.halton }),
    );

    if (!halton) {
      return;
    }

    cacheSinks(sinks.halton, halton, ["xy"]);
    state.halton = halton;

    const random = call(
      "points",
      Object.assign({}, request, { source: "random", out: sinks.random }),
    );

    if (random) {
      cacheSinks(sinks.random, random, ["xy"]);
    }

    state.random = random;

    const count = halton.count;

    scrub.max = String(count);
    scrub.disabled = false;
    playButton.disabled = false;

    if (!state.playing) {
      setReveal(count);
    } else {
      setReveal(Math.min(state.reveal, count));
    }

    if (state.selected >= count) {
      state.selected = -1;
    }

    updateTelemetry(halton, random);
    draw();
    refreshDigits();

    setStatus(
      `${count.toLocaleString("en-US")} points · ${halton.dims} dims · axes ${halton.axisX}×${halton.axisY} · scrambling ${request.scramble ? "on" : "off"}`,
      "ready",
    );
    announce(
      `${count} points, dimensions ${halton.axisX} and ${halton.axisY}, scrambling ${request.scramble ? "on" : "off"}.`,
    );
  }

  function updateTelemetry(halton, random) {
    telemetry.baseX.textContent =
      halton.baseX === null || halton.baseX === undefined
        ? "—"
        : String(halton.baseX);
    telemetry.baseY.textContent =
      halton.baseY === null || halton.baseY === undefined
        ? "—"
        : String(halton.baseY);
    telemetry.points.textContent = halton.count.toLocaleString("en-US");
    telemetry.skip.textContent = String(halton.skip);
    telemetry.seed.textContent = String(halton.seed);
    telemetry.scramble.textContent = halton.scramble ? "on" : "off";
    telemetry.scramble.dataset.tone = halton.scramble ? "good" : "";

    haltonMeta.textContent =
      halton.baseX && halton.baseY
        ? `base ${halton.baseX} × base ${halton.baseY}`
        : `dims ${halton.axisX} × ${halton.axisY}`;
    randomMeta.textContent = random
      ? `uniform × uniform · seed ${random.seed}`
      : "unavailable";
  }

  function draw() {
    const halton = state.halton;
    const random = state.random;
    const axisX = intValue(axisXSelect, 0);
    const axisY = intValue(axisYSelect, 1);

    Render.drawScatter(haltonCanvas, {
      xy: halton && halton.xy,
      count: halton ? halton.count : 0,
      reveal: state.reveal,
      color: Render.readVar("--halton", "#46e0c8"),
      glyph: "circle",
      selected: state.selected,
      xLabel: `dim ${axisX}`,
      yLabel: `dim ${axisY}`,
    });

    Render.drawScatter(randomCanvas, {
      xy: random && random.xy,
      count: random ? random.count : 0,
      reveal: state.reveal,
      color: Render.readVar("--random", "#ffb04a"),
      glyph: "cross",
      selected: state.selected,
      xLabel: `dim ${axisX}`,
      yLabel: `dim ${axisY}`,
    });
  }

  // --- reveal transport --------------------------------------------------

  function setReveal(value) {
    const total = state.halton ? state.halton.count : 0;
    const clamped = Math.max(0, Math.min(total, Math.round(value)));

    state.reveal = clamped;
    scrub.value = String(clamped);
    revealReadout.textContent = `${clamped.toLocaleString("en-US")} / ${total.toLocaleString("en-US")}`;
  }

  function setPlaying(playing) {
    state.playing = playing;
    playButton.setAttribute("aria-pressed", String(playing));
    playButton.textContent = playing ? "Pause" : "Play";
    state.lastTick = performance.now();
  }

  // The reveal advances on wall-clock time rather than once per animation
  // frame, so it takes the same three seconds on a 60 Hz and a 144 Hz display
  // whatever the point count.
  const REVEAL_SECONDS = 3;

  function tick(now) {
    if (state.playing && state.halton) {
      const total = state.halton.count;
      const elapsed = Math.max(0, now - state.lastTick);
      state.lastTick = now;

      const next = state.reveal + (elapsed / 1000) * (total / REVEAL_SECONDS);

      if (next >= total) {
        setReveal(total);
        setPlaying(false);
      } else {
        setReveal(next);
      }

      draw();
    }

    requestAnimationFrame(tick);
  }

  // --- digit inspector ---------------------------------------------------

  function refreshDigits() {
    if (!state.ready) {
      return;
    }

    const result = call("digits", {
      index: intValue(digitIndexInput, 0),
      dim: intValue(digitDimSelect, 0),
      skip: intValue(skipInput, 64),
      scramble: scrambleOn(),
      seed: intValue(seedInput, 1),
    });

    if (!result) {
      return;
    }

    renderMirror(result);
  }

  function digitCell(position, raw, permuted, label) {
    const cell = document.createElement("span");
    cell.className = "digit";
    cell.dataset.position = String(position);

    const pos = document.createElement("span");
    pos.className = "digit__pos";
    pos.textContent = label;
    cell.append(pos);

    const value = document.createElement("span");
    value.className = "digit__raw";
    value.textContent = String(raw);
    cell.append(value);

    if (permuted !== null && permuted !== undefined) {
      const perm = document.createElement("span");
      perm.className = "digit__perm";
      perm.textContent = String(permuted);
      cell.append(perm);
    }

    return cell;
  }

  function renderMirror(d) {
    const digits = Array.from(d.digits || []);
    const permuted = d.permuted ? Array.from(d.permuted) : null;

    mirror.innerHTML = "";

    if (!digits.length) {
      const empty = document.createElement("p");
      empty.className = "mirror-empty";
      empty.textContent = "no digits for this index";
      mirror.append(empty);
    } else {
      // `digits` arrives least-significant-first, which is the order the
      // FRACTION wants: value = d₀/p + d₁/p² + …  The INTEGER side is the same
      // list read the other way round, so it is reversed here deliberately.
      // Reversing a copy, not the array itself, keeps the fraction side intact.
      const integerDigits = digits.slice().reverse();

      const intSide = document.createElement("span");
      intSide.className = "mirror__side mirror__side--int";

      for (let k = 0; k < integerDigits.length; k += 1) {
        // Leftmost cell is the highest power. Its mirror position — the index
        // into the least-significant-first array — counts the other way.
        const position = integerDigits.length - 1 - k;
        intSide.append(
          digitCell(position, integerDigits[k], null, `p${sup(position)}`),
        );
      }

      const point = document.createElement("span");
      point.className = "mirror__point";
      point.textContent = ".";
      point.title = "radix point";

      const fracSide = document.createElement("span");
      fracSide.className = "mirror__side mirror__side--frac";

      for (let k = 0; k < digits.length; k += 1) {
        fracSide.append(
          digitCell(
            k,
            digits[k],
            permuted ? permuted[k] : null,
            `p${sup(-(k + 1))}`,
          ),
        );
      }

      mirror.append(intSide, point, fracSide);
      wireTwins();
    }

    dBase.textContent = String(d.base);
    dRawIndex.textContent = `${d.rawIndex} (index ${d.index} + skip)`;
    dValue.textContent = formatCoordinate(d.value);
    dPlainValue.textContent = formatCoordinate(d.unscrambledValue);

    permNote.innerHTML = permutationNote(d);
  }

  // sup renders a signed exponent in superscript digits so an axis of powers
  // reads as p⁻¹ rather than p^-1.
  function sup(exponent) {
    const glyphs = "⁰¹²³⁴⁵⁶⁷⁸⁹";
    const body = String(Math.abs(exponent))
      .split("")
      .map((c) => glyphs[Number(c)])
      .join("");

    return (exponent < 0 ? "⁻" : "") + body;
  }

  function formatCoordinate(value) {
    if (value === null || value === undefined || !isFinite(value)) {
      return "—";
    }

    return value.toFixed(10);
  }

  function permutationNote(d) {
    if (!d.permutation) {
      return "<b>Scrambling is off.</b> The digits are read straight off the index, so the coordinate and the unscrambled coordinate are the same number.";
    }

    const perm = Array.from(d.permutation);
    const preview =
      perm.length <= 24
        ? ` For this base it is π = [${perm.join(", ")}].`
        : ` It is a permutation of ${perm.length} symbols; only the digits that occur are shown.`;

    return `<b>Scrambling is on.</b> One permutation π of {0…${d.base - 1}} is drawn per dimension and applied to <i>every</i> digit position of dimension ${d.dim} — the violet row.${preview}`;
  }

  // Hovering a digit lights the cell at the same power on the other side of
  // the radix point. That pairing is the radical inverse, and it is easier to
  // feel than to read.
  function wireTwins() {
    const cells = mirror.querySelectorAll(".digit");

    const setTwin = (position, on) => {
      for (const cell of cells) {
        if (cell.dataset.position === String(position)) {
          cell.dataset.twin = on ? "on" : "";
        }
      }
    };

    for (const cell of cells) {
      cell.addEventListener("mouseenter", () =>
        setTwin(cell.dataset.position, true),
      );
      cell.addEventListener("mouseleave", () =>
        setTwin(cell.dataset.position, false),
      );
    }
  }

  // --- point picking -----------------------------------------------------

  function wirePicking(canvas, which) {
    canvas.addEventListener("click", (event) => {
      const result = which === "halton" ? state.halton : state.random;

      if (!result || !result.xy) {
        return;
      }

      const rect = canvas.getBoundingClientRect();
      const geo = Render.scatterGeometry(canvas);
      const index = Render.nearestPoint(
        geo,
        result.xy,
        state.reveal,
        event.clientX - rect.left,
        event.clientY - rect.top,
      );

      if (index < 0) {
        return;
      }

      state.selected = index;
      digitIndexInput.value = String(index);
      refreshDigits();
      draw();
      announce(`Point ${index} selected.`);
    });
  }

  // --- wiring ------------------------------------------------------------

  function wireControls() {
    for (const input of [dimsInput, countInput, skipInput]) {
      input.addEventListener("input", () => {
        syncOutputs();

        if (input === dimsInput) {
          rebuildDimensionSelects();
        }

        scheduleRefresh();
      });
    }

    for (const control of [axisXSelect, axisYSelect, seedInput]) {
      control.addEventListener("change", scheduleRefresh);
    }

    scrambleButton.addEventListener("click", () => {
      setScramble(!scrambleOn());
      scheduleRefresh();
    });

    newSeedButton.addEventListener("click", () => {
      seedInput.value = String(Math.floor(Math.random() * 100000) + 1);
      scheduleRefresh();
    });

    resetButton.addEventListener("click", () => {
      populateControls();
      state.selected = -1;
      digitIndexInput.value = "0";
      setPlaying(false);
      scheduleRefresh();
    });

    playButton.addEventListener("click", () => {
      if (!state.halton) {
        return;
      }

      if (!state.playing && state.reveal >= state.halton.count) {
        setReveal(0);
        draw();
      }

      setPlaying(!state.playing);
    });

    scrub.addEventListener("input", () => {
      setPlaying(false);
      setReveal(parseInt(scrub.value, 10) || 0);
      draw();
    });

    digitIndexInput.addEventListener("change", () => {
      state.selected = intValue(digitIndexInput, 0);
      refreshDigits();
      draw();
    });

    digitDimSelect.addEventListener("change", refreshDigits);

    digitPrev.addEventListener("click", () => stepDigitIndex(-1));
    digitNext.addEventListener("click", () => stepDigitIndex(1));

    wirePicking(haltonCanvas, "halton");
    wirePicking(randomCanvas, "random");

    let resizeTimer = null;

    window.addEventListener("resize", () => {
      window.clearTimeout(resizeTimer);
      resizeTimer = window.setTimeout(draw, 120);
    });

    Render.watchDPR(() => {
      Render.invalidateTheme();
      draw();
    });
  }

  function stepDigitIndex(delta) {
    const max = state.info ? state.info.maxIndex : 1000000;
    const next = Math.max(
      0,
      Math.min(max, intValue(digitIndexInput, 0) + delta),
    );

    digitIndexInput.value = String(next);
    state.selected = next;
    refreshDigits();
    draw();
  }

  // --- boot --------------------------------------------------------------

  // Streamed rather than instantiateStreaming'd so the boot ring can show real
  // progress. The streaming path is kept as the fallback for a response with
  // no body reader, and taken outright under prefers-reduced-motion, where an
  // animated progress ring is exactly what the user asked not to see.
  async function loadWasmWithProgress(onProgress) {
    if (!WebAssembly.instantiateStreaming) {
      WebAssembly.instantiateStreaming = async (resp, importObject) => {
        const source = await (await resp).arrayBuffer();

        return WebAssembly.instantiate(source, importObject);
      };
    }

    const go = new Go();
    const response = await fetch("qmc.wasm");

    if (!response.ok) {
      throw new Error(`fetch qmc.wasm: ${response.status}`);
    }

    if (!response.body || !response.body.getReader || reducedMotion) {
      onProgress(1);

      return {
        go,
        result: await WebAssembly.instantiateStreaming(
          response,
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

  async function initWasm() {
    setStatus("Loading WebAssembly…", "loading");

    const { go, result } = await loadWasmWithProgress((progress) => {
      Render.ring(bootRing, progress);
    });

    // Deliberately not awaited: the demo's main() ends in select{} so this
    // promise never resolves. Awaiting it would hang the page forever.
    go.run(result.instance);

    // Give the Go side one turn of the event loop to publish globalThis.qmc.
    await new Promise((resolve) => setTimeout(resolve, 0));

    const info = call("info", undefined);

    if (!info) {
      throw new Error("the wasm module did not publish its capability table");
    }

    state.info = info;
    state.ready = true;

    bootRing.dataset.state = "ready";
    rack.dataset.boot = "ready";

    populateControls();
    wireControls();

    scrambleButton.disabled = false;
    newSeedButton.disabled = false;
    resetButton.disabled = false;
    digitPrev.disabled = false;
    digitNext.disabled = false;

    setStatus("WASM ready", "ready");
    requestAnimationFrame(tick);
    refresh();
  }

  initWasm().catch((err) => {
    console.error(err);
    setStatus(
      "WebAssembly failed to load. Serve this page over HTTP — a file:// URL cannot fetch a .wasm — and check that qmc.wasm is sent with Content-Type: application/wasm.",
      "error",
    );
  });
})();
