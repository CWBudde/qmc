import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createServer } from "node:http";
import { readFile, mkdtemp, rm } from "node:fs/promises";
import { resolve, extname } from "node:path";
import { tmpdir } from "node:os";
import { StringDecoder } from "node:string_decoder";

const started = Date.now();
const root = resolve(process.argv[2] || "dist");
const runtimeFixture = process.argv[3] ? resolve(process.argv[3]) : null;
let useFixture = false;
let failureMode = null;
const mime = {
  ".wasm": "application/wasm",
  ".js": "text/javascript",
  ".html": "text/html",
  ".css": "text/css",
  ".svg": "image/svg+xml",
};
const server = createServer(async (req, res) => {
  try {
    const path = resolve(
      root,
      "." + decodeURIComponent(new URL(req.url, "http://local").pathname),
    );
    if (!path.startsWith(root + "/")) throw new Error("path");
    if (
      (failureMode === "missing-wasm" && path.endsWith("/qmc.wasm")) ||
      (failureMode === "missing-runtime" && path.endsWith("/wasm_exec.js"))
    ) {
      res.writeHead(503);
      res.end("deliberate smoke-test asset failure");
      return;
    }
    if (failureMode === "corrupt-wasm" && path.endsWith("/qmc.wasm")) {
      res.writeHead(200, { "Content-Type": "application/wasm" });
      res.end("invalid WASM fixture");
      return;
    }
    const data = await readFile(
      useFixture && path.endsWith("/qmc.wasm") ? runtimeFixture : path,
    );
    res.writeHead(200, {
      "Content-Type": mime[extname(path)] || "application/octet-stream",
      "Content-Length": data.length,
      "Cross-Origin-Opener-Policy": "same-origin",
      "Cross-Origin-Embedder-Policy": "require-corp",
    });
    res.end(data);
  } catch {
    res.writeHead(404);
    res.end("missing");
  }
});
const profile = await mkdtemp(resolve(tmpdir(), "qmc-chrome-"));
let chrome;
let chromeExited = false;
let deadlineExceeded = false;
const decoder = new StringDecoder("utf8");
const pending = new Map();
let serial = 0,
  buffer = "";
const errors = [];
const requests = [];
const consoleErrors = [];
const resourceErrors = [];
const rejectPending = (error) => {
  for (const p of pending.values()) {
    clearTimeout(p.timer);
    p.reject(error);
  }
  pending.clear();
};
const budget = setTimeout(() => {
  deadlineExceeded = true;
  rejectPending(new Error("Browser deadline exceeded"));
  chrome?.kill("SIGKILL");
}, 120000);
try {
  await new Promise((ok, fail) => {
    const timer = setTimeout(
      () => fail(new Error("Local server startup deadline exceeded")),
      5000,
    );
    server.once("error", fail);
    server.listen(0, "127.0.0.1", () => {
      clearTimeout(timer);
      ok();
    });
  });
  chrome = spawn(
    process.env.CHROME_BIN || "google-chrome",
    [
      "--headless=new",
      "--no-sandbox",
      "--disable-dev-shm-usage",
      "--disable-background-networking",
      "--no-first-run",
      "--remote-debugging-pipe",
      `--user-data-dir=${profile}`,
    ],
    { stdio: ["ignore", "ignore", "pipe", "pipe", "pipe"] },
  );
  chrome.stderr.on("data", () => {});
  chrome.on("error", rejectPending);
  chrome.once("exit", () => {
    chromeExited = true;
    rejectPending(new Error("Chrome exited before the test finished"));
  });
  chrome.stdio[3].on("error", rejectPending);
  chrome.stdio[4].on("data", (chunk) => {
    buffer += decoder.write(chunk);
    let end;
    while ((end = buffer.indexOf("\0")) >= 0) {
      const packet = JSON.parse(buffer.slice(0, end));
      buffer = buffer.slice(end + 1);
      if (packet.id) {
        const p = pending.get(packet.id);
        if (!p) continue;
        pending.delete(packet.id);
        clearTimeout(p.timer);
        if (packet.error) p.reject(new Error(JSON.stringify(packet.error)));
        else p.resolve(packet.result);
      } else if (packet.method === "Runtime.exceptionThrown")
        errors.push(packet.params.exceptionDetails);
      else if (packet.method === "Network.requestWillBeSent")
        requests.push(packet.params.request.url);
      else if (
        packet.method === "Runtime.consoleAPICalled" &&
        packet.params.type === "error"
      )
        consoleErrors.push(
          packet.params.args.map((arg) => arg.value || arg.description),
        );
      else if (
        packet.method === "Log.entryAdded" &&
        packet.params.entry.level === "error"
      ) {
        const entry = packet.params.entry;
        const expected =
          entry.source === "network" &&
          ((failureMode === "missing-wasm" &&
            entry.url?.endsWith("/qmc.wasm")) ||
            (failureMode === "missing-runtime" &&
              entry.url?.endsWith("/wasm_exec.js")));
        if (!expected) resourceErrors.push(entry);
      }
    }
  });
  const send = (method, params = {}, sessionId) =>
    new Promise((resolve, reject) => {
      if (chromeExited || deadlineExceeded) {
        reject(new Error("Browser is no longer running within its deadline"));
        return;
      }
      const id = ++serial,
        timer = setTimeout(() => {
          pending.delete(id);
          reject(new Error(`CDP timeout: ${method}`));
        }, 20000);
      pending.set(id, { resolve, reject, timer });
      chrome.stdio[3].write(
        JSON.stringify({
          id,
          method,
          params,
          ...(sessionId ? { sessionId } : {}),
        }) + "\0",
      );
    });
  const { targetId } = await send("Target.createTarget", {
    url: "about:blank",
  });
  const { sessionId } = await send("Target.attachToTarget", {
    targetId,
    flatten: true,
  });
  await send("Runtime.enable", {}, sessionId);
  await send("Page.enable", {}, sessionId);
  await send("Network.enable", {}, sessionId);
  await send("Log.enable", {}, sessionId);
  const evaluate = async (expression) => {
    const r = await send(
      "Runtime.evaluate",
      { expression, returnByValue: true, awaitPromise: true },
      sessionId,
    );
    if (r.exceptionDetails) throw new Error(JSON.stringify(r.exceptionDetails));
    return r.result.value;
  };
  await send(
    "Page.navigate",
    { url: `http://127.0.0.1:${server.address().port}/analysis.html` },
    sessionId,
  );
  const deadline = Date.now() + 30000;
  while (
    !(await evaluate(
      'document.getElementById("rack")?.dataset.boot === "ready"',
    ))
  ) {
    assert(Date.now() < deadline, "WASM boot deadline exceeded");
    await new Promise((r) => setTimeout(r, 100));
  }
  const result = await evaluate(`(async () => {
    const el=id=>document.getElementById(id);
    const set=(id,value,event='change')=>{el(id).value=String(value);el(id).dispatchEvent(new Event(event,{bubbles:true}));};
    const check=(value,msg)=>{if(!value)throw new Error(msg);};
    const settle=()=>new Promise(r=>setTimeout(r,50));
    let cases=0;
    set('convDims',2,'input'); set('budget',4096);
    for (const [id,value,event] of [['convDims',4,'input'],['convSkip',20,'input'],['convSeed',8,'input'],['convLeap',11,'input'],['integrand','sum','change'],['budget',16384,'change'],['convRandom','scramble','change'],['convSource','sobol','change']]) {
      set('convLeap',1,'input');
      el('start').click(); check(el('convRows').children.length===1,'first result missing '+id);
      set(id,value,event); await settle();
      check(el('convRows').children.length===0,'stale convergence row after '+id);
      check(!el('start').disabled && el('stop').disabled,'stranded convergence controls '+id);
      check(el('convConfig').textContent.includes('No results'),'stale snapshot '+id); cases++;
    }
    set('discDims',2,'input'); set('discMetric','cl2');
    for(const [id,value,event] of [['discDims',3,'input'],['discSkip',8,'input'],['discSeed',12,'input'],['discLeap',11,'input'],['discRandom','scramble','change'],['discSource','sobol','change'],['discMetric','star','change']]) {
      set('discLeap',1,'input'); el('discStart').click();
      check(el('discRows').children.length===1,'first discrepancy result missing '+id);
      set(id,value,event); await settle();
      check(el('discRows').children.length===0,'stale discrepancy row after '+id);
      check(!el('discStart').disabled && el('discStop').disabled,'stranded discrepancy controls '+id); cases++;
    }
    set('discMetric','cl2'); el('start').click();
    check(!el('discStart').disabled,'cannot switch to discrepancy'); el('discStart').click();
    check(el('stop').disabled && !el('start').disabled,'superseded convergence controls');
    check(!el('discStop').disabled,'discrepancy not running');
    set('convSkip',22,'input'); check(!el('discStop').disabled,'inactive reset cancelled discrepancy');
    el('start').click(); check(el('discStop').disabled,'reverse panel switch');
    el('stop').click(); check(el('convRows').children.length>0,'Stop lost partial results');
    check(el('convConfig').textContent.includes('skip 22'),'partial snapshot wrong');
    el('start').click(); check(el('convRows').children.length===1,'restart mixed old results'); el('stop').click();
    el('discStart').click(); set('discMetric','star'); set('discDims',39,'input');
    await settle(); check(el('discStart').disabled && el('discStop').disabled,'unavailable metric controls');
    check(el('discRows').children.length===0,'unavailable metric retained old results');
    set('discDims',2,'input'); check(!el('discStart').disabled,'available metric did not recover');
    // Independent composite Simpson reference for the separable Gaussian.
    let integral=0;
    const steps=10000, f=x=>Math.exp(-((x-0.5)**2)/(2*0.35**2));
    for(let i=0;i<=steps;i++)integral+=(i===0||i===steps?1:i%2?4:2)*f(i/steps);
    integral/=3*steps;
    set('integrand','gaussian');
    for(const dims of [1,4,32]) {
      set('convDims',dims,'input');
      const exact=integral**dims;
      const metadata=qmc.info({dims}).integrands.find(s=>s.key==='gaussian');
      const result=qmc.converge({integrand:'gaussian',dims,n:1});
      check(Math.abs(result.exact/exact-1)<1e-12,'Gaussian reference '+dims);
      check(metadata.exact===result.exact && metadata.dims===dims,'dimension-aware metadata '+dims);
      check(el('integrandNote').textContent.includes(Render.compact(exact)),'stale Gaussian note '+dims);
      el('start').click(); el('stop').click();
      check(el('tExact').textContent===Render.compact(result.exact),'Gaussian readout '+dims);
      check(el('integrandNote').textContent.includes(el('tExact').textContent),'note/readout disagreement '+dims);
    }
    const info=qmc.info();
    const plain=source=>info.sources.find(s=>s.key===source).randomizations.find(r=>r.key==='none').description;
    check(plain('halton')!==plain('sobol'),'identical source descriptions');
    check(plain('sobol').includes('base 2') && plain('halton').includes('High prime'),'source-specific explanation');
    check(!el('docReference').textContent.includes('five seeds'),'stale seed summary');
    // Typed output must agree with a fresh result, including malformed sinks.
    const request={dims:2,count:5,axisX:0,axisY:1,skip:0,seed:7};
    const reference=qmc.points(request).xy;
    const equal=(a,b)=>a instanceof Float32Array && a.length===b.length && Array.from(a).every((v,i)=>v===b[i]);
    const make=(size=40,offset=0)=>{const buffer=new ArrayBuffer(size);return {f32:new Float32Array(buffer,offset),u8:new Uint8Array(buffer,offset)}};
    let valid=make(80,8); valid.f32.fill(-7);
    let output=qmc.points({...request,out:{xy:valid}});
    check(equal(output.xy,reference),'matched nonzero-offset sink values');
    check(output.xy.buffer===valid.f32.buffer && output.xy.byteOffset===8 && output.xy.length===10,'matched sink was not reused');
    check(new Float32Array(valid.f32.buffer)[0]===0 && valid.f32[10]===-7,'sink wrote outside payload');
    const detached=make(); structuredClone(detached.f32.buffer,{transfer:[detached.f32.buffer]});
    const mismatches=[
      {f32:new Float32Array(10),u8:new Uint8Array(40)},
      {f32:new Float32Array(10),u8:new Uint8Array(1)},
      {f32:new Float32Array(1),u8:new Uint8Array(4)},
      {f32:new Float64Array(10),u8:new Uint8Array(80)},
      {f32:new Float32Array(10),u8:new Uint8ClampedArray(40)},
      {f32:new Float32Array(10),u8:[]},
      {f32:[],u8:new Uint8Array(40)},
      {f32:Object.create(Float32Array.prototype),u8:new Uint8Array(40)},
      detached,
      null,
    ];
    const shifted=make(80); shifted.u8=new Uint8Array(shifted.f32.buffer,4); mismatches.push(shifted);
    const short=make(); short.u8=new Uint8Array(short.f32.buffer,0,1); mismatches.push(short);
    for(const pair of mismatches) {
      output=qmc.points({...request,out:{xy:pair}});
      check(!output.error && equal(output.xy,reference),'malformed sink produced invalid floats');
      if(pair && ArrayBuffer.isView(pair.f32))check(output.xy.buffer!==pair.f32.buffer,'malformed sink reused');
    }
    check(typeof SharedArrayBuffer==='function','shared-buffer regression needs isolation headers');
    {
      const buffer=new SharedArrayBuffer(40), pair={f32:new Float32Array(buffer),u8:new Uint8Array(buffer)};
      output=qmc.points({...request,out:{xy:pair}});
      check(equal(output.xy,reference) && output.xy.buffer!==buffer,'shared sink reused');
    }
    const matrix=qmc.correlate({dims:3,count:10}).matrix, pair=make(36);
    output=qmc.correlate({dims:3,count:10,out:{matrix:pair}});
    check(equal(output.matrix,matrix) && output.matrix.buffer===pair.f32.buffer,'correlation buffer reuse');
    const names=['info','points','correlate','converge','digits','leaps','metrics','discrepancy'];
    const canonical=v=>ArrayBuffer.isView(v)||Array.isArray(v)?Array.from(v,canonical):v&&typeof v==='object'?Object.fromEntries(Object.keys(v).sort().map(k=>[k,canonical(v[k])])):v;
    for(const name of names) {
      const baseline=qmc[name]({});
      check(!baseline.error,'default object rejected by '+name);
      for(const opts of [undefined,null,false,1,'bad',[],{dims:NaN,count:Infinity,n:-Infinity,skip:null,leap:'bad',seed:NaN,source:null,randomization:false}]) {
        const result=qmc[name](opts);
        check(!result.error && JSON.stringify(canonical(result))===JSON.stringify(canonical(baseline)),'default/fallback mismatch '+name);
      }
    }
    check(typeof qmc.testExit==='undefined','fixture export in production build');
    const rejected=qmc.converge({integrand:'unknown'});
    check(rejected.error && rejected.panic===false,'request was not explicitly rejected');
    check(!qmc.converge({integrand:'sum',dims:2,n:4}).error,'valid request after rejection');
    return {cases,transitions:true,gaussianDimensions:[1,4,32],sourceDescriptions:true,typedArrayCases:mismatches.length+3};
  })()`);
  if (runtimeFixture) {
    useFixture = true;
    for (const page of ["analysis.html", "index.html"]) {
      await send(
        "Page.navigate",
        { url: `http://127.0.0.1:${server.address().port}/${page}` },
        sessionId,
      );
      const deadline = Date.now() + 30000;
      while (
        !(await evaluate(
          'document.getElementById("rack")?.dataset.boot === "ready" && typeof qmc?.testExit === "function"',
        ))
      ) {
        assert(Date.now() < deadline, "runtime fixture boot deadline");
        await new Promise((r) => setTimeout(r, 100));
      }
      const recovery = await evaluate(`(async () => {
        const el=id=>document.getElementById(id), check=(v,msg)=>{if(!v)throw new Error(msg)};
        const failure=qmc.testPanic();
        check(failure.panic && failure.error.includes('fixture request panic'),'fixture did not recover Go panic');
        check(!qmc.points({dims:2,count:2}).error,'valid request after recovered panic');
        const analysis=!!el('start'), method=analysis?'converge':'points', original=qmc[method];
        qmc[method]=()=>failure;
        if(analysis)el('start').click();else {el('count').value='10';el('count').dispatchEvent(new Event('input'));}
        await new Promise(r=>setTimeout(r,100));
        check(el('status').dataset.state==='error' && el('rack').dataset.boot==='ready','recovered panic marked instance terminal');
        check(el('reloadWasm').hidden,'reload required after recovered panic');
        qmc[method]=original;
        if(analysis){el('start').click();el('stop').click();check(el('convRows').children.length>0,'valid sweep after panic');}
        else {el('count').value='11';el('count').dispatchEvent(new Event('input'));await new Promise(r=>setTimeout(r,100));check(el('tPoints').textContent.includes('11'),'valid points after panic');}
        const exit=qmc.testExit;
        exit();
        await new Promise(r=>setTimeout(r,20));
        check(el('rack').dataset.boot==='failed' && !el('reloadWasm').hidden,'actual exit did not expose reload');
        check(Array.from(document.querySelectorAll('input,select,button')).every(e=>e.id==='reloadWasm'||e.disabled),'controls active after runtime exit');
        let threw=false; try {qmc.points({dims:2,count:2});}catch(e){threw=true;}
        check(threw,'fixture did not actually terminate Go');
        return {recoveredPanic:true,actualExit:true};
      })()`);
      assert.deepEqual(recovery, { recoveredPanic: true, actualExit: true });
      await evaluate('document.getElementById("reloadWasm").click(); true');
      const reloadDeadline = Date.now() + 30000;
      while (
        !(await evaluate(
          'document.getElementById("rack")?.dataset.boot === "ready"',
        ))
      ) {
        assert(Date.now() < reloadDeadline, "reload recovery deadline");
        await new Promise((r) => setTimeout(r, 100));
      }
      assert(
        await evaluate(
          'document.getElementById("reloadWasm").hidden && !qmc.points({dims:2,count:2}).error',
        ),
        "reload did not create a usable instance",
      );
    }
    useFixture = false;
    console.log("Both pages passed recovered-panic and actual Go-exit checks.");
  } else {
    console.log(
      "Runtime-exit checks require the optional test-fixture WASM argument.",
    );
  }
  // Switch sources and all their randomizations through the actual Point Lab UI.
  await send(
    "Page.navigate",
    { url: `http://127.0.0.1:${server.address().port}/index.html` },
    sessionId,
  );
  let pageDeadline = Date.now() + 30000;
  while (
    !(await evaluate(
      'document.getElementById("rack")?.dataset.boot === "ready"',
    ))
  ) {
    assert(Date.now() < pageDeadline, "Point Lab boot deadline");
    await new Promise((r) => setTimeout(r, 100));
  }
  const switching = await evaluate(`(async () => {
    const el=id=>document.getElementById(id), check=(v,msg)=>{if(!v)throw new Error(msg)};
    const set=(id,value,event='change')=>{el(id).value=String(value);el(id).dispatchEvent(new Event(event,{bubbles:true}));};
    set('count',10,'input'); set('dims',4,'input');
    for(const [source,randomizations] of [['halton',['none','scramble','nested']],['sobol',['none','shift','owen']]]) {
      set('source',source); await new Promise(r=>setTimeout(r,100));
      check(Array.from(el('randomization').options,o=>o.value).join(',')===randomizations.join(','),'incorrect randomization menu '+source);
      for(const randomization of randomizations) {
        set('randomization',randomization); await new Promise(r=>setTimeout(r,100));
        check(el('seqTitle').textContent=== (source==='halton'?'Halton':'Sobol'),'stale sequence title');
        check(el('tPoints').textContent==='10','point-count result mismatch');
        check(el('digitPanel').hidden===(source==='sobol'),'digit inspector visibility');
        check(el('tBaseXRow').hidden===(source==='sobol'),'prime-base visibility');
        check(el('status').dataset.state==='ready','source/randomization request failed');
      }
    }
    set('scrub',3,'input'); check(el('revealReadout').textContent.includes('3 / 10'),'scrub failed');
    el('play').click(); await new Promise(r=>setTimeout(r,100)); el('play').click();
    check(el('play').getAttribute('aria-pressed')==='false','pause failed');
    const originalRAF=window.requestAnimationFrame;
    let frames=0;
    window.requestAnimationFrame=callback=>originalRAF(time=>{frames++;callback(time)});
    await new Promise(r=>setTimeout(r,100)); const idle=frames;
    await new Promise(r=>setTimeout(r,150));
    check(frames===idle,'paused page still schedules animation frames');
    set('scrub',2,'input'); el('play').click();
    await new Promise(r=>setTimeout(r,150));
    check(frames>idle && el('play').getAttribute('aria-pressed')==='true','play did not resume animation');
    el('play').click(); const paused=frames;
    await new Promise(r=>setTimeout(r,150));
    check(frames===paused,'pause did not cancel pending animation');
    window.requestAnimationFrame=originalRAF;
    return true;
  })()`);
  assert(switching);
  await evaluate(
    `document.getElementById('scrub').value='2';document.getElementById('scrub').dispatchEvent(new Event('input'));document.getElementById('play').click();true`,
  );
  const background = await send("Target.createTarget", { url: "about:blank" });
  await send("Target.activateTarget", { targetId: background.targetId });
  await new Promise((r) => setTimeout(r, 100));
  assert(
    await evaluate(
      'document.hidden && document.getElementById("play").getAttribute("aria-pressed") === "false"',
    ),
    "background tab did not pause playback",
  );
  await send("Target.closeTarget", { targetId: background.targetId });
  await send("Target.activateTarget", { targetId });
  await send(
    "Emulation.setEmulatedMedia",
    { features: [{ name: "prefers-reduced-motion", value: "reduce" }] },
    sessionId,
  );
  await send(
    "Page.navigate",
    { url: `http://127.0.0.1:${server.address().port}/index.html` },
    sessionId,
  );
  pageDeadline = Date.now() + 30000;
  while (
    !(await evaluate(
      'document.getElementById("rack")?.dataset.boot === "ready"',
    ))
  ) {
    assert(Date.now() < pageDeadline, "reduced-motion boot deadline");
    await new Promise((r) => setTimeout(r, 100));
  }
  assert(
    await evaluate(`(() => {
    const el=id=>document.getElementById(id);
    el('scrub').value='2';el('scrub').dispatchEvent(new Event('input'));el('play').click();
    return el('play').getAttribute('aria-pressed')==='false' && el('scrub').value===el('scrub').max;
  })()`),
    "reduced-motion Play did not reveal statically",
  );
  await send("Emulation.setEmulatedMedia", { features: [] }, sessionId);
  // Deliberate loading failures are allowed only for their precise asset URLs.
  for (const page of ["index.html", "analysis.html"]) {
    for (const mode of ["missing-wasm", "corrupt-wasm", "missing-runtime"]) {
      failureMode = mode;
      await send(
        "Page.navigate",
        { url: `http://127.0.0.1:${server.address().port}/${page}` },
        sessionId,
      );
      pageDeadline = Date.now() + 30000;
      while (
        !(await evaluate(
          'document.getElementById("rack")?.dataset.boot === "failed"',
        ))
      ) {
        assert(
          Date.now() < pageDeadline,
          "loading failure was not reported: " + page + " " + mode,
        );
        await new Promise((r) => setTimeout(r, 100));
      }
      assert(
        await evaluate(
          '!document.getElementById("reloadWasm").hidden && document.getElementById("status").dataset.state === "error"',
        ),
        "missing loading-failure recovery action",
      );
      await new Promise((r) => setTimeout(r, 100));
      failureMode = null;
      await evaluate('document.getElementById("reloadWasm").click(); true');
      pageDeadline = Date.now() + 30000;
      while (
        !(await evaluate(
          'document.getElementById("rack")?.dataset.boot === "ready"',
        ))
      ) {
        assert(
          Date.now() < pageDeadline,
          "loading failure reload did not recover",
        );
        await new Promise((r) => setTimeout(r, 100));
      }
    }
  }
  assert.equal(errors.length, 0, JSON.stringify(errors));
  assert.deepEqual(consoleErrors, [], "unexpected browser console errors");
  assert.deepEqual(resourceErrors, [], "unexpected resource/browser errors");
  const thirdParty = requests.filter(
    (url) =>
      /^https?:/.test(url) &&
      new URL(url).origin !== `http://127.0.0.1:${server.address().port}`,
  );
  assert.deepEqual(thirdParty, [], "demo requested third-party resources");
  console.log(
    "Browser smoke passed:",
    JSON.stringify({
      ...result,
      pointLabSwitching: true,
      loadingFailureCases: 6,
      unexpectedErrors: 0,
      elapsedSeconds: (Date.now() - started) / 1000,
    }),
  );
} finally {
  clearTimeout(budget);
  if (chrome && chrome.exitCode === null) {
    chrome.kill("SIGTERM");
    await new Promise((r) => {
      chrome.once("exit", r);
      setTimeout(r, 2000);
    });
    if (chrome.exitCode === null) chrome.kill("SIGKILL");
  }
  server.closeAllConnections();
  await new Promise((r) => server.close(r));
  await rm(profile, { recursive: true, force: true });
}
