// 아주 작은 CDP 클라이언트 — 설치 없이 Node 22 의 내장 WebSocket 으로 크롬을 몬다.
//
//   const { open } = require("./cdp");
//   const page = await open(9333);          // 새 탭 하나
//   await page.goto(url);                   // 표가 뜰 때까지 기다린다
//   await page.eval("1+1");                 // 페이지 안에서 식을 돌려 값을 받는다
//
// 대화상자(confirm·beforeunload)는 늘 「확인」으로 닫는다. 페이지 콘솔·예외는 page.logs 에 쌓인다.
"use strict";

const MOD = { alt: 1, ctrl: 2, meta: 4, shift: 8 };

// waitVersion 은 크롬이 디버깅 포트를 열 때까지 기다린다. 못 열면 null.
async function waitVersion(port, ms) {
  const t0 = Date.now();
  while (Date.now() - t0 < ms) {
    try {
      const res = await fetch(`http://127.0.0.1:${port}/json/version`);
      if (res.ok) return await res.json();
    } catch (e) { /* 아직 안 떴다 */ }
    await new Promise((r) => setTimeout(r, 200));
  }
  return null;
}

async function open(port, { connectMs = 15000, width = 1400, height = 900 } = {}) {
  const ver = await waitVersion(port, connectMs);
  if (!ver) {
    const err = new Error(`크롬 디버깅 포트 ${port} 에 못 붙었다`);
    err.environment = true; // 코드가 아니라 기계 문제 — run.ps1 이 종료 2 로 낸다
    throw err;
  }
  const ws = new WebSocket(ver.webSocketDebuggerUrl);
  await new Promise((ok, bad) => { ws.onopen = ok; ws.onerror = bad; });

  let seq = 0;
  const waiting = new Map();
  const listeners = [];
  ws.onmessage = (m) => {
    const msg = JSON.parse(m.data);
    if (msg.id && waiting.has(msg.id)) {
      const { ok, bad } = waiting.get(msg.id);
      waiting.delete(msg.id);
      if (msg.error) bad(new Error(JSON.stringify(msg.error)));
      else ok(msg.result);
    } else if (msg.method) {
      listeners.forEach((f) => f(msg));
    }
  };
  const raw = (method, params = {}, sessionId) => new Promise((ok, bad) => {
    const id = ++seq;
    waiting.set(id, { ok, bad });
    ws.send(JSON.stringify({ id, method, params, sessionId }));
  });

  const { targetId } = await raw("Target.createTarget", { url: "about:blank" });
  const { sessionId } = await raw("Target.attachToTarget", { targetId, flatten: true });
  const send = (method, params) => raw(method, params, sessionId);
  await send("Page.enable");
  await send("Runtime.enable");

  const logs = [];
  listeners.push((msg) => {
    if (msg.sessionId !== sessionId) return;
    if (msg.method === "Runtime.consoleAPICalled") {
      logs.push(msg.params.type + ": " + msg.params.args.map((a) => a.value ?? a.description).join(" "));
    }
    if (msg.method === "Page.javascriptDialogOpening") {
      logs.push("DIALOG: " + msg.params.type + " " + msg.params.message);
      send("Page.handleJavaScriptDialog", { accept: true }).catch(() => {});
    }
    if (msg.method === "Runtime.exceptionThrown") {
      const d = msg.params.exceptionDetails;
      logs.push("EXC: " + ((d.exception && d.exception.description) || d.text));
    }
  });
  await send("Emulation.setDeviceMetricsOverride", { width, height, deviceScaleFactor: 1, mobile: false });

  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const page = {
    send, logs, sleep, MOD,

    async eval(expr) {
      const r = await send("Runtime.evaluate", { expression: expr, awaitPromise: true, returnByValue: true });
      if (r.exceptionDetails) {
        const d = r.exceptionDetails;
        throw new Error("eval: " + ((d.exception && d.exception.description) || d.text));
      }
      return r.result.value;
    },

    // goto 는 주소를 열고 표에 행이 들어올 때까지 기다린다. 걸린 ms 를 돌려준다.
    async goto(url, ms = 20000) {
      const t0 = Date.now();
      await send("Page.navigate", { url });
      await page.waitFor("typeof state !== 'undefined' && state.grid && state.grid.getDataCount() > 0", ms);
      return Date.now() - t0;
    },

    async waitFor(expr, ms = 10000) {
      const t0 = Date.now();
      while (Date.now() - t0 < ms) {
        try {
          if (await page.eval(`!!(${expr})`)) return Date.now() - t0;
        } catch (e) { /* 페이지가 아직 안 떴다 */ }
        await sleep(100);
      }
      throw new Error("시간 넘김: " + expr);
    },

    async click(x, y, count = 1, modifiers = 0) {
      await send("Input.dispatchMouseEvent", { type: "mouseMoved", x, y, modifiers });
      for (let c = 1; c <= count; c++) {
        await send("Input.dispatchMouseEvent", { type: "mousePressed", x, y, button: "left", clickCount: c, modifiers });
        await send("Input.dispatchMouseEvent", { type: "mouseReleased", x, y, button: "left", clickCount: c, modifiers });
      }
    },

    async key(key, code, keyCode, modifiers = 0) {
      await send("Input.dispatchKeyEvent", { type: "rawKeyDown", key, code, windowsVirtualKeyCode: keyCode, modifiers });
      await send("Input.dispatchKeyEvent", { type: "keyUp", key, code, windowsVirtualKeyCode: keyCode, modifiers });
    },

    async type(text) {
      await send("Input.insertText", { text });
    },

    async shot(file) {
      const { data } = await send("Page.captureScreenshot", { format: "png" });
      require("fs").writeFileSync(file, Buffer.from(data, "base64"));
    },

    close: () => ws.close(),
  };
  return page;
}

module.exports = { open, MOD };
