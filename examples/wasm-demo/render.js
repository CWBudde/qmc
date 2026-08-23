/*
 * render.js — every pixel both qmc demo pages draw.
 *
 * This file owns colour. It reads the two point-set hues and the diverging
 * correlation ramp out of the page's CSS custom properties rather than
 * hardcoding them, so the canvas and the stylesheet cannot drift apart: change
 * --halton in style.css and the scatter glyphs, the legend swatch and the
 * convergence curve all follow.
 *
 * It owns no state about the data. Everything it draws is passed in, which is
 * what lets the Point Lab redraw any prefix of a point set on any frame and
 * lets the Bench redraw a partial sweep after every single call into Go.
 *
 * It also owns no quasi-Monte Carlo logic whatsoever. Not one coordinate, not
 * one correlation, not one integration error is computed here — those all
 * arrive from the Go library through globalThis.qmc. This file turns numbers
 * into pixels and nothing else.
 *
 * No modules — everything hangs off window.Render.
 */
(function () {
  "use strict";

  // --- theme -------------------------------------------------------------

  let varCache = null;

  function readVar(name, fallback) {
    if (!varCache) {
      varCache = new Map();
    }

    if (varCache.has(name)) {
      return varCache.get(name);
    }

    const raw = getComputedStyle(document.documentElement)
      .getPropertyValue(name)
      .trim();
    const value = raw || fallback;
    varCache.set(name, value);

    return value;
  }

  function invalidateTheme() {
    varCache = null;
  }

  // parseColor handles the #rgb / #rrggbb forms this stylesheet actually uses.
  // Anything else falls back to mid grey rather than throwing inside a draw
  // call, because a broken colour should not take the whole canvas down.
  function parseColor(input) {
    const value = String(input || "").trim();
    const short = /^#([0-9a-f])([0-9a-f])([0-9a-f])$/i.exec(value);

    if (short) {
      return [
        parseInt(short[1] + short[1], 16),
        parseInt(short[2] + short[2], 16),
        parseInt(short[3] + short[3], 16),
      ];
    }

    const long = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(value);

    if (long) {
      return [
        parseInt(long[1], 16),
        parseInt(long[2], 16),
        parseInt(long[3], 16),
      ];
    }

    return [128, 128, 128];
  }

  function mix(a, b, t) {
    return [
      Math.round(a[0] + (b[0] - a[0]) * t),
      Math.round(a[1] + (b[1] - a[1]) * t),
      Math.round(a[2] + (b[2] - a[2]) * t),
    ];
  }

  function alpha(color, a) {
    const [r, g, b] = parseColor(color);

    return `rgba(${r}, ${g}, ${b}, ${a})`;
  }

  function rgb(triplet) {
    return `rgb(${triplet[0]}, ${triplet[1]}, ${triplet[2]})`;
  }

  // --- canvas sizing -----------------------------------------------------

  function currentDPR() {
    return Math.min(window.devicePixelRatio || 1, 2);
  }

  // fitCanvas sizes the backing store to the CSS box times DPR and returns a
  // context already scaled, so every draw call below works in CSS pixels.
  function fitCanvas(canvas) {
    const dpr = currentDPR();
    const rect = canvas.getBoundingClientRect();
    const width = Math.max(1, Math.round(rect.width));
    const height = Math.max(1, Math.round(rect.height));

    // Round before comparing. canvas.width is an integer attribute, so at a
    // fractional device pixel ratio (1.5 on many Windows displays) the raw
    // product never equals it, the comparison would stay true on every frame,
    // and the backing store would be reallocated and cleared continuously.
    const backingWidth = Math.round(width * dpr);
    const backingHeight = Math.round(height * dpr);

    if (canvas.width !== backingWidth || canvas.height !== backingHeight) {
      canvas.width = backingWidth;
      canvas.height = backingHeight;
    }

    const ctx = canvas.getContext("2d");
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

    return { ctx, width, height };
  }

  function clear(ctx, width, height) {
    ctx.clearRect(0, 0, width, height);
  }

  // watchDPR fires onChange when the page moves between displays of different
  // pixel densities. A media query on the *current* ratio is the only portable
  // way to hear about it, so the listener re-arms itself each time.
  function watchDPR(onChange) {
    if (!window.matchMedia) {
      return;
    }

    const listen = () => {
      const query = window.matchMedia(`(resolution: ${currentDPR()}dppx)`);
      query.addEventListener(
        "change",
        () => {
          listen();
          onChange();
        },
        { once: true },
      );
    };

    listen();
  }

  // --- number formatting -------------------------------------------------

  function compact(value) {
    if (value === null || value === undefined || !isFinite(value)) {
      return "—";
    }

    const magnitude = Math.abs(value);

    if (magnitude === 0) {
      return "0";
    }

    if (magnitude < 1e-4 || magnitude >= 1e5) {
      return value.toExponential(2);
    }

    if (magnitude < 1) {
      return value.toFixed(4);
    }

    return value.toFixed(magnitude < 100 ? 3 : 1);
  }

  // decade renders a power of ten the way an axis wants it: 10^-3, not 0.001.
  function decade(exponent) {
    const digits = String(Math.abs(exponent))
      .split("")
      .map((d) => "⁰¹²³⁴⁵⁶⁷⁸⁹"[Number(d)])
      .join("");

    return `10${exponent < 0 ? "⁻" : ""}${digits}`;
  }

  // --- text and frames ---------------------------------------------------

  function monoFont(size) {
    return `${size}px "JetBrains Mono", ui-monospace, monospace`;
  }

  function label(ctx, text, x, y, align, color, size) {
    ctx.fillStyle = color || readVar("--dim", "#7f88b6");
    ctx.font = monoFont(size || 10);
    ctx.textAlign = align || "left";
    ctx.textBaseline = "middle";
    ctx.fillText(text, x, y);
  }

  function frame(ctx, x, y, width, height) {
    ctx.strokeStyle = readVar("--rule", "#232b4c");
    ctx.lineWidth = 1;
    ctx.strokeRect(x + 0.5, y + 0.5, width - 1, height - 1);
  }

  // --- scatter -----------------------------------------------------------

  const SCATTER_PAD = { top: 14, right: 14, bottom: 26, left: 34 };

  // scatterGeometry is exported so the page can hit-test a click against the
  // same square the points were drawn in, without duplicating the maths.
  function scatterGeometry(canvas) {
    const rect = canvas.getBoundingClientRect();
    const width = Math.max(1, Math.round(rect.width));
    const height = Math.max(1, Math.round(rect.height));
    const size = Math.max(
      1,
      Math.min(
        width - SCATTER_PAD.left - SCATTER_PAD.right,
        height - SCATTER_PAD.top - SCATTER_PAD.bottom,
      ),
    );

    return {
      left: SCATTER_PAD.left,
      top: SCATTER_PAD.top,
      size,
      width,
      height,
    };
  }

  // The unit square, gridded in tenths. The grid is what turns "the points
  // look spread out" into "there is exactly one point per cell", which is the
  // property a low-discrepancy sequence is actually claiming.
  function drawScatterGrid(ctx, geo) {
    ctx.save();
    ctx.strokeStyle = alpha(readVar("--rule", "#232b4c"), 0.75);
    ctx.lineWidth = 1;
    ctx.beginPath();

    for (let i = 1; i < 10; i += 1) {
      const offset = Math.round((geo.size * i) / 10) + 0.5;
      ctx.moveTo(geo.left + offset, geo.top);
      ctx.lineTo(geo.left + offset, geo.top + geo.size);
      ctx.moveTo(geo.left, geo.top + offset);
      ctx.lineTo(geo.left + geo.size, geo.top + offset);
    }

    ctx.stroke();
    ctx.restore();
  }

  function crossPath(ctx, x, y, radius) {
    ctx.moveTo(x - radius, y - radius);
    ctx.lineTo(x + radius, y + radius);
    ctx.moveTo(x + radius, y - radius);
    ctx.lineTo(x - radius, y + radius);
  }

  // drawScatter paints one point set into the unit square.
  //
  // Halton is a filled circle, pseudo-random is a diagonal cross. Shape
  // carries the same information as hue, so the two panels stay tellable apart
  // in greyscale, in print, and for a reader who cannot separate teal from
  // amber. Every page that shows both uses the same pairing.
  function drawScatter(canvas, options) {
    const { ctx, width, height } = fitCanvas(canvas);
    const opts = options || {};
    const geo = scatterGeometry(canvas);

    clear(ctx, width, height);
    drawScatterGrid(ctx, geo);

    const xy = opts.xy;
    const total = Math.max(0, opts.count || 0);
    const reveal = Math.max(
      0,
      Math.min(total, opts.reveal === undefined ? total : opts.reveal),
    );
    const color = opts.color || readVar("--halton", "#46e0c8");
    const glyph = opts.glyph === "cross" ? "cross" : "circle";
    const radius = reveal > 4000 ? 1.4 : reveal > 1200 ? 2.1 : 3;

    if (!xy || !total) {
      label(ctx, "no points", width / 2, height / 2, "center");
      frame(ctx, geo.left, geo.top, geo.size, geo.size);

      return geo;
    }

    const toX = (v) => geo.left + v * geo.size;
    // Canvas y grows downward; the unit square's y = 1 is the top edge.
    const toY = (v) => geo.top + geo.size - v * geo.size;

    ctx.save();

    if (glyph === "circle") {
      ctx.fillStyle = alpha(color, 0.85);
      ctx.beginPath();

      for (let i = 0; i < reveal; i += 1) {
        const x = toX(xy[i * 2]);
        const y = toY(xy[i * 2 + 1]);

        if (!isFinite(x) || !isFinite(y)) {
          continue;
        }

        ctx.moveTo(x + radius, y);
        ctx.arc(x, y, radius, 0, Math.PI * 2);
      }

      ctx.fill();
    } else {
      ctx.strokeStyle = alpha(color, 0.85);
      ctx.lineWidth = radius > 2 ? 1.4 : 1;
      ctx.beginPath();

      for (let i = 0; i < reveal; i += 1) {
        const x = toX(xy[i * 2]);
        const y = toY(xy[i * 2 + 1]);

        if (!isFinite(x) || !isFinite(y)) {
          continue;
        }

        crossPath(ctx, x, y, radius);
      }

      ctx.stroke();
    }

    ctx.restore();

    // The scrub head: the point most recently revealed, ringed so the *order*
    // of the sequence is visible and not only its final cloud.
    if (reveal > 0 && reveal < total) {
      const x = toX(xy[(reveal - 1) * 2]);
      const y = toY(xy[(reveal - 1) * 2 + 1]);

      ctx.save();
      ctx.strokeStyle = readVar("--probe", "#8b7dff");
      ctx.lineWidth = 1.4;
      ctx.beginPath();
      ctx.arc(x, y, radius + 5, 0, Math.PI * 2);
      ctx.stroke();
      ctx.restore();
    }

    // The selected point, the one the digit inspector is expanding. Outlined
    // rather than filled so it never hides the glyph underneath it.
    const selected = opts.selected;

    if (typeof selected === "number" && selected >= 0 && selected < reveal) {
      const x = toX(xy[selected * 2]);
      const y = toY(xy[selected * 2 + 1]);
      const markColor = readVar("--mark", "#ff5d8f");

      ctx.save();
      ctx.strokeStyle = markColor;
      ctx.lineWidth = 1.6;
      ctx.beginPath();
      ctx.arc(x, y, radius + 4, 0, Math.PI * 2);
      ctx.stroke();

      ctx.strokeStyle = alpha(markColor, 0.4);
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(geo.left, y + 0.5);
      ctx.lineTo(geo.left + geo.size, y + 0.5);
      ctx.moveTo(x + 0.5, geo.top);
      ctx.lineTo(x + 0.5, geo.top + geo.size);
      ctx.stroke();
      ctx.restore();
    }

    frame(ctx, geo.left, geo.top, geo.size, geo.size);

    label(ctx, "0", geo.left, geo.top + geo.size + 12, "center");
    label(ctx, "1", geo.left + geo.size, geo.top + geo.size + 12, "center");
    label(ctx, "1", geo.left - 8, geo.top, "right");
    label(ctx, "0", geo.left - 8, geo.top + geo.size, "right");

    if (opts.xLabel) {
      label(
        ctx,
        opts.xLabel,
        geo.left + geo.size / 2,
        geo.top + geo.size + 12,
        "center",
      );
    }

    if (opts.yLabel) {
      ctx.save();
      ctx.translate(geo.left - 20, geo.top + geo.size / 2);
      ctx.rotate(-Math.PI / 2);
      label(ctx, opts.yLabel, 0, 0, "center");
      ctx.restore();
    }

    return geo;
  }

  // nearestPoint maps a click in CSS pixels back to a point index. It searches
  // only the revealed prefix, so clicking never selects a point the viewer
  // cannot see.
  function nearestPoint(geo, xy, reveal, px, py, tolerance) {
    if (!xy || !reveal) {
      return -1;
    }

    const limit = tolerance === undefined ? 14 : tolerance;
    let best = -1;
    let bestDistance = limit * limit;

    for (let i = 0; i < reveal; i += 1) {
      const x = geo.left + xy[i * 2] * geo.size;
      const y = geo.top + geo.size - xy[i * 2 + 1] * geo.size;
      const dx = x - px;
      const dy = y - py;
      const distance = dx * dx + dy * dy;

      if (distance < bestDistance) {
        bestDistance = distance;
        best = i;
      }
    }

    return best;
  }

  // --- correlation heatmap -----------------------------------------------

  const HEAT_PAD = { top: 10, right: 10, bottom: 30, left: 34 };

  // corrColor maps a signed correlation onto the diverging ramp. The magnitude
  // is eased (^0.65) because the values that matter here — a 0.14 worst pair
  // against a 0.81 one — live at the low end, and a linear ramp paints them
  // both as ground.
  function corrColor(value, ramp) {
    const t = Math.max(0, Math.min(1, Math.pow(Math.abs(value), 0.65)));

    return rgb(mix(ramp.zero, value < 0 ? ramp.neg : ramp.pos, t));
  }

  function corrRamp() {
    return {
      zero: parseColor(readVar("--corr-zero", "#0d1230")),
      neg: parseColor(readVar("--corr-neg", "#4f8cff")),
      pos: parseColor(readVar("--corr-pos", "#ff5d8f")),
    };
  }

  // drawHeatmap paints a dims x dims correlation matrix and returns the
  // geometry the page needs to translate a pointer position into (i, j).
  function drawHeatmap(canvas, options) {
    const { ctx, width, height } = fitCanvas(canvas);
    const opts = options || {};
    const dims = opts.dims || 0;
    const matrix = opts.matrix;

    clear(ctx, width, height);

    const plot = Math.max(
      1,
      Math.min(
        width - HEAT_PAD.left - HEAT_PAD.right,
        height - HEAT_PAD.top - HEAT_PAD.bottom,
      ),
    );
    const geo = {
      left: HEAT_PAD.left,
      top: HEAT_PAD.top,
      size: plot,
      dims,
      cell: dims ? plot / dims : 0,
    };

    if (!matrix || !dims) {
      label(ctx, "no matrix", width / 2, height / 2, "center");

      return geo;
    }

    const ramp = corrRamp();

    // Cells are drawn from rounded edges rather than a rounded width, so
    // neighbouring cells share an exact boundary and the grid has no seams.
    for (let row = 0; row < dims; row += 1) {
      const y0 = geo.top + Math.round((row * plot) / dims);
      const y1 = geo.top + Math.round(((row + 1) * plot) / dims);

      for (let col = 0; col < dims; col += 1) {
        const x0 = geo.left + Math.round((col * plot) / dims);
        const x1 = geo.left + Math.round(((col + 1) * plot) / dims);
        const value = matrix[row * dims + col];

        ctx.fillStyle = corrColor(isFinite(value) ? value : 0, ramp);
        ctx.fillRect(x0, y0, Math.max(1, x1 - x0), Math.max(1, y1 - y0));
      }
    }

    // The worst adjacent pair, called out on the map itself. This is the cell
    // the library's README quotes a number for, so it should not have to be
    // hunted for by eye.
    const worst = opts.worstPair;

    if (
      Array.isArray(worst) &&
      worst.length === 2 &&
      worst[0] >= 0 &&
      worst[0] < dims &&
      worst[1] >= 0 &&
      worst[1] < dims
    ) {
      ctx.save();
      ctx.strokeStyle = readVar("--lume", "#eaedff");
      ctx.lineWidth = 1.5;

      for (const [row, col] of [
        [worst[0], worst[1]],
        [worst[1], worst[0]],
      ]) {
        const x0 = geo.left + (col * plot) / dims;
        const y0 = geo.top + (row * plot) / dims;
        ctx.strokeRect(
          x0 - 0.5,
          y0 - 0.5,
          Math.max(3, geo.cell) + 1,
          Math.max(3, geo.cell) + 1,
        );
      }

      ctx.restore();
    }

    const hover = opts.hover;

    if (
      hover &&
      hover.i >= 0 &&
      hover.i < dims &&
      hover.j >= 0 &&
      hover.j < dims
    ) {
      ctx.save();
      ctx.strokeStyle = readVar("--mark", "#ff5d8f");
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(geo.left, geo.top + (hover.i + 0.5) * geo.cell);
      ctx.lineTo(geo.left + plot, geo.top + (hover.i + 0.5) * geo.cell);
      ctx.moveTo(geo.left + (hover.j + 0.5) * geo.cell, geo.top);
      ctx.lineTo(geo.left + (hover.j + 0.5) * geo.cell, geo.top + plot);
      ctx.stroke();
      ctx.restore();
    }

    frame(ctx, geo.left, geo.top, plot, plot);

    // Dimension ticks. Every fifth index, plus the last one, so the axes can
    // be read without counting cells.
    const step = dims > 32 ? 8 : dims > 16 ? 4 : 2;

    for (let i = 0; i < dims; i += step) {
      const offset = (i + 0.5) * geo.cell;
      label(ctx, String(i), geo.left + offset, geo.top + plot + 11, "center");
      label(ctx, String(i), geo.left - 6, geo.top + offset, "right");
    }

    label(
      ctx,
      "dimension j",
      geo.left + plot / 2,
      geo.top + plot + 23,
      "center",
      readVar("--dim", "#7f88b6"),
      10,
    );

    return geo;
  }

  // drawHeatLegend draws the diverging scale under the map, labelled at -1, 0
  // and +1, plus the eased mid-points so the non-linear ramp is not a lie.
  function drawHeatLegend(canvas) {
    const { ctx, width, height } = fitCanvas(canvas);

    clear(ctx, width, height);

    const ramp = corrRamp();
    const pad = 34;
    const barW = Math.max(1, width - pad * 2);
    const barH = 12;
    const top = 6;

    for (let x = 0; x < barW; x += 1) {
      const value = (x / (barW - 1)) * 2 - 1;
      ctx.fillStyle = corrColor(value, ramp);
      ctx.fillRect(pad + x, top, 1, barH);
    }

    frame(ctx, pad, top, barW, barH);

    for (const tick of [-1, -0.5, 0, 0.5, 1]) {
      const x = pad + ((tick + 1) / 2) * barW;

      ctx.save();
      ctx.strokeStyle = readVar("--rule", "#232b4c");
      ctx.beginPath();
      ctx.moveTo(Math.round(x) + 0.5, top + barH);
      ctx.lineTo(Math.round(x) + 0.5, top + barH + 4);
      ctx.stroke();
      ctx.restore();

      label(ctx, tick.toFixed(1), x, top + barH + 13, "center");
    }

    label(
      ctx,
      "correlation r  ·  colour magnitude is eased, not linear",
      width / 2,
      top + barH + 28,
      "center",
    );
  }

  // --- log–log convergence chart -----------------------------------------

  const CHART_PAD = { top: 16, right: 22, bottom: 40, left: 62 };

  function logBounds(values, minSpan) {
    let low = Infinity;
    let high = -Infinity;

    for (const value of values) {
      if (!isFinite(value) || value <= 0) {
        continue;
      }

      const l = Math.log10(value);
      low = Math.min(low, l);
      high = Math.max(high, l);
    }

    if (!isFinite(low) || !isFinite(high)) {
      return null;
    }

    low = Math.floor(low);
    high = Math.ceil(high);

    while (high - low < (minSpan || 1)) {
      high += 1;
    }

    return { low, high };
  }

  // drawLogLog is the convergence chart: absolute integration error against N,
  // both axes logarithmic, which is the only pair of axes on which "QMC beats
  // Monte Carlo" is a statement you can check rather than take on faith. On
  // log–log a power law is a straight line, so the reference slopes for 1/N
  // and 1/sqrt(N) turn the comparison into reading which line each series runs
  // parallel to.
  //
  // series: [{points: [{x, y}], color, glyph: "circle"|"cross"|"none", dash, width}]
  // refs:   [{slope, label, anchor: {x, y}, color}]
  // opts:   {xLabel, yLabel, empty, refs, yMinSpan}
  function drawLogLog(canvas, series, options) {
    const { ctx, width, height } = fitCanvas(canvas);
    const opts = options || {};

    clear(ctx, width, height);

    const plotW = Math.max(1, width - CHART_PAD.left - CHART_PAD.right);
    const plotH = Math.max(1, height - CHART_PAD.top - CHART_PAD.bottom);
    const live = (series || []).filter(
      (s) => s && s.points && s.points.length > 0,
    );

    const xs = [];
    const ys = [];

    for (const s of live) {
      for (const p of s.points) {
        xs.push(p.x);
        ys.push(p.y);
      }
    }

    const xb = logBounds(xs, 1);

    // Two decades is right for an error curve that falls by three or four over
    // a sweep, and wrong for a star discrepancy sweep that lives inside one:
    // forcing the extra decade there squashes the whole picture into the top
    // half of the plot. yMinSpan lets a caller that knows its range say so.
    const yb = logBounds(ys, opts.yMinSpan === undefined ? 2 : opts.yMinSpan);

    if (!xb || !yb) {
      frame(ctx, CHART_PAD.left, CHART_PAD.top, plotW, plotH);
      label(
        ctx,
        opts.empty || "press start to sweep N",
        CHART_PAD.left + plotW / 2,
        CHART_PAD.top + plotH / 2,
        "center",
      );

      return;
    }

    const toX = (x) =>
      CHART_PAD.left + ((Math.log10(x) - xb.low) / (xb.high - xb.low)) * plotW;
    const toY = (y) =>
      CHART_PAD.top +
      plotH -
      ((Math.log10(y) - yb.low) / (yb.high - yb.low)) * plotH;

    // decade grid
    ctx.save();
    ctx.strokeStyle = alpha(readVar("--rule", "#232b4c"), 0.85);
    ctx.lineWidth = 1;
    ctx.beginPath();

    for (let e = yb.low; e <= yb.high; e += 1) {
      const y = Math.round(toY(Math.pow(10, e))) + 0.5;
      ctx.moveTo(CHART_PAD.left, y);
      ctx.lineTo(CHART_PAD.left + plotW, y);
    }

    for (let e = xb.low; e <= xb.high; e += 1) {
      const x = Math.round(toX(Math.pow(10, e))) + 0.5;
      ctx.moveTo(x, CHART_PAD.top);
      ctx.lineTo(x, CHART_PAD.top + plotH);
    }

    ctx.stroke();
    ctx.restore();

    for (let e = yb.low; e <= yb.high; e += 1) {
      label(ctx, decade(e), CHART_PAD.left - 8, toY(Math.pow(10, e)), "right");
    }

    for (let e = xb.low; e <= xb.high; e += 1) {
      label(
        ctx,
        decade(e),
        toX(Math.pow(10, e)),
        CHART_PAD.top + plotH + 12,
        "center",
      );
    }

    // reference slopes, drawn under the data
    for (const ref of opts.refs || []) {
      if (!ref || !ref.anchor || !(ref.anchor.y > 0)) {
        continue;
      }

      const x0 = Math.pow(10, xb.low);
      const x1 = Math.pow(10, xb.high);
      const at = (x) => ref.anchor.y * Math.pow(x / ref.anchor.x, ref.slope);
      const color = ref.color || readVar("--mark", "#ff5d8f");

      ctx.save();
      ctx.strokeStyle = alpha(color, 0.7);
      ctx.lineWidth = 1;
      ctx.setLineDash([2, 4]);
      ctx.beginPath();
      ctx.moveTo(toX(x0), toY(at(x0)));
      ctx.lineTo(toX(x1), toY(at(x1)));
      ctx.stroke();
      ctx.restore();

      if (ref.label) {
        const y = toY(at(x1));

        if (y > CHART_PAD.top + 6 && y < CHART_PAD.top + plotH - 6) {
          label(
            ctx,
            ref.label,
            CHART_PAD.left + plotW - 4,
            y - 8,
            "right",
            alpha(color, 0.85),
          );
        }
      }
    }

    // series
    for (const s of live) {
      const color = s.color || readVar("--lume", "#eaedff");
      const points = s.points;

      ctx.save();
      ctx.strokeStyle = color;
      ctx.lineWidth = s.width || 1.8;

      if (s.dash) {
        ctx.setLineDash(s.dash);
      }

      ctx.beginPath();

      let started = false;

      for (const p of points) {
        if (!(p.x > 0) || !(p.y > 0)) {
          continue;
        }

        const x = toX(p.x);
        const y = toY(p.y);

        if (started) {
          ctx.lineTo(x, y);
        } else {
          ctx.moveTo(x, y);
          started = true;
        }
      }

      ctx.stroke();
      ctx.restore();

      // Markers repeat the series identity as a shape, so the two curves stay
      // distinguishable where they cross and in greyscale.
      ctx.save();
      ctx.setLineDash([]);

      if (s.glyph === "none") {
        // A reference curve, not a measurement: no markers, because there is
        // nothing at those x values that was actually computed.
        ctx.restore();

        continue;
      }

      if (s.glyph === "cross") {
        ctx.strokeStyle = color;
        ctx.lineWidth = 1.4;
        ctx.beginPath();

        for (const p of points) {
          if (!(p.x > 0) || !(p.y > 0)) {
            continue;
          }

          crossPath(ctx, toX(p.x), toY(p.y), 3.4);
        }

        ctx.stroke();
      } else {
        ctx.fillStyle = color;
        ctx.beginPath();

        for (const p of points) {
          if (!(p.x > 0) || !(p.y > 0)) {
            continue;
          }

          const x = toX(p.x);
          const y = toY(p.y);
          ctx.moveTo(x + 3, y);
          ctx.arc(x, y, 3, 0, Math.PI * 2);
        }

        ctx.fill();
      }

      ctx.restore();
    }

    frame(ctx, CHART_PAD.left, CHART_PAD.top, plotW, plotH);

    label(
      ctx,
      opts.xLabel || "N — points drawn",
      CHART_PAD.left + plotW / 2,
      CHART_PAD.top + plotH + 27,
      "center",
    );

    ctx.save();
    ctx.translate(14, CHART_PAD.top + plotH / 2);
    ctx.rotate(-Math.PI / 2);
    label(ctx, opts.yLabel || "absolute error", 0, 0, "center");
    ctx.restore();
  }

  // --- boot ring ---------------------------------------------------------

  function ring(element, progress) {
    if (element) {
      element.style.setProperty("--boot-progress", String(progress));
    }
  }

  window.Render = {
    readVar,
    invalidateTheme,
    parseColor,
    alpha,
    mix,
    fitCanvas,
    clear,
    compact,
    decade,
    watchDPR,
    drawScatter,
    scatterGeometry,
    nearestPoint,
    drawHeatmap,
    drawHeatLegend,
    corrColor,
    corrRamp,
    drawLogLog,
    ring,
  };
})();
