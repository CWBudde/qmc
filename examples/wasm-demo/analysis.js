/*
 * analysis.js — the Discrepancy Bench controller.
 *
 * Two invariants worth stating up front, because both are easy to break:
 *
 *   Stop. A synchronous call into Go blocks the event loop for its whole
 *   duration, so a click on Stop cannot be dispatched while one is running.
 *   The sweep therefore asks Go for exactly one N per call and awaits a
 *   zero-delay timeout between calls. That gap is the entire cancellation
 *   mechanism; remove the yield and Stop stops working.
 *
 *   runId. Every sweep carries a monotonic id, and each step re-checks it
 *   after the yield. A sweep restarted while an older one is mid-flight must
 *   not append its points to the new chart.
 *
 * As on the Point Lab, no quasi-Monte Carlo logic lives here. Every
 * correlation and every integration error arrives from the Go library as a
 * finished number.
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

  const heatmap = el("heatmap");
  const heatLegend = el("heatLegend");
  const cellReadout = el("cellReadout");
  const worstAdjacent = el("worstAdjacent");
  const worstPairLabel = el("worstPairLabel");
  const docReference = el("docReference");
  const corrDims = el("corrDims");
  const corrDimsOut = el("corrDimsOut");
  const corrCount = el("corrCount");
  const corrCountOut = el("corrCountOut");
  const corrSkip = el("corrSkip");
  const corrSkipOut = el("corrSkipOut");
  const corrSeed = el("corrSeed");
  const corrSource = el("corrSource");
  const corrRandom = el("corrRandom");
  const corrNote = el("corrNote");
  const corrBaseNote = el("corrBaseNote");

  const integrandSelect = el("integrand");
  const integrandNote = el("integrandNote");
  const budgetSelect = el("budget");
  const convDims = el("convDims");
  const convDimsOut = el("convDimsOut");
  const convSkip = el("convSkip");
  const convSkipOut = el("convSkipOut");
  const convSeed = el("convSeed");
  const convSource = el("convSource");
  const convRandom = el("convRandom");
  const startButton = el("start");
  const stopButton = el("stop");
  const progressBar = el("progressBar");
  const progressText = el("progressText");
  const convChart = el("convChart");
  const convRows = el("convRows");

  const readout = {
    exact: el("tExact"),
    qmc: el("tQmcError"),
    mc: el("tMcError"),
    ratio: el("tRatio"),
  };

  const reducedMotion =
    window.matchMedia &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  const LIVE_THROTTLE_MS = 700;

  // The figures the library's README documents, so the big readout can be read
  // against something instead of floating free.
  // The figures are Halton's, at one configuration, with random-digit
  // scrambling. Quoting them beside a Sobol run or a nested one would compare
  // two different measurements, so the readout below checks the source and the
  // randomization as well as the point set.
  const DOCUMENTED = {
    source: "halton",
    dims: 39,
    count: 600,
    skip: 64,
    plain: 0.81,
    scrambled: 0.14,
  };

  // Sweep ceilings. Each is a plausible sampling budget rather than a round
  // binary number for its own sake; the sweep walks up to the chosen one.
  const BUDGETS = [
    { value: 4096, label: "4 096 — quick" },
    { value: 16384, label: "16 384 — standard" },
    { value: 65536, label: "65 536 — patient" },
  ];

  // --- state -------------------------------------------------------------

  const state = {
    ready: false,
    dead: false,
    info: null,
    matrix: null,
    corr: null,
    hover: null,
    pending: false,
    runId: 0,
    running: false,
    qmc: [],
    mc: [],
    lastAnnounce: 0,
  };

  const sinks = { correlate: {} };

  function setStatus(message, tone) {
    statusEl.textContent = message;
    statusEl.dataset.state = tone || "";
  }

  function announce(message) {
    const now = Date.now();

    if (now - state.lastAnnounce < LIVE_THROTTLE_MS) {
      return;
    }

    state.lastAnnounce = now;
    liveRegion.textContent = message;
  }

  // --- the wasm call wrapper ---------------------------------------------

  // Identical in contract to the Point Lab's: a missing export, a thrown error
  // and an {error} result all collapse to "message on the status line, return
  // null". Nothing from wasm is permitted to throw into a draw call.
  function call(name, opts, callOpts) {
    const silent = callOpts && callOpts.silent;

    // A panic aborts the instance. Once one is reported the module is rubble,
    // so the gate stays shut until the page is reloaded.
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

  // cacheSinks re-wraps the whole ArrayBuffer rather than storing the returned
  // view, because that view may be a subarray of a larger buffer.
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

  // --- helpers -----------------------------------------------------------

  function intValue(input, fallback) {
    const value = parseInt(input.value, 10);

    return Number.isFinite(value) ? value : fallback;
  }

  // sourceSpec, randomizationsFor and fillSourceSelect are the page's whole
  // knowledge of what a sequence offers: every menu entry, every ceiling and
  // every description comes from info(). Neither menu is written out here, and
  // the two pairs of <select>s — correlation and sweep — are filled by the same
  // two functions so they cannot drift apart.
  function sourceSpec(select) {
    const list = (state.info && state.info.sources) || [];

    return (
      list.find((spec) => spec.key === select.value) ||
      list.find((spec) => spec.sequence) ||
      null
    );
  }

  function fillSourceSelect(select, preferred) {
    const list = (state.info && state.info.sources) || [];

    select.innerHTML = "";

    for (const spec of list) {
      if (!spec.sequence) {
        continue;
      }

      const option = document.createElement("option");
      option.value = spec.key;
      option.textContent = spec.label;
      option.title = spec.description || "";
      select.append(option);
    }

    if (preferred !== undefined) {
      select.value = String(preferred);
    }

    if (!select.value && select.options.length) {
      select.value = select.options[0].value;
    }
  }

  // A randomization the newly selected sequence does not offer falls back to
  // its unrandomized entry rather than being sent to Go and refused: the
  // constructors reject a mismatched option by name, and a rejection the user
  // did not ask for reads as a broken page.
  function fillRandomizationSelect(sourceSelect, select, preferred) {
    const spec = sourceSpec(sourceSelect);
    const list = (spec && spec.randomizations) || [];
    const wanted = preferred === undefined ? select.value : preferred;

    select.innerHTML = "";

    for (const entry of list) {
      const option = document.createElement("option");
      option.value = entry.key;
      option.textContent = entry.label;
      option.title = entry.description || "";
      select.append(option);
    }

    const keep = list.some((entry) => entry.key === wanted);
    select.value = keep ? String(wanted) : list.length ? list[0].key : "";
  }

  function randomizationSpec(sourceSelect, select) {
    const spec = sourceSpec(sourceSelect);
    const list = (spec && spec.randomizations) || [];

    return list.find((entry) => entry.key === select.value) || null;
  }

  function sci(value) {
    if (value === null || value === undefined || !isFinite(value)) {
      return "—";
    }

    return value === 0 ? "0" : value.toExponential(3);
  }

  // --- controls built from info() ----------------------------------------

  function populateControls() {
    const info = state.info;
    const defaults = info.defaults || {};

    fillSourceSelect(corrSource, defaults.source);
    fillRandomizationSelect(corrSource, corrRandom, defaults.randomization);
    fillSourceSelect(convSource, defaults.source);

    // The sweep opens randomized, because RQMC is what you would actually
    // ship; the correlation panel opens unrandomized, because that is the
    // defect it exists to show. Both take whatever the selected sequence's
    // menu offers rather than a key written here — "scramble" is Halton's
    // name for it and Sobol has no such entry.
    fillRandomizationSelect(convSource, convRandom);
    convRandom.value = firstRandomized(convSource, convRandom);

    corrDims.min = "2";
    corrDims.max = String(info.maxCorrelateDims);
    corrCount.min = "8";
    corrCount.max = String(info.maxCorrelatePoints);
    corrSkip.min = "0";
    corrSkip.max = String(info.maxSkip);

    corrDims.value = String(
      Math.min(info.maxCorrelateDims, defaults.dims || DOCUMENTED.dims),
    );
    corrCount.value = String(
      Math.min(info.maxCorrelatePoints, defaults.count || DOCUMENTED.count),
    );
    corrSkip.value = String(defaults.skip);
    corrSeed.value = String(defaults.seed);

    convSkip.min = "0";
    convSkip.max = String(info.maxSkip);
    convSkip.value = String(defaults.skip);
    convSeed.value = String(defaults.seed);

    integrandSelect.innerHTML = "";

    for (const spec of info.integrands || []) {
      const option = document.createElement("option");
      option.value = spec.key;
      option.textContent = spec.label;
      integrandSelect.append(option);
    }

    budgetSelect.innerHTML = "";

    for (const budget of BUDGETS) {
      const option = document.createElement("option");
      option.value = String(budget.value);
      option.textContent = budget.label;
      budgetSelect.append(option);
    }

    budgetSelect.value = "16384";

    applyCorrSource();
    applyConvSource();
    updateCorrNote();
    applyIntegrand();
    syncOutputs();

    buildInfo.textContent = `${info.goVersion} · ${info.goos}/${info.goarch}`;
  }

  function syncOutputs() {
    corrDimsOut.textContent = corrDims.value;
    corrCountOut.textContent = Number(corrCount.value).toLocaleString("en-US");
    corrSkipOut.textContent = corrSkip.value;
    convDimsOut.textContent = convDims.value;
    convSkipOut.textContent = convSkip.value;
  }

  // firstRandomized is the sweep's opening choice: the first entry of the
  // selected sequence's menu that actually randomizes, or the unrandomized one
  // if that is all there is.
  function firstRandomized(sourceSelect, select) {
    const spec = sourceSpec(sourceSelect);
    const list = (spec && spec.randomizations) || [];
    const randomized = list.find((entry) => entry.key !== "none");

    return randomized ? randomized.key : select.value;
  }

  // Each source carries its own dimension ceiling, which may be lower than the
  // shared one. Re-ranging the slider here is what keeps a dragged control from
  // asking Go for a dimension the direction-number table cannot cover.
  function applyCorrSource() {
    const spec = sourceSpec(corrSource);

    if (!spec) {
      return;
    }

    const ceiling = Math.max(
      2,
      Math.min(state.info.maxCorrelateDims, spec.maxDims),
    );

    corrDims.max = String(ceiling);
    corrDims.value = String(Math.min(ceiling, intValue(corrDims, ceiling)));

    // Nothing on this page should print "base ?" beside a sequence that has no
    // bases, so the sentence promising them goes away with them.
    corrBaseNote.hidden = !spec.primeBases;
    syncOutputs();
  }

  function applyConvSource() {
    const spec = sourceSpec(convSource);

    if (!spec) {
      return;
    }

    applyIntegrand();
  }

  function currentIntegrand() {
    const list = (state.info && state.info.integrands) || [];

    return list.find((s) => s.key === integrandSelect.value) || null;
  }

  function applyIntegrand() {
    const spec = currentIntegrand();

    if (!spec) {
      integrandNote.textContent = "—";

      return;
    }

    // Each integrand is defined for its own range of dimensions, so the slider
    // is re-ranged rather than left free to ask Go for something it will
    // reject.
    const source = sourceSpec(convSource);
    const low = Math.max(1, spec.minDims || 1);
    const high = Math.max(
      low,
      Math.min(
        spec.maxDims || state.info.maxDims,
        source ? source.maxDims : state.info.maxDims,
      ),
    );

    convDims.min = String(low);
    convDims.max = String(high);
    convDims.value = String(
      Math.max(low, Math.min(high, intValue(convDims, low))),
    );

    integrandNote.innerHTML = `<b>${spec.label}.</b> ${spec.description} Exact value over the unit cube: <b>${Render.compact(spec.exact)}</b>. Defined for ${low}–${high} dimensions.`;
    syncOutputs();
  }

  // The description is the library's, forwarded through info(). The band that
  // the unrandomized Halton view shows is this page's own observation, so it is
  // appended rather than folded into a claim about the option.
  function updateCorrNote() {
    const entry = randomizationSpec(corrSource, corrRandom);

    if (!entry) {
      corrNote.textContent = "—";

      return;
    }

    const aside =
      entry.key === "none"
        ? " The bright band hugging the diagonal is adjacent high-dimensional coordinates walking up their ramps together; pick a randomization and it should collapse."
        : " Watch the off-diagonal warmth fall away — and note that it does not fall to exactly zero, because a finite point set never has exactly independent coordinates.";

    corrNote.innerHTML = `<b>${entry.label}.</b> ${entry.description}${aside}`;
  }

  // --- correlation -------------------------------------------------------

  function scheduleCorrelate() {
    if (state.pending || !state.ready) {
      return;
    }

    state.pending = true;
    requestAnimationFrame(() => {
      state.pending = false;
      refreshCorrelation();
    });
  }

  function refreshCorrelation() {
    if (!state.ready) {
      return;
    }

    const result = call("correlate", {
      source: corrSource.value,
      randomization: corrRandom.value,
      dims: intValue(corrDims, 39),
      count: intValue(corrCount, 600),
      skip: intValue(corrSkip, 64),
      seed: intValue(corrSeed, 1),
      out: sinks.correlate,
    });

    if (!result) {
      return;
    }

    cacheSinks(sinks.correlate, result, ["matrix"]);
    state.corr = result;
    state.matrix = result.matrix;
    state.hover = null;

    drawHeat();
    updateVerdict(result);

    setStatus(
      `${result.dims}×${result.dims} correlation over ${result.count.toLocaleString("en-US")} points · ${result.source} · randomization ${result.randomization}`,
      "ready",
    );
    announce(
      `Correlation recomputed. Worst adjacent pair ${result.worstAdjacent.toFixed(3)}.`,
    );
  }

  function drawHeat() {
    const corr = state.corr;

    state.geo = Render.drawHeatmap(heatmap, {
      matrix: state.matrix,
      dims: corr ? corr.dims : 0,
      worstPair: corr ? corr.worstPair : null,
      hover: state.hover,
    });

    Render.drawHeatLegend(heatLegend);
  }

  function updateVerdict(result) {
    const worst = Math.abs(result.worstAdjacent);
    const pair = result.worstPair || [];

    worstAdjacent.textContent = worst.toFixed(3);
    worstAdjacent.dataset.tone =
      worst < 0.3 ? "good" : worst > 0.6 ? "bad" : "";
    worstPairLabel.textContent =
      pair.length === 2
        ? `dimensions ${pair[0]} and ${pair[1]}${pairBases(result, pair)}`
        : "—";

    // The README's pair of figures is a Halton measurement with random-digit
    // scrambling. Comparing a Sobol run or a nested one against it would put
    // two different experiments in the same sentence, so the comparison is
    // only offered when every part of the configuration matches.
    const scrambled = result.randomization === "scramble";
    const target = scrambled ? DOCUMENTED.scrambled : DOCUMENTED.plain;
    const sameSetup =
      result.source === DOCUMENTED.source &&
      (scrambled || result.randomization === "none") &&
      result.dims === DOCUMENTED.dims &&
      result.count === DOCUMENTED.count &&
      result.skip === DOCUMENTED.skip;

    docReference.innerHTML = sameSetup
      ? `At this exact configuration the README quotes <b>${target.toFixed(2)}</b> ${scrambled ? "(worst of five seeds)" : ""}. You are seeing <b>${worst.toFixed(3)}</b> at seed ${result.seed}.`
      : `The README's figures — <b>0.81</b> unscrambled, <b>0.14</b> scrambled — are measured on Halton at 39 dimensions, 600 points, burn-in 64. This is ${result.source} with randomization ${result.randomization}, ${result.dims} dimensions, ${result.count.toLocaleString("en-US")} points, burn-in ${result.skip}, so the numbers are not directly comparable.`;
  }

  // A source without prime bases sends bases: null, so the clause naming them
  // is dropped rather than filled with question marks.
  function pairBases(result, pair) {
    if (!result.bases) {
      return "";
    }

    return ` — bases ${basisOf(result, pair[0])} and ${basisOf(result, pair[1])}`;
  }

  function basisOf(result, index) {
    const bases = result.bases || [];

    return bases[index] === undefined ? "?" : bases[index];
  }

  function cellAt(event) {
    const geo = state.geo;

    if (!geo || !geo.dims || !geo.cell) {
      return null;
    }

    const rect = heatmap.getBoundingClientRect();
    const x = event.clientX - rect.left - geo.left;
    const y = event.clientY - rect.top - geo.top;

    if (x < 0 || y < 0 || x >= geo.size || y >= geo.size) {
      return null;
    }

    return {
      i: Math.min(geo.dims - 1, Math.floor(y / geo.cell)),
      j: Math.min(geo.dims - 1, Math.floor(x / geo.cell)),
    };
  }

  function wireHeatmapHover() {
    heatmap.addEventListener("mousemove", (event) => {
      const cell = cellAt(event);
      const corr = state.corr;

      if (!cell || !corr) {
        return;
      }

      const previous = state.hover;

      if (previous && previous.i === cell.i && previous.j === cell.j) {
        return;
      }

      state.hover = cell;
      drawHeat();

      const r = state.matrix[cell.i * corr.dims + cell.j];

      cellReadout.innerHTML =
        cell.i === cell.j
          ? `<b>dim ${cell.i}</b> against itself${corr.bases ? ` — base ${basisOf(corr, cell.i)}` : ""}, r = 1 by construction`
          : `<b>dim ${cell.i} × dim ${cell.j}</b>${pairBases(corr, [cell.i, cell.j])}, r = <b>${r.toFixed(4)}</b>`;
    });

    heatmap.addEventListener("mouseleave", () => {
      if (!state.hover) {
        return;
      }

      state.hover = null;
      drawHeat();
      cellReadout.textContent = "hover a cell for the pair, their bases and r";
    });
  }

  // --- convergence sweep -------------------------------------------------

  // A geometric ladder at √2 per step: dense enough that a slope is readable
  // on log–log axes, sparse enough that the whole sweep is a couple of dozen
  // blocking calls rather than a couple of hundred.
  function sweepPoints(ceiling) {
    const values = [];
    let n = 64;

    while (n < ceiling) {
      const rounded = Math.round(n);

      if (values[values.length - 1] !== rounded) {
        values.push(rounded);
      }

      n *= Math.SQRT2;
    }

    values.push(ceiling);

    return values;
  }

  function resetSweep() {
    state.qmc = [];
    state.mc = [];
    convRows.innerHTML = "";
    readout.exact.textContent = "—";
    readout.qmc.textContent = "—";
    readout.mc.textContent = "—";
    readout.ratio.textContent = "—";
    progressBar.style.width = "0%";
    drawChart();
  }

  async function startSweep() {
    if (state.running || !state.ready) {
      return;
    }

    state.runId += 1;
    state.running = true;

    const runId = state.runId;
    const ns = sweepPoints(parseInt(budgetSelect.value, 10) || 16384);
    const request = {
      source: convSource.value,
      randomization: convRandom.value,
      dims: intValue(convDims, 8),
      skip: intValue(convSkip, 64),
      seed: intValue(convSeed, 1),
      integrand: integrandSelect.value,
    };

    resetSweep();
    startButton.disabled = true;
    stopButton.disabled = false;
    setStatus("Sweeping N…", "loading");

    for (let step = 0; step < ns.length; step += 1) {
      // The guard is checked before the blocking call, not only after it: Stop
      // and a restart both land in the yield below, and neither should get one
      // more Go call out of a sweep that is already over.
      if (runId !== state.runId) {
        return;
      }

      const result = call(
        "converge",
        Object.assign({}, request, { n: ns[step] }),
      );

      if (runId !== state.runId) {
        return;
      }

      if (!result) {
        finishSweep(runId, "sweep stopped — see the status line");

        return;
      }

      state.qmc.push({ x: result.n, y: Math.abs(result.qmcError) });
      state.mc.push({ x: result.n, y: Math.abs(result.mcError) });

      appendRow(result);
      updateSweepReadout(result);
      drawChart();

      const done = step + 1;
      progressBar.style.width = `${(done / ns.length) * 100}%`;
      progressText.textContent = `N = ${result.n.toLocaleString("en-US")} · ${done} / ${ns.length}`;
      announce(`N ${result.n}, QMC error ${sci(Math.abs(result.qmcError))}.`);

      // THE yield. A synchronous Go call cannot be interrupted, so this gap is
      // the only moment a Stop click or a restart can be dispatched. It is not
      // a politeness; delete it and the Stop button becomes decorative.
      await new Promise((resolve) => setTimeout(resolve, 0));
    }

    finishSweep(runId, "sweep complete");
  }

  function finishSweep(runId, message) {
    if (runId !== state.runId) {
      return;
    }

    state.running = false;
    startButton.disabled = false;
    stopButton.disabled = true;
    progressText.textContent = message;

    if (statusEl.dataset.state !== "error") {
      setStatus(message, "ready");
    }
  }

  function stopSweep() {
    if (!state.running) {
      return;
    }

    // Bumping the id is what actually cancels: the loop re-reads it after its
    // next yield, sees a stranger's number, and returns without touching
    // anything the new sweep owns.
    state.runId += 1;
    state.running = false;
    startButton.disabled = false;
    stopButton.disabled = true;
    progressText.textContent = "stopped — partial results kept";
    setStatus("Sweep stopped", "ready");
  }

  function appendRow(result) {
    const qmcError = Math.abs(result.qmcError);
    const mcError = Math.abs(result.mcError);
    const ratio = qmcError > 0 ? mcError / qmcError : Infinity;

    const row = document.createElement("tr");
    row.innerHTML =
      `<td>${result.n.toLocaleString("en-US")}</td>` +
      `<td>${sci(qmcError)}</td>` +
      `<td>${sci(mcError)}</td>` +
      `<td class="${ratio >= 1 ? "win" : "lose"}">${isFinite(ratio) ? `${ratio.toFixed(1)}×` : "∞"}</td>`;
    convRows.append(row);
  }

  function updateSweepReadout(result) {
    const qmcError = Math.abs(result.qmcError);
    const mcError = Math.abs(result.mcError);

    readout.exact.textContent = Render.compact(result.exact);
    readout.qmc.textContent = sci(qmcError);
    readout.mc.textContent = sci(mcError);
    readout.ratio.textContent =
      qmcError > 0 ? `${(mcError / qmcError).toFixed(1)}×` : "∞";
  }

  function drawChart() {
    const qmcColor = Render.readVar("--halton", "#46e0c8");
    const mcColor = Render.readVar("--random", "#ffb04a");
    const refColor = Render.readVar("--mark", "#ff5d8f");

    const refs = [];

    // Anchor each reference to the first measured point of the series it is
    // there to be compared against, so the eye compares slopes and not
    // intercepts.
    if (state.qmc.length) {
      refs.push({
        slope: -1,
        label: "1/N",
        anchor: state.qmc[0],
        color: refColor,
      });
    }

    if (state.mc.length) {
      refs.push({
        slope: -0.5,
        label: "1/√N",
        anchor: state.mc[0],
        color: refColor,
      });
    }

    Render.drawLogLog(
      convChart,
      [
        { points: state.qmc, color: qmcColor, glyph: "circle", width: 1.9 },
        {
          points: state.mc,
          color: mcColor,
          glyph: "cross",
          dash: [6, 4],
          width: 1.6,
        },
      ],
      {
        refs,
        xLabel: "N — points drawn",
        yLabel: "absolute error",
        empty: "press Start to sweep N",
      },
    );
  }

  // --- wiring ------------------------------------------------------------

  function wireControls() {
    for (const input of [corrDims, corrCount, corrSkip]) {
      input.addEventListener("input", () => {
        syncOutputs();
        scheduleCorrelate();
      });
    }

    corrSeed.addEventListener("change", scheduleCorrelate);

    corrSource.addEventListener("change", () => {
      fillRandomizationSelect(corrSource, corrRandom);
      applyCorrSource();
      updateCorrNote();
      scheduleCorrelate();
    });

    corrRandom.addEventListener("change", () => {
      updateCorrNote();
      scheduleCorrelate();
    });

    for (const input of [convDims, convSkip]) {
      input.addEventListener("input", syncOutputs);
    }

    integrandSelect.addEventListener("change", () => {
      applyIntegrand();
      resetSweep();
    });

    convSource.addEventListener("change", () => {
      fillRandomizationSelect(convSource, convRandom);
      applyConvSource();
      resetSweep();
    });

    convRandom.addEventListener("change", resetSweep);

    startButton.addEventListener("click", () => {
      startSweep().catch((err) => {
        console.error(err);
        setStatus(`sweep failed: ${err && err.message}`, "error");
        stopSweep();
      });
    });

    stopButton.addEventListener("click", stopSweep);

    wireHeatmapHover();

    let resizeTimer = null;

    window.addEventListener("resize", () => {
      window.clearTimeout(resizeTimer);
      resizeTimer = window.setTimeout(() => {
        drawHeat();
        drawChart();
      }, 120);
    });

    Render.watchDPR(() => {
      Render.invalidateTheme();
      drawHeat();
      drawChart();
    });
  }

  // --- boot --------------------------------------------------------------

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

    corrSource.disabled = false;
    corrRandom.disabled = false;
    convSource.disabled = false;
    convRandom.disabled = false;
    startButton.disabled = false;

    setStatus("WASM ready", "ready");
    refreshCorrelation();
    drawChart();
  }

  initWasm().catch((err) => {
    console.error(err);
    setStatus(
      "WebAssembly failed to load. Serve this page over HTTP — a file:// URL cannot fetch a .wasm — and check that qmc.wasm is sent with Content-Type: application/wasm.",
      "error",
    );
  });
})();
