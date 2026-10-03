import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createServer } from "node:http";
import { readFile, mkdtemp, rm } from "node:fs/promises";
import { resolve, extname } from "node:path";
import { tmpdir } from "node:os";

const root = resolve(process.argv[2] || "dist");
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
    const data = await readFile(path);
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
const pending = new Map();
let serial = 0,
  buffer = "";
const errors = [];
const rejectPending = (error) => {
  for (const p of pending.values()) {
    clearTimeout(p.timer);
    p.reject(error);
  }
  pending.clear();
};
const budget = setTimeout(() => {
  rejectPending(new Error("Browser deadline exceeded"));
  chrome?.kill("SIGKILL");
}, 120000);
try {
  await new Promise((ok, fail) => {
    server.once("error", fail);
    server.listen(0, "127.0.0.1", ok);
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
  chrome.once("exit", () =>
    rejectPending(new Error("Chrome exited before the test finished")),
  );
  chrome.stdio[3].on("error", rejectPending);
  chrome.stdio[4].on("data", (chunk) => {
    buffer += chunk.toString();
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
    }
  });
  const send = (method, params = {}, sessionId) =>
    new Promise((resolve, reject) => {
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
    return {cases,transitions:true,gaussianDimensions:[1,4,32],sourceDescriptions:true,typedArrayCases:mismatches.length+3};
  })()`);
  assert.equal(errors.length, 0, JSON.stringify(errors));
  console.log("Browser sweep contracts passed:", JSON.stringify(result));
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
