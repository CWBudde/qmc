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
    return {cases,transitions:true};
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
