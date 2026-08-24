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
  const corrLeapInput = el("corrLeap");
  const corrLeapNext = el("corrLeapNext");
  const corrLeapNote = el("corrLeapNote");

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
  const convLeapInput = el("convLeap");
  const convLeapNext = el("convLeapNext");
  const convLeapNote = el("convLeapNote");
  const startButton = el("start");
  const stopButton = el("stop");
  const progressBar = el("progressBar");
  const progressText = el("progressText");
  const convChart = el("convChart");
  const convRows = el("convRows");

  const discMetric = el("discMetric");
  const discMetricNote = el("discMetricNote");
  const discSuggest = el("discSuggest");
  const discDims = el("discDims");
  const discDimsOut = el("discDimsOut");
  const discSkip = el("discSkip");
  const discSkipOut = el("discSkipOut");
  const discSeed = el("discSeed");
  const discSource = el("discSource");
  const discRandom = el("discRandom");
  const discLeapInput = el("discLeap");
  const discLeapNext = el("discLeapNext");
  const discLeapNote = el("discLeapNote");
  const discStart = el("discStart");
  const discStop = el("discStop");
  const discProgressBar = el("discProgressBar");
  const discProgressText = el("discProgressText");
  const discChart = el("discChart");
  const discRows = el("discRows");
  const discCeilingNote = el("discCeilingNote");
  const discVerdict = el("discVerdict");

  const readout = {
    exact: el("tExact"),
    qmc: el("tQmcError"),
    mc: el("tMcError"),
    ratio: el("tRatio"),
  };

  const discReadout = {
    ratio: el("dRatio"),
    value: el("dValue"),
    random: el("dRandom"),
    analytic: el("dAnalytic"),
    ceiling: el("dCeiling"),
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

    // The README's figures are unleaped. A leap is a different experiment —
    // the same generator sampled on a stride — so it disqualifies the
    // comparison exactly the way a different burn-in does.
    leap: 1,
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

    // The one sweep now walking its ladder, of either panel, or null. Both
    // panels share it for the reason runSweep explains.
    sweep: null,

    // The last metrics() answer for the dimension count now on the slider.
    // Whether a metric is available there, and how many points it affords, are
    // the library's questions to answer, not this page's to guess.
    metrics: null,
    qmc: [],
    mc: [],
    disc: { seq: [], rnd: [], analytic: [] },
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

  // --- the leap controls -------------------------------------------------

  // Created in wireControls, because the factory attaches its own listeners
  // and this page has exactly one place where listeners are attached.
  let corrLeap = null;
  let convLeap = null;
  let discLeap = null;

  // Both panels have a leap of their own, so this is a factory rather than two
  // copies of the same handler. Each instance owns one <input>, one note and
  // one "next admissible" button, and remembers the last verdict the leaps()
  // export gave for that combination of sequence, dimension count and leap.
  //
  // The page never decides for itself whether a leap is legal. Which leaps a
  // generator accepts depends on its bases — every even number is refused by
  // Sobol, and Halton's answer moves as dimensions are added — and the export
  // settles it by building a generator and reading the constructor's error,
  // which is the same thing the correlate or converge call is about to do.
  function leapControl(parts) {
    const local = { verdict: null };

    function refresh() {
      const leap = intValue(parts.input, 1);

      const result = call("leaps", {
        source: parts.source.value,
        dims: intValue(parts.dims, 39),
        leap: leap,
      });

      local.verdict = result;
      render(result, leap);

      parts.button.disabled = !(
        result &&
        result.suggested &&
        result.suggested !== leap
      );
    }

    function render(result, leap) {
      if (!result) {
        parts.note.textContent =
          "The leap could not be checked; leave it at 1 until the module answers again.";

        return;
      }

      const examples = (result.examples || []).join(", ");

      if (leap <= 1) {
        parts.note.innerHTML = `Leap <b>1</b> is every point in order. Raise it and point <i>i</i> becomes raw index <b>skip + 1 + i·L</b> — a deterministic alternative to scrambling, with no seed${examples ? `. The smallest admissible leaps here are <b>${examples}</b>` : ""}.`;

        return;
      }

      if (result.admissible) {
        parts.note.innerHTML = `Leap <b>${leap}</b> shares no factor with any base in use, so every coordinate still sees its whole digit alphabet.`;

        return;
      }

      // The library's own sentence, not a paraphrase: it names the dimension
      // and the base, which is the part that explains the trap.
      parts.note.innerHTML = `<b>Refused:</b> ${escapeHTML(result.reason || "this leap is not admissible here")}. That coordinate's leading digit would never change, confining it to one strip of width 1/base — and scrambling does not rescue it.${result.suggested ? ` The next admissible leap is <b>${result.suggested}</b>.` : ""}`;
    }

    parts.input.addEventListener("input", () => {
      refresh();

      if (parts.onChange) {
        parts.onChange();
      }
    });

    parts.button.addEventListener("click", () => {
      if (!local.verdict || !local.verdict.suggested) {
        return;
      }

      parts.input.value = String(local.verdict.suggested);
      refresh();

      if (parts.onChange) {
        parts.onChange();
      }
    });

    return {
      refresh: refresh,
      value: () => intValue(parts.input, 1),
      admissible: () => !local.verdict || local.verdict.admissible,
      reason: () => (local.verdict && local.verdict.reason) || "",
    };
  }

  function escapeHTML(text) {
    const box = document.createElement("span");
    box.textContent = text;

    return box.innerHTML;
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
    corrLeapInput.min = "1";
    corrLeapInput.max = String(info.maxLeap);
    corrLeapInput.value = String(defaults.leap);

    convSkip.min = "0";
    convSkip.max = String(info.maxSkip);
    convSkip.value = String(defaults.skip);
    convSeed.value = String(defaults.seed);
    convLeapInput.min = "1";
    convLeapInput.max = String(info.maxLeap);
    convLeapInput.value = String(defaults.leap);

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

    fillSourceSelect(discSource, defaults.source);

    // The discrepancy panel opens randomized for the same reason the sweep
    // does: RQMC is what you would actually ship, and an unrandomized Halton
    // at 39 dimensions would be showing the leaping defect on top of the
    // saturation this panel exists to show.
    fillRandomizationSelect(discSource, discRandom);
    discRandom.value = firstRandomized(discSource, discRandom);

    discSkip.min = "0";
    discSkip.max = String(info.maxSkip);
    discSkip.value = String(defaults.skip);
    discSeed.value = String(defaults.seed);
    discLeapInput.min = "1";
    discLeapInput.max = String(info.maxLeap);
    discLeapInput.value = String(defaults.leap);
    discDims.value = String(defaults.dims || DOCUMENTED.dims);

    discMetric.innerHTML = "";

    for (const spec of info.discrepancies || []) {
      const option = document.createElement("option");
      option.value = spec.key;
      option.textContent = spec.label;
      option.title = spec.description || "";
      discMetric.append(option);
    }

    // The menu stays fully populated at every dimension count. What changes is
    // whether Start is enabled, under the library's own refusal — the same
    // shape as the leap control, where an inadmissible value is shown and
    // explained rather than removed.
    discMetric.value = defaults.metric || discMetric.options[0].value;

    applyCorrSource();
    applyConvSource();
    applyDiscSource();
    updateCorrNote();
    applyIntegrand();
    syncOutputs();
    refreshMetrics();

    buildInfo.textContent = `${info.goVersion} · ${info.goos}/${info.goarch}`;
  }

  function syncOutputs() {
    corrDimsOut.textContent = corrDims.value;
    corrCountOut.textContent = Number(corrCount.value).toLocaleString("en-US");
    corrSkipOut.textContent = corrSkip.value;
    convDimsOut.textContent = convDims.value;
    convSkipOut.textContent = convSkip.value;
    discDimsOut.textContent = discDims.value;
    discSkipOut.textContent = discSkip.value;
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
    // bases, so both sentences promising them go away with them.
    corrBaseNote.hidden = !spec.primeBases;
    cellReadout.textContent = idlePrompt();
    syncOutputs();
  }

  function applyConvSource() {
    const spec = sourceSpec(convSource);

    if (!spec) {
      return;
    }

    applyIntegrand();
  }

  // Each source carries its own dimension ceiling, and the discrepancy panel
  // ranges from 1 rather than from 2: star discrepancy in one dimension has a
  // closed form the library's tests pin, and it is the one place on this page
  // where the sequence beats random by a factor of forty.
  function applyDiscSource() {
    const spec = sourceSpec(discSource);

    if (!spec) {
      return;
    }

    const ceiling = Math.max(1, Math.min(state.info.maxDims, spec.maxDims));

    discDims.min = "1";
    discDims.max = String(ceiling);
    discDims.value = String(Math.min(ceiling, intValue(discDims, ceiling)));
    syncOutputs();
  }

  // The hover prompt names the bases only when there are bases to name.
  function idlePrompt() {
    const spec = sourceSpec(corrSource);

    return spec && spec.primeBases
      ? "hover a cell for the pair, their bases and r"
      : "hover a cell for the pair and r";
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

  // The description is the library's, forwarded through info(). What the map
  // beside it actually does is this page's own observation, so it is appended
  // rather than folded into a claim about the option.
  function updateCorrNote() {
    const entry = randomizationSpec(corrSource, corrRandom);

    if (!entry) {
      corrNote.textContent = "—";

      return;
    }

    corrNote.innerHTML = `<b>${entry.label}.</b> ${entry.description}${correlationAside(entry)}`;
  }

  // The aside follows the source, because the two sequences fail differently
  // and the same sentence cannot describe both maps. primeBases is the flag
  // that separates them: one base per dimension is exactly what makes a
  // high-dimensional coordinate ramp, and the ramp is what puts the band on
  // the diagonal. Sobol is base 2 everywhere and has no band — at the defaults
  // above, seed 1, its worst adjacent |r| is 0.027 against Halton's 0.808 — so
  // promising a collapse there would have this text contradicting the picture
  // next to it.
  function correlationAside(entry) {
    const ramps = (sourceSpec(corrSource) || {}).primeBases;

    if (entry.key === "none") {
      return ramps
        ? " The bright band hugging the diagonal is adjacent high-dimensional coordinates walking up their ramps together; pick a randomization and it should collapse."
        : " There is no band to collapse here: base 2 in every dimension leaves the unrandomized map already near-independent, worst adjacent |r| 0.027 against Halton's 0.808 at the defaults above, seed 1.";
    }

    return ramps
      ? " Watch the off-diagonal warmth fall away — and note that it does not fall to exactly zero, because a finite point set never has exactly independent coordinates."
      : " Expect the map to stay much as it was. Pairwise correlation was never the defect this randomization is for; what it buys is a distribution over seeds, which is what the error curve below is drawn from.";
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

    // An inadmissible leap is refused by the constructor, so the call would
    // only put a Go error on the status line over a stale heatmap. The note
    // under the control has already said why, in the library's own words.
    if (corrLeap && !corrLeap.admissible()) {
      setStatus(corrLeap.reason(), "error");

      return;
    }

    const result = call("correlate", {
      source: corrSource.value,
      randomization: corrRandom.value,
      dims: intValue(corrDims, 39),
      count: intValue(corrCount, 600),
      skip: intValue(corrSkip, 64),
      leap: corrLeap ? corrLeap.value() : 1,
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
      `Correlation recomputed. Worst adjacent pair ${coefficient(result.worstAdjacent)}.`,
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
    // Math.abs(null) is 0, so reading the coefficient without testing it first
    // would not throw here — it would do something worse and print a confident
    // 0.000, the best possible verdict, for a measurement that does not exist.
    const worst = finite(result.worstAdjacent)
      ? Math.abs(result.worstAdjacent)
      : null;
    const pair = result.worstPair || [];

    worstAdjacent.textContent = coefficient(worst);
    worstAdjacent.dataset.tone =
      worst === null ? "" : worst < 0.3 ? "good" : worst > 0.6 ? "bad" : "";
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
      result.skip === DOCUMENTED.skip &&
      result.leap === DOCUMENTED.leap;

    docReference.innerHTML = sameSetup
      ? `At this exact configuration the README quotes <b>${target.toFixed(2)}</b> ${scrambled ? "(worst of five seeds)" : ""}. You are seeing <b>${coefficient(worst)}</b> at seed ${result.seed}.`
      : `The README's figures — <b>0.81</b> unscrambled, <b>0.14</b> scrambled — are measured on Halton at 39 dimensions, 600 points, burn-in 64, no leap. This is ${result.source} with randomization ${result.randomization}, ${result.dims} dimensions, ${result.count.toLocaleString("en-US")} points, burn-in ${result.skip}, leap ${result.leap}, so the numbers are not directly comparable.`;
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
      cellReadout.textContent = idlePrompt();
    });
  }

  // --- convergence sweep -------------------------------------------------

  // A geometric ladder at √2 per step: dense enough that a slope is readable
  // on log–log axes, sparse enough that the whole sweep is a couple of dozen
  // blocking calls rather than a couple of hundred.
  //
  // The floor is a parameter because the discrepancy panel's ceiling can be
  // below the convergence panel's first rung: star discrepancy at six
  // dimensions affords 32 points in total, and a ladder starting at 64 would
  // be one rung long.
  function sweepPoints(ceiling, floor) {
    const values = [];
    let n = Math.min(floor || 64, ceiling);

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

  // runSweep is the ladder BOTH panels walk. It was extracted from the
  // convergence sweep rather than copied for it: the yield-and-recheck dance
  // below is subtle enough that two copies would drift, and the second copy is
  // always the one that forgets to re-check the id before the blocking call.
  //
  // job = {
  //   steps          array of N values, in order
  //   exportName     the Go export to call, once per step
  //   request(n)     the options object for step n
  //   onResult(r, i) draw it
  //   buttons        {start, stop}
  //   progress       {bar, text}
  //   label(r)       the left half of the progress line
  //   announce(r)    the aria-live sentence
  //   status         the status line while it runs
  // }
  //
  // The two sweeps SHARE state.runId, deliberately. The page has one thread
  // and every rung is a blocking call into Go, so two sweeps could not run
  // side by side even with separate ids — they would interleave, each freezing
  // the other's yields, and both progress bars would crawl. Sharing the id
  // makes "start the other panel" mean "cancel this one", which is what the
  // machine was going to do anyway; the difference is that the cancelled
  // panel's transport is restored here instead of being left disabled.
  async function runSweep(job) {
    if (!state.ready || state.sweep === job) {
      return;
    }

    state.runId += 1;

    const runId = state.runId;

    if (state.sweep) {
      finishSweep(state.sweep, "superseded by the other sweep");
    }

    state.sweep = job;
    job.buttons.start.disabled = true;
    job.buttons.stop.disabled = false;
    job.progress.bar.style.width = "0%";
    setStatus(job.status, "loading");

    for (let step = 0; step < job.steps.length; step += 1) {
      // The guard is checked before the blocking call, not only after it: Stop
      // and a restart both land in the yield below, and neither should get one
      // more Go call out of a sweep that is already over.
      if (runId !== state.runId) {
        return;
      }

      const result = call(job.exportName, job.request(job.steps[step]));

      if (runId !== state.runId) {
        return;
      }

      if (!result) {
        finishSweep(job, "sweep stopped — see the status line");

        return;
      }

      job.onResult(result, step);

      const done = step + 1;
      job.progress.bar.style.width = `${(done / job.steps.length) * 100}%`;
      job.progress.text.textContent = `${job.label(result)} · ${done} / ${job.steps.length}`;
      announce(job.announce(result));

      // THE yield. A synchronous Go call cannot be interrupted, so this gap is
      // the only moment a Stop click or a restart can be dispatched. It is not
      // a politeness; delete it and the Stop button becomes decorative.
      await new Promise((resolve) => setTimeout(resolve, 0));
    }

    if (runId !== state.runId) {
      return;
    }

    finishSweep(job, "sweep complete");
  }

  function finishSweep(job, message) {
    if (state.sweep === job) {
      state.sweep = null;
    }

    job.buttons.start.disabled = false;
    job.buttons.stop.disabled = true;
    job.progress.text.textContent = message;

    if (statusEl.dataset.state !== "error") {
      setStatus(message, "ready");
    }

    // A panel whose Start is not unconditionally legal gets the last word on
    // its own transport: the discrepancy panel re-asks metrics() here, so a
    // sweep that ended at a dimension count where the metric is unavailable
    // does not leave an enabled button that Go would only refuse.
    if (job.onFinish) {
      job.onFinish();
    }
  }

  function stopSweep() {
    const job = state.sweep;

    if (!job) {
      return;
    }

    // Bumping the id is what actually cancels: the loop re-reads it after its
    // next yield, sees a stranger's number, and returns without touching
    // anything the new sweep owns.
    state.runId += 1;
    finishSweep(job, "stopped — partial results kept");
    setStatus("Sweep stopped", "ready");
  }

  function startSweep() {
    if (convLeap && !convLeap.admissible()) {
      setStatus(convLeap.reason(), "error");

      return Promise.resolve();
    }

    const request = {
      source: convSource.value,
      randomization: convRandom.value,
      dims: intValue(convDims, 8),
      skip: intValue(convSkip, 64),
      leap: convLeap ? convLeap.value() : 1,
      seed: intValue(convSeed, 1),
      integrand: integrandSelect.value,
    };

    resetSweep();

    return runSweep({
      steps: sweepPoints(parseInt(budgetSelect.value, 10) || 16384),
      exportName: "converge",
      request: (n) => Object.assign({}, request, { n: n }),
      buttons: { start: startButton, stop: stopButton },
      progress: { bar: progressBar, text: progressText },
      status: "Sweeping N…",
      label: (result) => `N = ${result.n.toLocaleString("en-US")}`,
      announce: (result) =>
        `N ${result.n}, QMC error ${sci(Math.abs(result.qmcError))}.`,
      onResult: (result) => {
        state.qmc.push({ x: result.n, y: Math.abs(result.qmcError) });
        state.mc.push({ x: result.n, y: Math.abs(result.mcError) });

        appendRow(result);
        updateSweepReadout(result);
        drawChart();
      },
    });
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

  // --- discrepancy sweep ---------------------------------------------------

  // The discrepancy ladder starts lower than the convergence one, because its
  // ceiling can be below the convergence panel's first rung: star discrepancy
  // at six dimensions affords 32 points in total, and a ladder starting at 64
  // would be one rung long and would not draw a line.
  const DISC_FLOOR = 16;

  // times formats a ratio. Every number Go sends arrives jsNumber-nullable —
  // NaN and ±Inf become null on the way out — and null.toFixed is a TypeError
  // that would kill a sweep mid-ladder, so nothing here calls toFixed on a
  // value it has not tested first.
  function times(value) {
    if (value === null || value === undefined || !isFinite(value)) {
      return "—";
    }

    return `${value.toFixed(2)}×`;
  }

  // finite is the test times() makes, pulled out so the correlation panel can
  // make the same one. jsNumber in bridge.go turns NaN and ±Inf into null on
  // the way out, so every number the Go side sends — worstAdjacent included —
  // arrives nullable whether or not today's arithmetic can produce one.
  function finite(value) {
    return value !== null && value !== undefined && isFinite(value);
  }

  // coefficient formats a correlation coefficient, or an em-dash when there is
  // no number to format. Calling toFixed on the null directly is a TypeError,
  // and in refreshCorrelation it would escape uncaught from the announce()
  // call at the very end of a successful refresh — the heatmap already drawn,
  // the status line already saying "ready", and the panel dead for every
  // subsequent control change.
  function coefficient(value) {
    return finite(value) ? value.toFixed(3) : "—";
  }

  function currentMetric() {
    const list = (state.metrics && state.metrics.metrics) || [];

    return list.find((entry) => entry.key === discMetric.value) || null;
  }

  // refreshMetrics is this panel's leaps(): it asks Go whether the selected
  // metric can be computed at the dimension count now on the slider, and how
  // many points it can afford there.
  //
  // Neither answer is derived here. The star ceiling is a property of the
  // library's own work budget and the centred-L2 ceiling is a property of a
  // measured js/wasm cost model, and both live in the Go file that owns them.
  // When the library refuses, its sentence is printed verbatim: it names the
  // dimension count, the leaf count, that the ceiling is a property of the
  // problem rather than a tuning knob, and the affordable point counts per
  // dimension. No paraphrase of that is worth writing.
  function refreshMetrics() {
    const result = call("metrics", {
      source: discSource.value,
      dims: intValue(discDims, 39),
    });

    state.metrics = result;

    const entry = currentMetric();

    if (!entry) {
      discMetricNote.textContent =
        "The metric menu could not be checked; reload the page if this persists.";
      discSuggest.hidden = true;
      discStart.disabled = true;
      discReadout.ceiling.textContent = "—";
      discCeilingNote.textContent = "—";

      return;
    }

    if (!entry.available) {
      discMetricNote.innerHTML = `<b>Refused:</b> ${escapeHTML(entry.reason || "this metric is not available here")}`;

      const suggestion = entry.suggestedDims;
      const offered = suggestion !== null && suggestion !== undefined;

      discSuggest.hidden = !offered;

      if (offered) {
        discSuggest.textContent = `Drop to ${suggestion} dimension${suggestion === 1 ? "" : "s"}`;
      }

      discStart.disabled = true;
      discReadout.ceiling.textContent = "—";
      discCeilingNote.textContent =
        "There is no N ceiling to report: this metric cannot be computed at this dimension count at any point count.";

      return;
    }

    discMetricNote.innerHTML = `<b>${escapeHTML(entry.label)}.</b> ${escapeHTML(entry.description)}`;
    discSuggest.hidden = true;
    discStart.disabled = !state.ready || state.sweep !== null;
    discReadout.ceiling.textContent = entry.maxPoints.toLocaleString("en-US");
    discCeilingNote.innerHTML = ceilingSentence(entry);
  }

  // The sentence that keeps a moving ceiling from reading as a bug. It is the
  // only control on either page whose range changes when a different control
  // moves, and the two metrics move it for entirely different reasons.
  function ceilingSentence(entry) {
    const affordable = `<b>${entry.maxPoints.toLocaleString("en-US")}</b> points at <b>${entry.dims}</b> dimensions`;

    if (entry.key === "star") {
      return `<b>The N ceiling moves with the dimension slider.</b> Exact star discrepancy is NP-hard in the dimension, so its ceiling does not fall as dimensions are added — it collapses: ${affordable}, against a few thousand at two. Expect a short ladder, two or three rungs wide, and read the ratio rather than the slope.`;
    }

    return `<b>The N ceiling moves with the dimension slider.</b> Centred L2 costs O(N²s) and the library computes one N in a single atomic call, so it cannot be sliced the way the sweep itself is; capping N is the only lever left, and the cap has to fall as dimensions are added — ${affordable}, against a few thousand in two or three. This is a browser-responsiveness limit and not a mathematical one: the library will measure as many points as you have patience for.`;
  }

  function resetDiscSweep() {
    state.disc = { seq: [], rnd: [], analytic: [] };
    discRows.innerHTML = "";
    discReadout.ratio.textContent = "—";
    discReadout.ratio.dataset.tone = "";
    discReadout.value.textContent = "—";
    discReadout.random.textContent = "—";
    discReadout.analytic.textContent = "—";
    discVerdict.textContent =
      "Press Start. The page opens on 39 dimensions and centred L2, which is the configuration in which this statistic says nothing.";
    discProgressBar.style.width = "0%";
    drawDiscChart();
  }

  function startDiscSweep() {
    const entry = currentMetric();

    if (!entry || !entry.available) {
      setStatus(
        (entry && entry.reason) || "that metric is unavailable here",
        "error",
      );

      return Promise.resolve();
    }

    if (discLeap && !discLeap.admissible()) {
      setStatus(discLeap.reason(), "error");

      return Promise.resolve();
    }

    const request = {
      metric: discMetric.value,
      source: discSource.value,
      randomization: discRandom.value,
      dims: intValue(discDims, 39),
      skip: intValue(discSkip, 64),
      leap: discLeap ? discLeap.value() : 1,
      seed: intValue(discSeed, 1),
    };

    resetDiscSweep();

    return runSweep({
      steps: sweepPoints(entry.maxPoints, DISC_FLOOR),
      exportName: "discrepancy",
      request: (n) => Object.assign({}, request, { n: n }),
      buttons: { start: discStart, stop: discStop },
      progress: { bar: discProgressBar, text: discProgressText },
      status: "Measuring discrepancy…",
      label: (result) => `N = ${result.n.toLocaleString("en-US")}`,
      announce: (result) => `N ${result.n}, ratio ${times(result.ratio)}.`,
      onFinish: refreshMetrics,
      onResult: (result) => {
        state.disc.seq.push({ x: result.n, y: result.value });
        state.disc.rnd.push({ x: result.n, y: result.randomValue });

        // Star has no closed form for the random expectation, so its third
        // series simply stays empty and the chart draws two.
        if (result.analytic !== null && result.analytic !== undefined) {
          state.disc.analytic.push({ x: result.n, y: result.analytic });
        }

        appendDiscRow(result);
        updateDiscReadout(result);
        drawDiscChart();
      },
    });
  }

  function appendDiscRow(result) {
    const row = document.createElement("tr");
    row.innerHTML =
      `<td>${result.n.toLocaleString("en-US")}</td>` +
      `<td>${sci(result.value)}</td>` +
      `<td>${sci(result.randomValue)}</td>` +
      `<td class="${result.separates ? "win" : "lose"}">${times(result.ratio)}</td>`;
    discRows.append(row);
  }

  // Both the tone and the sentence are Go's. Whether 1.28 counts as "still
  // separating them" is a judgement about a measured decay, and the threshold
  // it was picked from lives beside the measurements in discrepancy.go; a page
  // that re-decided it here would be a second copy of that judgement, free to
  // disagree with the constant.
  function updateDiscReadout(result) {
    discReadout.ratio.textContent = times(result.ratio);
    discReadout.ratio.dataset.tone = result.separates ? "good" : "bad";
    discReadout.value.textContent = sci(result.value);
    discReadout.random.textContent = sci(result.randomValue);
    discReadout.analytic.textContent =
      result.analytic === null || result.analytic === undefined
        ? "no closed form"
        : sci(result.analytic);
    discReadout.ceiling.textContent = result.maxPoints.toLocaleString("en-US");
    discVerdict.textContent = result.verdict;
  }

  function drawDiscChart() {
    const seqColor = Render.readVar("--halton", "#46e0c8");
    const rndColor = Render.readVar("--random", "#ffb04a");
    const refColor = Render.readVar("--mark", "#ff5d8f");

    Render.drawLogLog(
      discChart,
      [
        {
          points: state.disc.seq,
          color: seqColor,
          glyph: "circle",
          width: 1.9,
        },
        {
          points: state.disc.rnd,
          color: rndColor,
          glyph: "cross",
          dash: [6, 4],
          width: 1.6,
        },
        {
          points: state.disc.analytic,
          color: refColor,
          glyph: "none",
          dash: [2, 4],
          width: 1.2,
        },
      ],
      {
        // No reference slopes, deliberately. Neither statistic's theoretical
        // rate is a power law — the classical star bound carries a (log N)^s —
        // so a straight 1/N line here would be a decoration that read as a
        // claim. The analytic random expectation is drawn instead, and that
        // one is exact.
        xLabel: "N — points drawn",
        yLabel: "discrepancy",
        empty: "press Start to sweep N",

        // One decade, not the default two. A star sweep at four dimensions
        // runs from 0.11 to 0.06 and the forced second decade would squash the
        // whole picture into the top of the plot.
        yMinSpan: 1,
      },
    );
  }

  // --- wiring ------------------------------------------------------------

  function wireControls() {
    corrLeap = leapControl({
      input: corrLeapInput,
      button: corrLeapNext,
      note: corrLeapNote,
      source: corrSource,
      dims: corrDims,
      onChange: scheduleCorrelate,
    });

    convLeap = leapControl({
      input: convLeapInput,
      button: convLeapNext,
      note: convLeapNote,
      source: convSource,
      dims: convDims,
      onChange: resetSweep,
    });

    discLeap = leapControl({
      input: discLeapInput,
      button: discLeapNext,
      note: discLeapNote,
      source: discSource,
      dims: discDims,
      onChange: resetDiscSweep,
    });

    for (const input of [corrDims, corrCount, corrSkip]) {
      input.addEventListener("input", () => {
        syncOutputs();

        // A leap admissible at six dimensions can be refused at seven, where
        // the next prime joins the base list, so the verdict is not a property
        // of the leap alone.
        if (input === corrDims) {
          corrLeap.refresh();
        }

        scheduleCorrelate();
      });
    }

    corrSeed.addEventListener("change", scheduleCorrelate);

    corrSource.addEventListener("change", () => {
      fillRandomizationSelect(corrSource, corrRandom);
      applyCorrSource();
      updateCorrNote();

      // Sobol is base 2 in every dimension, so it refuses every even leap and
      // nothing else; Halton's answer depends on its whole prime list.
      corrLeap.refresh();
      scheduleCorrelate();
    });

    corrRandom.addEventListener("change", () => {
      updateCorrNote();
      scheduleCorrelate();
    });

    for (const input of [convDims, convSkip]) {
      input.addEventListener("input", () => {
        syncOutputs();

        if (input === convDims) {
          convLeap.refresh();
        }
      });
    }

    integrandSelect.addEventListener("change", () => {
      applyIntegrand();
      resetSweep();
    });

    convSource.addEventListener("change", () => {
      fillRandomizationSelect(convSource, convRandom);
      applyConvSource();
      convLeap.refresh();
      resetSweep();
    });

    convRandom.addEventListener("change", resetSweep);

    for (const input of [discDims, discSkip]) {
      input.addEventListener("input", () => {
        syncOutputs();

        // The dimension count decides three things at once here: which leaps
        // are admissible, whether the metric is available at all, and how many
        // points it can afford. All three are Go's to answer.
        if (input === discDims) {
          discLeap.refresh();
          refreshMetrics();
        }

        resetDiscSweep();
      });
    }

    discMetric.addEventListener("change", () => {
      refreshMetrics();
      resetDiscSweep();
    });

    discSource.addEventListener("change", () => {
      fillRandomizationSelect(discSource, discRandom);
      applyDiscSource();
      discLeap.refresh();
      refreshMetrics();
      resetDiscSweep();
    });

    discRandom.addEventListener("change", resetDiscSweep);
    discSeed.addEventListener("change", resetDiscSweep);

    // Offered, never applied silently — the leap control's rule. The number on
    // screen has to be the number the measurement used.
    discSuggest.addEventListener("click", () => {
      const entry = currentMetric();

      if (!entry || entry.suggestedDims === null) {
        return;
      }

      discDims.value = String(entry.suggestedDims);
      syncOutputs();
      discLeap.refresh();
      refreshMetrics();
      resetDiscSweep();
    });

    discStart.addEventListener("click", () => {
      startDiscSweep().catch((err) => {
        console.error(err);
        setStatus(`discrepancy sweep failed: ${err && err.message}`, "error");
        stopSweep();
      });
    });

    discStop.addEventListener("click", stopSweep);

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
        drawDiscChart();
      }, 120);
    });

    Render.watchDPR(() => {
      Render.invalidateTheme();
      drawHeat();
      drawChart();
      drawDiscChart();
    });

    // The first verdicts, drawn once the controls carry their defaults.
    corrLeap.refresh();
    convLeap.refresh();
    discLeap.refresh();
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
    discSource.disabled = false;
    discRandom.disabled = false;
    startButton.disabled = false;

    // discStart is NOT enabled here. Whether the selected metric can run at
    // the dimension count on the slider is the library's answer, and
    // refreshMetrics has already asked it.
    setStatus("WASM ready", "ready");
    refreshCorrelation();
    drawChart();
    resetDiscSweep();
  }

  initWasm().catch((err) => {
    console.error(err);
    setStatus(
      "WebAssembly failed to load. Serve this page over HTTP — a file:// URL cannot fetch a .wasm — and check that qmc.wasm is sent with Content-Type: application/wasm.",
      "error",
    );
  });
})();
