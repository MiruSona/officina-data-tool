// U6 시나리오 — 크롬 headless 에서 사람처럼 표를 고치고 저장한 뒤 git diff --numstat 으로 판정한다.
//
//   node u6.js --url <serve 주소> --data <데이터 폴더(git 저장소)> --cdp <크롬 디버깅 포트>
//              --out <결과 폴더> --serve-pid <datatool serve PID> [--exe <datatool.exe>] [--gen <C# 폴더>]
//
// K1~K6 · X2 는 스키마·Enum 탭이다 (설계 DataTool/Docs/Design/2026-10-04-스키마Enum편집설계.md 5장).
// --gen 은 데이터 폴더 .datatool.json 의 gen 칸이 가리키는 폴더다 — K6 이 C# 이 거기 생겼는지 본다.
//
// 시나리오마다 데이터 폴더를 첫 커밋으로 되돌리고(git reset --hard) 페이지를 새로 연다.
// 결과는 <out>/u6-results.json, 그림은 <out>/*.png. 종료 0 전부 통과 · 1 하나라도 실패 · 2 크롬에 못 붙음.
// 시나리오 표는 설계 DataTool/Docs/Design/2026-10-03-Unity검증과웹개선설계.md 7장.
"use strict";

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");
const { open, MOD } = require("./cdp");

function args() {
  const a = {};
  const rest = process.argv.slice(2);
  for (let i = 0; i < rest.length; i += 2) a[rest[i].replace(/^--/, "")] = rest[i + 1];
  for (const need of ["url", "data", "cdp", "out", "serve-pid"]) {
    if (!a[need]) {
      console.error(`--${need} 가 없다`);
      process.exit(2);
    }
  }
  return a;
}
const A = args();
fs.mkdirSync(A.out, { recursive: true });

/* git · 판정 도우미 ---------------------------------------------------- */

const git = (...g) => execFileSync("git", g, { cwd: A.data, encoding: "utf8" });
const BASE = git("rev-parse", "HEAD").trim();
const resetData = (to = BASE) => { git("reset", "-q", "--hard", to); git("clean", "-q", "-fd"); };

const numstat = () => git("diff", "--numstat").trim().split("\n").filter(Boolean)
  .map((l) => { const [a, d, f] = l.split("\t"); return { f, a: +a, d: +d }; });
const none = (ns) => ns.length === 0;
const only = (f, a, d) => (ns) => ns.length === 1 && ns[0].f === f && ns[0].a === a && ns[0].d === d;
const fmtNs = (ns) => (ns.length ? ns.map((n) => `${n.f} ${n.a} ${n.d}`).join(", ") : "없음");
const readData = (name) => fs.readFileSync(path.join(A.data, name), "utf8");

const results = [];
function record(id, title, expect, pass, ns, note) {
  results.push({ id, title, expect, actual: ns === null ? "—" : fmtNs(ns), pass, note: note || "" });
  console.log(`${pass ? "PASS" : "FAIL"} ${id} ${title} :: 기대 ${expect} :: 실제 ${ns === null ? "—" : fmtNs(ns)} :: ${note || ""}`);
}

/* 페이지 도우미 -------------------------------------------------------- */

let page;
const J = JSON.stringify;

async function rectOf(js) {
  return page.eval(`(async()=>{ const el = await (${js}); const r = el.getBoundingClientRect();
    return {x:r.x+r.width/2, y:r.y+r.height/2}; })()`);
}
async function cellRect(id, field) {
  return rectOf(`(async()=>{ const row = state.grid.getRow(${J(id)}); await row.scrollTo("center", false);
    await new Promise(r=>setTimeout(r,150)); return row.getCell(${J(field)}).getElement(); })()`);
}
async function clickCell(id, field, modifiers = 0) {
  const r = await cellRect(id, field);
  await page.click(r.x, r.y, 1, modifiers);
  await page.sleep(120);
}
async function clickButton(id) {
  const r = await rectOf(`document.getElementById(${J(id)})`);
  await page.click(r.x, r.y);
}
// openEditor 는 칸을 더블클릭해 편집기를 열고 글을 다 고른다 (editTriggerEvent: dblclick).
async function openEditor(id, field) {
  const r = await cellRect(id, field);
  await page.click(r.x, r.y, 2);
  await page.sleep(150);
  await page.key("a", "KeyA", 65, MOD.ctrl);
}
async function editCell(id, field, text) {
  await openEditor(id, field);
  await page.type(text);
  await page.key("Enter", "Enter", 13);
  await page.sleep(150);
}
async function hideToast() {
  await page.eval(`clearTimeout(toastTimer); document.getElementById("toast").hidden = true`);
}
async function waitToast(ms = 10000) {
  await page.waitFor(`!document.getElementById("toast").hidden`, ms);
  return page.eval(`document.getElementById("toast").textContent`);
}
async function clickSave() {
  await hideToast();
  await clickButton("save");
  const text = await waitToast();
  await page.sleep(200);
  return text;
}
// paste 는 지금 고른 칸에 클립보드 붙이기 이벤트를 쏜다. OS 클립보드는 CDP 로 못 만지므로 이벤트로 넣는다.
async function paste(text) {
  await page.eval(`(()=>{ const dt = new DataTransfer(); dt.setData("text/plain", ${J(text)});
    const a = document.activeElement;
    const t = a && a.closest && a.closest(".tabulator") ? a : document.querySelector(".tabulator-tableholder");
    t.dispatchEvent(new ClipboardEvent("paste", { clipboardData: dt, bubbles: true, cancelable: true }));
    return 1; })()`);
  await page.sleep(200);
}
async function rowData(id) {
  return page.eval(`(()=>{ const r = state.grid.getRow(${J(id)}); return r ? r.getData() : null; })()`);
}
// clickSortIcon 은 열 머리의 정렬 화살표를 누른다 (headerSortClickElement: "icon").
async function clickSortIcon(field) {
  const r = await rectOf(`state.grid.getColumn(${J(field)}).getElement().querySelector(".tabulator-col-sorter")`);
  await page.click(r.x, r.y);
  await page.sleep(400);
}
const dialogs = () => page.logs.filter((l) => l.startsWith("DIALOG:"));
async function openTable(name) {
  await page.eval(`openTable(${J(name)}).then(()=>true)`);
  await page.sleep(200);
}
async function fresh(table = "item") {
  resetData();
  await page.goto(A.url);
  if (table !== "item") await openTable(table);
  await page.sleep(200);
}

// scenario 는 기준판에서 페이지를 새로 열고 body 를 돌린 뒤 numstat 으로 판정한다.
// body 는 메모 글을 돌려주거나, { note, ok } 로 numstat 밖의 조건도 함께 낸다.
async function scenario(id, title, expectText, expectFn, body, table = "item") {
  let note = "";
  let extraOk = true;
  try {
    await fresh(table);
    const r = await body();
    if (r && typeof r === "object") { note = r.note || ""; extraOk = r.ok !== false; } else note = r || "";
  } catch (e) {
    note = "오류: " + e.message;
    extraOk = false;
  }
  const ns = numstat();
  record(id, title, expectText, extraOk && expectFn(ns), ns, note);
}

/* 시나리오 ------------------------------------------------------------- */

async function run() {
  const itemRows = (await (async () => { resetData(); return readData("item.json").split("\n").filter((l) => l.startsWith("{")).length; })());
  const dropRows = readData("drop.json").split("\n").filter((l) => l.startsWith("{")).length;

  // S0 — 2,000행 열기 · 120걸음 스크롤
  {
    const openMs = await page.goto(A.url);
    const s = await page.eval(`(async()=>{
      const h = document.querySelector(".tabulator-tableholder");
      const frames = []; let last = performance.now();
      const step = h.scrollHeight / 120;
      for (let i = 0; i < 120; i++) {
        h.scrollTop = i * step;
        await new Promise(r => requestAnimationFrame(r));
        const now = performance.now(); frames.push(now - last); last = now;
      }
      frames.sort((a,b)=>a-b);
      return { p95: frames[113], median: frames[60], rows: state.grid.getDataCount() };
    })()`);
    const ok = openMs < 2000 && s.p95 <= 33;
    record("S0", "2,000행 열기 · 120걸음 스크롤", "열기 2초 안 · p95 33ms 안", ok && none(numstat()), numstat(),
      `열기 ${openMs}ms · p95 ${s.p95.toFixed(1)}ms · 중간값 ${s.median.toFixed(1)}ms · ${s.rows}행`);
  }

  await scenario("S1", "안 고치고 저장", "없음", none, async () => clickSave());

  await scenario("S2", "int·string·float 세 칸", "item.json 3 3", only("item.json", 3, 3), async () => {
    await editCell("item_0500", "atk", "777");
    await editCell("item_1500", "name", "고친 이름");
    await editCell("item_1999", "price", "1.5");
    return clickSave();
  });

  await scenario("S3", "이름 열 정렬(아이콘) 뒤 한 칸", "item.json 1 1", only("item.json", 1, 1), async () => {
    await clickSortIcon("name");
    const top = await page.eval(`state.grid.getRows("active")[0].getData().id`);
    await editCell("item_0042", "atk", "4242");
    return `정렬 뒤 맨 위 ${top} · ${await clickSave()}`;
  });

  await scenario("S4", "3행×2열 TSV (LF · 끝 줄바꿈 없음)", "item.json 3 3", only("item.json", 3, 3), async () => {
    await clickCell("item_0100", "atk");
    await paste("11\t1.25\n22\t2.5\n33\t3.75");
    return clickSave();
  });

  await scenario("S5", "행 추가 + 이름 쓰기", "item.json 2 1", only("item.json", 2, 1), async () => {
    await clickButton("addRow");
    await page.sleep(300);
    await editCell("item_1", "name", "새 아이템");
    return clickSave();
  });

  await scenario("S6", "고치고 Ctrl+Z 뒤 저장", "없음", none, async () => {
    await editCell("item_0300", "atk", "1");
    await page.key("z", "KeyZ", 90, MOD.ctrl);
    await page.sleep(200);
    const v = (await rowData("item_0300")).atk;
    return `되돌린 값 ${v} · ${await clickSave()}`;
  });

  await scenario("S7", "편집 칸 연 채 Ctrl+S", "item.json 1 1", only("item.json", 1, 1), async () => {
    await openEditor("item_0800", "name");
    await page.type("저장중편집");
    await hideToast();
    await page.key("s", "KeyS", 83, MOD.ctrl);
    const text = await waitToast();
    await page.sleep(200);
    const dirty = await page.eval("state.dirty");
    return { note: `${text} · dirty ${dirty}`, ok: !dirty && readData("item.json").includes("저장중편집") };
  });

  await scenario("S8", "enum·ref 드롭다운", `enum 3개 · ref ${itemRows}개 · 없음`, none, async () => {
    let r = await cellRect("item_0001", "grade");
    await page.click(r.x, r.y, 2);
    await page.sleep(300);
    const enumItems = await page.eval(`[...document.querySelectorAll(".tabulator-edit-list-item")].map(e=>e.textContent)`);
    await page.shot(path.join(A.out, "S8-enum.png"));
    await page.key("Escape", "Escape", 27);
    await page.eval("state.dirty = false");
    await openTable("drop");
    r = await cellRect("drop_5", "item_id");
    await page.click(r.x, r.y, 2);
    await page.sleep(400);
    const refCount = await page.eval(`document.querySelectorAll(".tabulator-edit-list-item").length`);
    await page.shot(path.join(A.out, "S8-ref.png"));
    await page.key("Escape", "Escape", 27);
    const ok = J(enumItems) === J(["common", "rare", "epic"]) && refCount === itemRows;
    return { note: `enum ${J(enumItems)} · ref ${refCount}개`, ok };
  });

  await scenario("S9", "틀린 값 저장", "400 · 빨간 칸 · 없음", none, async () => {
    await editCell("item_1200", "atk", "-5");
    const text = await clickSave();
    const bad = await page.eval(`state.grid.getRow("item_1200").getCell("atk").getElement().classList.contains("bad")`);
    await page.shot(path.join(A.out, "S9-bad.png"));
    return { note: `${text} · 빨간 칸 ${bad}`, ok: bad && text.includes("검증에 걸려") };
  });

  // S10 — list 열 원소 문제(`counts[1]`)도 그 칸을 빨갛게 칠하고, 문제 줄을 누르면 그 칸으로 간다.
  await scenario("S10", "list 원소 틀림 → 빨간 칸 · 문제 줄로 그 칸", "400 · 빨간 칸 · 그 칸으로 · 없음", none, async () => {
    await editCell("drop_5", "counts", "1, x");
    const text = await clickSave();
    const cell = `state.grid.getRow("drop_5").getCell("counts").getElement()`;
    const bad = await page.eval(`${cell}.classList.contains("bad")`);
    await page.eval(`document.querySelector(".tabulator-tableholder").scrollTop = 1e9`);
    await page.sleep(300);
    await page.eval(`[...document.querySelectorAll("#problemList button.row")].find((b) => b.textContent.includes("counts[1]")).click()`);
    await page.sleep(400);
    const jumped = await page.eval(`${cell}.classList.contains("flash")`);
    await page.shot(path.join(A.out, "S10-list-bad.png"));
    return { note: `${text} · 빨간 칸 ${bad} · 그 칸으로 ${jumped}`, ok: bad && jumped && text.includes("검증에 걸려") };
  }, "drop");

  // Z1 — asset 썸네일을 누르면 크게 보기 덮개가 뜨고, 칸 편집기는 안 열리며, Esc 로 닫힌다. 저장은 안 한다.
  // monster 자료엔 icon 값이 없어 화면에서만 값을 넣는다 (gen.js·기준판은 그대로).
  await scenario("Z1", "썸네일 누름 → 크게 보기 · 편집 안 열림 · Esc 닫힘", "덮개 · 그림 로드 · 편집기 없음 · Esc 닫힘 · 없음", none, async () => {
    await page.eval(`state.grid.getRow("mon_1").update({ icon: "icons[icon_sword]" }).then(() => true)`);
    await page.waitFor(`document.querySelector(".asset-thumb.zoomable")`);
    const r = await rectOf(`document.querySelector(".asset-thumb.zoomable")`);
    await page.click(r.x, r.y);
    await page.waitFor(`document.querySelector(".zoom .zoom-img") && document.querySelector(".zoom .zoom-img").naturalWidth > 0`);
    await page.sleep(200);
    const info = await page.eval(`({ w: document.querySelector(".zoom-frame").style.width, cap: document.querySelector(".zoom-caption").textContent,
      editor: !!document.querySelector(".tabulator-editing, .tabulator-edit-list") })`);
    await page.shot(path.join(A.out, "Z1-zoom.png"));
    await page.key("Escape", "Escape", 27);
    await page.sleep(200);
    const closed = await page.eval(`!document.querySelector(".zoom")`);
    // 「닫기」 단추는 Space 로도 눌린다.
    await page.click(r.x, r.y);
    await page.waitFor(`document.querySelector(".zoom")`);
    await page.sleep(150);
    await page.key(" ", "Space", 32);
    await page.sleep(200);
    const spaceClosed = await page.eval(`!document.querySelector(".zoom")`);
    // 닫은 뒤에는 표 키가 다시 먹는다 — 칸 편집으로 글자를 넣어 본다 (저장은 안 한다).
    await editCell("mon_2", "name", "Z1확인");
    const typed = (await rowData("mon_2")).name;
    await page.eval("state.dirty = false");
    return { note: `틀 ${info.w} · ${info.cap} · 편집기 ${info.editor} · 닫힘 ${closed} · Space 닫힘 ${spaceClosed} · 닫은 뒤 입력 ${typed}`,
      ok: !info.editor && closed && spaceClosed && typed === "Z1확인" && info.cap.startsWith("icons[icon_sword]") };
  }, "monster");

  // L1 — list<asset> 「+N」 은 안 보인 원소 전부다. 미리보기 없는 원소(없는 주소)도 센다.
  await scenario("L1", "list<asset> +N = 원소 수 − 보인 미리보기", "▶ 3 · +2 · 없음", none, async () => {
    const cell = `state.grid.getRow("mon_1").getCell("sfx").getElement()`;
    await page.eval(`state.grid.getRow("mon_1").update({ sfx: ["nope", "Sfx/hit.wav", "Sfx/hit.wav", "Sfx/hit.wav", "Sfx/hit.wav"] }).then(() => true)`);
    await page.sleep(300);
    const got = await page.eval(`({ play: ${cell}.querySelectorAll(".asset-play").length, more: (${cell}.querySelector(".asset-more") || {}).textContent || "" })`);
    await page.eval("state.dirty = false");
    return { note: `▶ ${got.play} · ${got.more}`, ok: got.play === 3 && got.more === "+2" };
  }, "monster");

  // X1 — 드롭다운 XSS : 주소·enum 값·ref id 에 든 태그가 실행되지 않고 글자로 보이는가 (asset·enum·ref 편집기 셋).
  // asset 은 시험 색인의 xss 항목, enum·ref 는 화면의 값 배열에 넣는다 (파일은 안 바꾼다).
  await scenario("X1", "드롭다운 항목의 HTML 은 글자로 (asset·enum·ref)", "실행 안 됨 · 글자 그대로 ×3 · 없음", none, async () => {
    const XSS = `<img src=x onerror="window.__xss=(window.__xss||0)+1">`;
    const probe = async (id, field, typed) => {
      await openEditor(id, field);
      if (typed) {
        // page.type 은 글자만 넣고 키 이벤트를 안 내서 자동완성 거르기가 안 돈다 — keyup 을 한 번 쏜다.
        await page.type(typed);
        await page.eval(`(() => { const a = document.activeElement; a.dispatchEvent(new Event("input", { bubbles: true }));
          a.dispatchEvent(new KeyboardEvent("keyup", { bubbles: true, key: "s", code: "KeyS", keyCode: 83 })); return 1; })()`);
      }
      const hasText = `[...document.querySelectorAll(".tabulator-edit-list-item")].some((e) => e.textContent.includes('onerror="window.__xss'))`;
      await page.waitFor(hasText, 4000).catch(() => {});
      await page.sleep(600);
      const got = await page.eval(`({ xss: window.__xss || 0, text: ${hasText}, n: document.querySelectorAll(".tabulator-edit-list-item").length,
        first: (document.querySelector(".tabulator-edit-list-item") || {}).textContent || "", input: (document.activeElement || {}).value || "" })`);
      await page.key("Escape", "Escape", 27);
      await page.sleep(150);
      return got;
    };
    const asset = await probe("mon_1", "icon", "xss");
    await page.eval(`state.schema.enums.Element.push(${J(XSS)}); true`);
    const enm = await probe("mon_1", "element", "");
    await page.eval(`state.schema.enums.Element.pop(); true`);
    await openTable("drop");
    await page.eval(`state.ids.get("monster").push(${J("xss_" + XSS)}); true`);
    const ref = await probe("drop_1", "monster_id", "xss");
    await page.eval(`state.ids.get("monster").pop(); state.dirty = false; true`);
    const total = await page.eval(`window.__xss || 0`);
    return { note: `asset ${J(asset)} · enum ${J(enm)} · ref ${J(ref)} · 실행 ${total}`,
      ok: total === 0 && asset.text && enm.text && ref.text };
  }, "monster");

  // G1 — 아틀라스 격자 : 맨 아틀라스 칸에서 격자가 열리고, 타일을 골라 「이 칸에 넣기」 → 칸 값·dirty.
  // 크게 보기의 「아틀라스 전부 보기」 는 지금 스프라이트를 고른 채 격자를 열고, list 칸 격자에는 넣기 단추가 없다. 저장은 안 한다.
  await scenario("G1", "아틀라스 격자 훑어보기 · 고르기 · 넣기", "타일 3 · 넣기 → 주소[이름]·dirty · 전부 보기 · list 넣기 없음 · 없음", none, async () => {
    const cell = `state.grid.getRow("mon_1").getCell("icon").getElement()`;
    const grid = `document.querySelector(".zoom .atlas-grid")`;
    await page.eval(`state.grid.getRow("mon_1").update({ icon: "ui_atlas" }).then(() => true)`);
    await page.waitFor(`${cell}.querySelector(".asset-thumb.zoomable")`);
    let r = await rectOf(`${cell}.querySelector(".asset-thumb.zoomable")`);
    await page.click(r.x, r.y);
    await page.waitFor(grid, 3000);
    const head = await page.eval(`document.querySelector(".zoom .atlas-head").textContent`);
    const tiles = await page.eval(`document.querySelectorAll(".zoom .atlas-tile").length`);
    await page.waitFor(`[...document.querySelectorAll(".zoom .atlas-tile .asset-thumb")].filter((t) => t.style.backgroundImage).length >= 2`, 3000).catch(() => {});
    r = await rectOf(`[...document.querySelectorAll(".zoom .atlas-tile")].find((t) => t.textContent.includes("icon_potion"))`);
    await page.click(r.x, r.y);
    await page.sleep(200);
    const picked = await page.eval(`({ sel: (document.querySelector(".zoom .atlas-tile.selected") || {}).textContent || "", info: document.querySelector(".zoom .atlas-info").textContent, value: state.grid.getRow("mon_1").getData().icon })`);
    await page.shot(path.join(A.out, "G1-grid.png"));
    r = await rectOf(`document.querySelector(".zoom .atlas-put")`);
    await page.click(r.x, r.y);
    await page.sleep(300);
    const put = await page.eval(`({ open: !!document.querySelector(".zoom"), value: state.grid.getRow("mon_1").getData().icon, dirty: state.dirty })`);
    // 크게 보기 → 「아틀라스 전부 보기」 → 지금 스프라이트가 골라진 격자.
    await page.waitFor(`${cell}.querySelector(".asset-thumb.zoomable") && ${cell}.textContent.includes("icon_potion")`);
    r = await rectOf(`${cell}.querySelector(".asset-thumb.zoomable")`);
    await page.click(r.x, r.y);
    await page.waitFor(`document.querySelector(".zoom .zoom-all")`, 3000);
    r = await rectOf(`document.querySelector(".zoom .zoom-all")`);
    await page.click(r.x, r.y);
    await page.waitFor(grid, 3000);
    const again = await page.eval(`(document.querySelector(".zoom .atlas-tile.selected") || {}).textContent || ""`);
    await page.key("Escape", "Escape", 27);
    await page.sleep(200);
    const closed = await page.eval(`!document.querySelector(".zoom")`);
    // list 칸 : 보기만 — 넣기 단추가 없다.
    const lcell = `state.grid.getRow("mon_2").getCell("sfx").getElement()`;
    await page.eval(`state.grid.getRow("mon_2").update({ sfx: ["ui_atlas[btn_ok]"] }).then(() => true)`);
    await page.waitFor(`${lcell}.querySelector(".asset-thumb.zoomable")`);
    r = await rectOf(`${lcell}.querySelector(".asset-thumb.zoomable")`);
    await page.click(r.x, r.y);
    await page.waitFor(`document.querySelector(".zoom .zoom-all")`, 3000);
    r = await rectOf(`document.querySelector(".zoom .zoom-all")`);
    await page.click(r.x, r.y);
    await page.waitFor(grid, 3000);
    const listPut = await page.eval(`!!document.querySelector(".zoom .atlas-put")`);
    await page.key("Escape", "Escape", 27);
    await page.sleep(150);
    await page.eval("state.dirty = false");
    return { note: `${head} · 타일 ${tiles} · 고름 ${picked.sel} (${picked.info}, 값 ${picked.value}) · 넣은 뒤 ${J(put)} · 전부 보기 고름 ${again} · 닫힘 ${closed} · list 넣기 ${listPut}`,
      ok: head === "ui_atlas · 3장" && tiles === 3 && picked.sel.includes("icon_potion") && picked.info === "icon_potion · 64×64" &&
        picked.value === "ui_atlas" && !put.open && put.value === "ui_atlas[icon_potion]" && put.dirty === true &&
        again.includes("icon_potion") && closed && !listPut };
  }, "monster");

  // A1 — 아틀라스 : 맨 주소는 첫 스프라이트 + 「N장」, `주소[이름]` 은 sub.path 그림(rect 0 = 전체) + sprite 태그 · 크게 보기.
  await scenario("A1", "아틀라스 스프라이트 썸네일 · 크게 보기", "N장 · sprite · 24×16 · 없음", none, async () => {
    const cell = `state.grid.getRow("mon_1").getCell("icon").getElement()`;
    await page.eval(`state.grid.getRow("mon_1").update({ icon: "ui_atlas" }).then(() => true)`);
    await page.waitFor(`${cell}.querySelector(".asset-thumb.zoomable")`);
    const bare = await page.eval(`${cell}.querySelector(".kindtag").textContent`);
    // 맨 아틀라스는 「N장」 표를 눌러도 격자 창이 열린다 (썸네일은 G1 이 본다).
    // 기본 열 너비에선 표가 칸 밖으로 잘려 눌리지 않는다 — 열을 넓혀 표가 보이게 한다.
    await page.eval(`state.grid.getColumn("icon").setWidth(280) || true`);
    await page.sleep(100);
    const rb = await rectOf(`${cell}.querySelector(".kindtag")`);
    await page.click(rb.x, rb.y);
    await page.waitFor(`document.querySelector(".zoom .atlas-head")`, 3000).catch(() => {});
    const bareCap = await page.eval(`(document.querySelector(".zoom .atlas-head") || {}).textContent || ""`);
    await page.key("Escape", "Escape", 27);
    await page.sleep(150);
    // 없는 하위 이름에는 sprite 표를 달지 않고 「없음」 으로 보인다.
    await page.eval(`state.grid.getRow("mon_1").update({ icon: "ui_atlas[nope]" }).then(() => true)`);
    await page.sleep(200);
    const nope = await page.eval(`({ tag: (${cell}.querySelector(".kindtag") || {}).textContent || "", missing: !!${cell}.querySelector(".asset-missing") })`);
    await page.eval(`state.grid.getRow("mon_1").update({ icon: "ui_atlas[btn_ok]" }).then(() => true)`);
    await page.waitFor(`${cell}.querySelector(".asset-thumb.zoomable") && ${cell}.textContent.includes("btn_ok")`);
    const tag = await page.eval(`${cell}.querySelector(".kindtag").textContent`);
    await page.shot(path.join(A.out, "A1-atlas-cell.png"));
    const r = await rectOf(`${cell}.querySelector(".asset-thumb.zoomable")`);
    await page.click(r.x, r.y);
    await page.waitFor(`document.querySelector(".zoom .zoom-img") && document.querySelector(".zoom .zoom-img").naturalWidth > 0`);
    await page.sleep(200);
    const cap = await page.eval(`document.querySelector(".zoom-caption").textContent`);
    await page.shot(path.join(A.out, "A1-atlas-zoom.png"));
    await page.key("Escape", "Escape", 27);
    await page.sleep(200);
    const closed = await page.eval(`!document.querySelector(".zoom")`);
    await page.eval("state.dirty = false");
    return { note: `맨 주소 ${bare} · ${bareCap} · 없는 이름 ${J(nope)} · 하위 ${tag} · ${cap} · 닫힘 ${closed}`,
      ok: bare === "3장" && bareCap === "ui_atlas · 3장" && nope.tag !== "sprite" && nope.missing &&
        tag === "sprite" && cap === "ui_atlas[btn_ok] · 24×16" && closed };
  }, "monster");

  await scenario("P1", "엑셀식 CRLF + 끝 줄바꿈 (2행)", "item.json 2 2", only("item.json", 2, 2), async () => {
    await clickCell("item_0100", "atk");
    await paste("11\t1.25\r\n22\t2.5\r\n");
    const below = await rowData("item_0102");
    const text = await clickSave();
    return { note: `${text} · 아래 행 atk ${below.atk}`, ok: !readData("item.json").includes("\r") };
  });

  await scenario("P2", "문자열 칸 2행 `이름A\\r\\n이름B`", "item.json 2 2 · \\r 0", only("item.json", 2, 2), async () => {
    await clickCell("item_0400", "name");
    await paste("이름A\r\n이름B");
    const text = await clickSave();
    const file = readData("item.json");
    return { note: text, ok: !file.includes("\r") && file.includes(`"이름A"`) && file.includes(`"이름B"`) };
  });

  await scenario("P3", "끝 2행에 4행 붙이기", "item.json 2 2 · 토스트 「넘어」", only("item.json", 2, 2), async () => {
    const ids = await page.eval(`state.grid.getRows("active").slice(-2).map(r => r.getData().id)`);
    await clickCell(ids[0], "atk");
    await hideToast();
    await paste("1\n2\n3\n4");
    const warn = await waitToast(3000).catch(() => "");
    await page.shot(path.join(A.out, "P3-overflow.png"));
    const count = await page.eval("state.grid.getDataCount()");
    const text = await clickSave();
    return { note: `알림 「${warn}」 · 행 ${count} · ${text}`, ok: warn.includes("넘어") && count === itemRows };
  });

  await scenario("P4", "TRUE/FALSE 붙이기", "검증이 막는다 · 없음", none, async () => {
    await clickCell("item_0200", "usable");
    await paste("TRUE\nFALSE");
    const text = await clickSave();
    return { note: text, ok: text.includes("검증에 걸려") };
  });

  await scenario("P5", "열 넘침 붙이기 (마지막 열에 3열)", "item.json 1 1 · 토스트 「2열이 … 넘어」", only("item.json", 1, 1), async () => {
    const last = await page.eval(`state.grid.getColumns().filter(c => c.isVisible() && c.getField()).pop().getField()`);
    await clickCell("item_0100", last);
    await hideToast();
    await paste("newtag\tX\tY");
    const warn = await waitToast(3000).catch(() => "");
    await page.shot(path.join(A.out, "P5-column-overflow.png"));
    const row = await rowData("item_0100");
    const fields = await page.eval(`state.grid.getColumns().map(c => c.getField()).filter(Boolean)`);
    const extra = Object.keys(row).filter((k) => !fields.includes(k));
    const text = await clickSave();
    const ok = last === "tags" && warn.includes("3열 중 2열") && warn.includes("넘어") && extra.length === 0 && row.tags === "newtag";
    return { note: `마지막 열 ${last} · 알림 「${warn}」 · tags ${J(row.tags)} · 딴 키 ${J(extra)} · ${text}`, ok };
  });

  // F1a — fmt 판에 300.0 을 손으로 넣고 커밋 → fmt --check 종료 2
  {
    let note = "";
    let pass = false;
    try {
      resetData();
      const file = readData("item.json");
      const line = file.split("\n").find((l) => l.includes(`"price":300,`) && l.startsWith(`{"id":"item_`));
      fs.writeFileSync(path.join(A.data, "item.json"), file.replace(line, line.replace(`"price":300,`, `"price":300.0,`)));
      git("commit", "-q", "-am", "F1: 300.0 을 손으로 넣음");
      let code = 0;
      try {
        execFileSync(A.exe, ["fmt", "--check", "--data", A.data], { encoding: "utf8", stdio: "pipe" });
      } catch (e) {
        code = e.status;
      }
      pass = code === 2;
      note = `fmt --check 종료 ${code}`;
    } catch (e) {
      note = "오류: " + e.message;
    }
    record("F1a", "300.0 을 넣고 fmt --check", "종료 2", pass, null, note);
  }

  // F1b — fmt 뒤 커밋, 딴 칸 하나 고침 → 1 1
  {
    let note = "";
    let ns = [];
    try {
      execFileSync(A.exe, ["fmt", "--data", A.data], { encoding: "utf8", stdio: "pipe" });
      git("commit", "-q", "-am", "F1: fmt");
      const f1Base = git("rev-parse", "HEAD").trim();
      await page.goto(A.url);
      await editCell("item_0700", "atk", "7007");
      note = await clickSave();
      ns = numstat();
      resetData(f1Base);
    } catch (e) {
      note = "오류: " + e.message;
    }
    record("F1b", "fmt 뒤 딴 칸 하나", "item.json 1 1", only("item.json", 1, 1)(ns), ns, note);
  }

  await scenario("N1", "숫자 철자 15건 — Go 꼴 = JS 꼴", "없음 · 줄마다 JS String(Number) 와 같음", none, async () => {
    const lines = readData("item.json").split("\n").filter((l) => l.startsWith(`{"id":"n1_`));
    const wrong = [];
    lines.forEach((l) => {
      const spelling = /"name":"N1 ([^"]+)"/.exec(l)[1];
      const js = String(Number(spelling));
      const want = js === "0" ? !l.includes(`"price"`) : l.includes(`"price":${js}}`);
      if (!want) wrong.push(`${spelling}→${l}`);
    });
    const text = await clickSave();
    return { note: `${lines.length}줄 · 어긋남 ${wrong.length}${wrong.length ? " " + wrong.join(" / ") : ""} · ${text}`, ok: lines.length === 15 && wrong.length === 0 };
  });

  await scenario("R1", "drop 가운데 한 행 지우기", "drop.json 0 1", only("drop.json", 0, 1), async () => {
    const mid = `drop_${Math.floor(dropRows / 2)}`;
    await clickCell(mid, "rate");
    await hideToast();
    await clickButton("delRow");
    const text = await waitToast();
    const gone = (await rowData(mid)) === null;
    return { note: `${text} · ${await clickSave()}`, ok: gone };
  }, "drop");

  await scenario("R2", "drop 가운데 3행 범위 지우기 (+ Ctrl+Z 한 번에 되살림)", "drop.json 0 3", only("drop.json", 0, 3), async () => {
    const m = Math.floor(dropRows / 2);
    const ids = [`drop_${m}`, `drop_${m + 1}`, `drop_${m + 2}`];
    await clickCell(ids[0], "rate");
    await clickCell(ids[2], "rate", MOD.shift);
    await hideToast();
    await clickButton("delRow");
    const text = await waitToast();
    await page.shot(path.join(A.out, "R2-delete.png"));
    const afterDelete = await page.eval("state.grid.getDataCount()");
    await page.key("z", "KeyZ", 90, MOD.ctrl); // 초점은 「행 지우기」 단추 — 표 밖 Ctrl+Z
    await page.sleep(200);
    const afterUndo = await page.eval("state.grid.getDataCount()");
    const order = await page.eval(`state.grid.getData().slice(${m - 1}, ${m + 4}).map(r => r.id).join(",")`);
    const wantOrder = [`drop_${m - 1}`, ...ids, `drop_${m + 3}`].join(",");
    // 되살린 뒤 다시 골라 지우고 저장한다
    await clickCell(ids[0], "rate");
    await clickCell(ids[2], "rate", MOD.shift);
    await clickButton("delRow");
    await page.sleep(200);
    const save = await clickSave();
    const ok = text.includes("3행") && afterDelete === dropRows - 3 && afterUndo === dropRows && order === wantOrder;
    return { note: `${text} · 지운 뒤 ${afterDelete} · Ctrl+Z 뒤 ${afterUndo} · 차례 ${order === wantOrder ? "그대로" : order} · ${save}`, ok };
  }, "drop");

  // R3 — 200행 범위 지우기는 확인창이 뜬다 (cdp.js 가 「확인」으로 닫는다). 저장 → 0 200, Ctrl+Z → 저장 → 없음.
  await scenario("R3", "drop 200행 범위 지우기 (확인창 수락) + Ctrl+Z 한 번에 되살림", "중간 drop.json 0 200 · 끝 없음", none, async () => {
    const m = Math.floor(dropRows / 2) - 100;
    const firstId = `drop_${m}`;
    const lastId = `drop_${m + 199}`;
    await clickCell(firstId, "rate");
    await clickCell(lastId, "rate", MOD.shift);
    await page.shot(path.join(A.out, "R3-before-confirm.png"));
    const before = dialogs().length;
    await hideToast();
    await clickButton("delRow");
    const text = await waitToast();
    const dlg = dialogs().slice(before);
    const afterDelete = await page.eval("state.grid.getDataCount()");
    await clickSave();
    const mid = numstat();
    await page.key("z", "KeyZ", 90, MOD.ctrl); // 초점은 「저장」 단추 — 표 밖 Ctrl+Z
    await page.sleep(300);
    const afterUndo = await page.eval("state.grid.getDataCount()");
    const order = await page.eval(`state.grid.getData().slice(${m - 1}, ${m + 201}).map(r => r.id).join(",")`);
    const wantOrder = Array.from({ length: 202 }, (_, i) => `drop_${m - 1 + i}`).join(",");
    const save = await clickSave();
    const ok = dlg.length === 1 && dlg[0].includes("200행을 지울까") && text.includes("200행") &&
      afterDelete === dropRows - 200 && only("drop.json", 0, 200)(mid) && afterUndo === dropRows && order === wantOrder;
    return { note: `확인창 ${J(dlg)} · ${text} · 지운 뒤 ${afterDelete} · 중간 numstat ${fmtNs(mid)} · Ctrl+Z 뒤 ${afterUndo} · 차례 ${order === wantOrder ? "그대로" : "어긋남"} · ${save}`, ok };
  }, "drop");

  // R4 — 칸 하나를 누른 뒤 열 머리(정렬 화살표)를 눌러도 범위는 누른 그 칸이다 (정렬로 줄이 바뀌어도).
  //      행 지우기는 누른 1행만, 확인창 없이.
  await scenario("R4", "열 머리(정렬 아이콘) 클릭 뒤 행 지우기", "drop.json 0 1 · 확인창 없음", only("drop.json", 0, 1), async () => {
    const mid = `drop_${Math.floor(dropRows / 2)}`;
    await clickCell(mid, "rate");
    await clickSortIcon("rate");
    const ranged = await page.eval(`rangeRows().map(r => r.getData().id)`);
    const before = dialogs().length;
    await hideToast();
    await clickButton("delRow");
    const text = await waitToast();
    const dlg = dialogs().slice(before);
    const count = await page.eval("state.grid.getDataCount()");
    const gone = ranged.length === 1 ? (await rowData(ranged[0])) === null : false;
    const save = await clickSave();
    const ok = ranged.length === 1 && ranged[0] === mid && gone && dlg.length === 0 && count === dropRows - 1;
    return { note: `범위 ${J(ranged)} (누른 행 ${mid}) · 확인창 ${dlg.length} · ${text} · 행 ${count} · ${save}`, ok };
  }, "drop");

  await schemaScenarios();

  // D1 — 서버를 끄고 저장 (맨 끝). 서버가 죽으니 늘 마지막이다.
  await scenario("D1", "서버를 끄고 저장", "빨간 토스트 「서버에 못 닿았다」 · dirty · 없음", none, async () => {
    await editCell("item_0005", "atk", "5");
    try { process.kill(Number(A["serve-pid"])); } catch (e) { return { note: "serve 를 못 껐다: " + e.message, ok: false }; }
    await page.sleep(500);
    const text = await clickSave();
    const bad = await page.eval(`document.getElementById("toast").classList.contains("bad")`);
    const dirty = await page.eval("state.dirty");
    await page.eval(`document.getElementById("toast").hidden = false`);
    await page.shot(path.join(A.out, "D1-unreachable.png"));
    return { note: `${text} · 빨강 ${bad} · dirty ${dirty}`, ok: text.includes("서버에 못 닿았다") && bad && dirty };
  });
}

/* 스키마 · Enum 탭 (설계 2026-10-04 5장) ------------------------------- */

// openTab 은 위 탭 단추를 눌러 화면을 바꾼다. 스키마·Enum 탭은 사본이 서야 끝난다.
async function openTab(tab) {
  const r = await rectOf(`document.querySelector('.tabs [data-tab=${J(tab)}]')`);
  await page.click(r.x, r.y);
  if (tab !== "data") await page.waitFor(`schemaState.draft && !document.getElementById("schemaView").hidden`);
  await page.sleep(150);
}
// pick 은 왼쪽 목록(표·enum)에서 하나를 누른다.
async function pick(name) {
  const r = await rectOf(`[...document.querySelectorAll("#pickList button")].find((b) => b.firstChild.textContent === ${J(name)})`);
  await page.click(r.x, r.y);
  await page.sleep(150);
}
// setInput 은 칸을 눌러 글을 다 고르고 새로 친 뒤 초점을 빼 change 를 낸다 (사람이 고치고 딴 데 누르는 것과 같다).
async function setInput(js, text) {
  const r = await rectOf(js);
  await page.click(r.x, r.y);
  await page.key("a", "KeyA", 65, MOD.ctrl);
  await page.type(text);
  await page.eval(`document.activeElement.blur(), true`);
  await page.sleep(150);
}
const columnInput = (column, nth) => `document.querySelector('#columnTable tr[data-column=${J(column)}]').querySelectorAll("input,select")[${nth}]`;
const valueRow = (value) => `document.querySelector('#valueTable tr[data-value=${J(value)}]')`;
async function clickEl(js) {
  const r = await rectOf(js);
  await page.click(r.x, r.y);
  await page.sleep(150);
}
// preview 는 「미리보기 · 저장」 을 눌러 창이 뜰 때까지 기다리고 창 내용을 돌려준다.
async function preview() {
  await clickEl(`document.getElementById("preview")`);
  await page.waitFor(`document.getElementById("sheet").open && document.querySelector("#sheet .sheet-summary")`);
  return sheetInfo();
}
async function sheetInfo() {
  return page.eval(`({ summary: [...document.querySelectorAll("#sheet .sheet-summary")].map((e) => e.textContent).join(" / "),
    files: [...document.querySelectorAll("#sheet .plan-files tr")].slice(1).map((tr) => [...tr.children].map((td) => td.textContent).join(" ")),
    problems: [...document.querySelectorAll("#sheet .plan-problems li")].map((li) => li.textContent),
    notes: [...document.querySelectorAll("#sheet .plan-notes li")].map((li) => li.textContent),
    save: document.getElementById("sheetSave") ? !document.getElementById("sheetSave").disabled : null })`);
}
// saveSheet 는 미리보기 창의 「저장」 을 누르고 알림을 기다린다.
async function saveSheet() {
  await hideToast();
  await clickEl(`document.getElementById("sheetSave")`);
  return waitToast();
}
const lineCount = (file, needle) => readData(file).split("\n").filter((l) => l.includes(needle)).length;
// exactly 는 numstat 이 [파일, 더한 줄, 지운 줄] 목록과 차례 없이 같은지 본다.
const exactly = (want) => (ns) => ns.length === want.length &&
  want.every(([f, a, d]) => ns.some((n) => n.f === f && n.a === a && n.d === d));

async function schemaScenarios() {
  resetData();
  const atkLines = lineCount("item.json", `"atk":`);
  const rareLines = lineCount("item.json", `"grade":"rare"`);

  // K1 — 열 이름 바꾸기 → 미리보기 → 저장 → 표에 반영 (atk → attack, 행 값까지 따라온다)
  await scenario("K1", "열 이름 바꾸기 → 미리보기 → 저장 → 표에 반영", `item.json ${atkLines} ${atkLines} · schema.json 1 1`,
    exactly([["item.json", atkLines, atkLines], ["schema.json", 1, 1]]), async () => {
      const before = (await rowData("item_0500")).atk;
      await openTab("schema");
      await page.shot(path.join(A.out, "K1-schema-tab.png"));
      await setInput(columnInput("atk", 0), "attack");
      const ops = await page.eval(`JSON.stringify(schemaState.ops)`);
      const info = await preview();
      await page.shot(path.join(A.out, "K1-preview.png"));
      const text = await saveSheet();
      await openTab("data");
      await page.waitFor(`state.grid.getColumn("attack")`);
      const after = (await rowData("item_0500")).attack;
      const ok = ops === J([{ op: "renameColumn", table: "item", from: "atk", to: "attack" }]) &&
        info.files.some((f) => f.startsWith(`item.json ${atkLines} 0`)) && info.save === true &&
        text.includes("스키마를 저장했다") && after === before && !(await page.eval(`!!state.grid.getColumn("atk")`)) &&
        /"name":\s*"attack",\s*"type":\s*"int"/.test(readData("schema.json"));
      return { note: `ops ${ops} · ${info.summary} · 파일 ${J(info.files)} · ${text} · item_0500 ${before} → attack ${after}`, ok };
    });

  // K2 — enum 값 이름 바꾸기 (Grade.rare → uncommon). 숫자는 그대로라 schema.json 은 배열 꼴 한 줄만 바뀐다.
  await scenario("K2", "enum 값 이름 바꾸기", `item.json ${rareLines} ${rareLines} · schema.json 1 1`,
    exactly([["item.json", rareLines, rareLines], ["schema.json", 1, 1]]), async () => {
      await openTab("enum");
      await pick("Grade");
      await page.shot(path.join(A.out, "K2-enum-tab.png"));
      await setInput(`${valueRow("rare")}.querySelector("input")`, "uncommon");
      const info = await preview();
      const text = await saveSheet();
      const file = readData("item.json");
      const enums = await page.eval(`JSON.stringify(state.schema.enums.Grade)`);
      const ok = text.includes("스키마를 저장했다") && !file.includes(`"rare"`) && file.includes(`"grade":"uncommon"`) &&
        enums === J(["common", "uncommon", "epic"]);
      return { note: `${info.summary} · ${J(info.files)} · ${text} · Grade ${enums}`, ok };
    });

  // K3 — 쓰는 행이 있는 값을 대체 값 없이 지우면 미리보기가 막는다. 대체 값을 고르면 통과한다 (저장은 안 한다).
  await scenario("K3", "쓰이는 enum 값 지우기 — 대체 값 없으면 막힘", "막힘 · 저장 단추 꺼짐 · 대체 값이면 통과 · 없음", none, async () => {
    await openTab("enum");
    await pick("Grade");
    const drop = async (replace) => {
      await clickEl(`[...${valueRow("rare")}.querySelectorAll("button")].find((b) => b.textContent === "지우기")`);
      await page.waitFor(`document.getElementById("replaceWith")`);
      if (replace) await page.eval(`(() => { const s = document.getElementById("replaceWith"); s.value = ${J(replace)}; s.dispatchEvent(new Event("change")); return 1; })()`);
      await clickEl(`document.getElementById("dropValueOk")`);
    };
    await drop("");
    const blocked = await preview();
    await page.shot(path.join(A.out, "K3-blocked.png"));
    await clickEl(`document.getElementById("sheetClose")`);
    await page.eval(`loadSchemaEdit().then(() => true)`);
    await pick("Grade");
    await drop("common");
    const ops = await page.eval(`JSON.stringify(schemaState.ops)`);
    const passed = await preview();
    await clickEl(`document.getElementById("sheetClose")`);
    const ok = !blocked.save && blocked.summary.includes("막혔다") && blocked.problems.some((p) => p.includes("replaceWith")) &&
      passed.save === true && passed.files.some((f) => f.startsWith("item.json")) &&
      ops === J([{ op: "dropEnumValue", enum: "Grade", value: "rare", replaceWith: "common" }]);
    await page.eval(`setSchemaDirty(false), true`);
    return { note: `막힘 ${J(blocked)} · 대체 ${ops} → ${passed.summary} ${J(passed.files)}`, ok };
  });

  // K4 — 미리보기 뒤 디스크의 schema.json 이 바뀌면 저장은 409 — 아무것도 안 쓰고 「다시 읽기」 를 권한다.
  await scenario("K4", "409 — 디스크가 바뀌면 다시 읽기 안내", "schema.json 1 0 (밖에서 고친 줄만) · item.json 그대로", only("schema.json", 1, 0), async () => {
    await openTab("schema");
    await setInput(columnInput("price", 0), "cost");
    const info = await preview();
    fs.appendFileSync(path.join(A.data, "schema.json"), "\n"); // 다른 사람·편집기가 고친 것
    await clickEl(`document.getElementById("sheetSave")`);
    await page.waitFor(`document.getElementById("sheetReload")`);
    const conflict = await sheetInfo();
    await page.shot(path.join(A.out, "K4-conflict.png"));
    const revBefore = await page.eval("schemaState.rev");
    await clickEl(`document.getElementById("sheetReload")`);
    await page.waitFor(`!schemaState.dirty && schemaState.rev !== ${J(revBefore)}`);
    const back = await page.eval(`!!document.querySelector('#columnTable tr[data-column="price"]')`);
    const ok = info.save === true && conflict.summary.includes("바뀌었다") && back;
    return { note: `${conflict.summary} · 다시 읽은 뒤 price ${back}`, ok };
  });

  // K5 — 저장 안 한 행 편집이 있으면 스키마 편집을 막고, 저장 안 한 스키마 변경이 있으면 행 편집을 막는다.
  await scenario("K5", "행 편집 중엔 스키마 편집 막힘 (반대도)", "스키마 막힘 · 행 막힘 · 없음", none, async () => {
    await editCell("item_0500", "atk", "777");
    await openTab("schema");
    const a = await page.eval(`({ bar: document.getElementById("schemaLock").hidden ? "" : document.getElementById("schemaLock").textContent,
      fields: document.getElementById("schemaFields").disabled, preview: document.getElementById("preview").disabled })`);
    await page.shot(path.join(A.out, "K5-schema-locked.png"));
    await openTab("data");
    await page.key("z", "KeyZ", 90, MOD.ctrl); // 표 밖 Ctrl+Z 로 되돌리고 dirty 를 끈다
    await page.eval("setDirty(false), true");
    await openTab("schema");
    await setInput(columnInput("name", 4), "아이템 이름"); // 설명 칸(이름·형·default·loc·설명) — op 없는 편집
    await openTab("data");
    const b = await page.eval(`({ bar: document.getElementById("rowLock").hidden ? "" : document.getElementById("rowLock").textContent,
      save: document.getElementById("save").disabled, locked: document.getElementById("table").classList.contains("locked") })`);
    await hideToast();
    await page.key("s", "KeyS", 83, MOD.ctrl);
    const text = await waitToast(3000).catch(() => "");
    await page.shot(path.join(A.out, "K5-rows-locked.png"));
    await page.eval("setSchemaDirty(false), true");
    const ok = a.bar.includes("저장 안 한 행 편집") && a.fields && a.preview && b.bar.includes("저장 안 한 스키마 변경") &&
      b.save && b.locked && text.includes("저장 안 한 스키마 변경");
    return { note: `스키마 쪽 ${J(a)} · 행 쪽 ${J(b)} · Ctrl+S 「${text}」`, ok };
  });

  // X2 — 서버가 돌려준 글(알림·문제)에 든 태그는 미리보기 창에서 글자로만 보인다.
  //      monster.icon 기본값에 태그를 넣으면 plan 의 「기본값이 바뀐다」 알림이 그 값을 그대로 싣는다.
  await scenario("X2", "미리보기 창의 HTML 은 글자로", "실행 안 됨 · 알림에 태그 글자 · 없음", none, async () => {
    const XSS = `<img src=x onerror="window.__xss=(window.__xss||0)+1">`;
    await openTab("schema");
    await pick("monster");
    await setInput(columnInput("icon", 3), XSS); // default 칸(이름·형·kind·default)
    const info = await preview();
    await page.sleep(300);
    const total = await page.eval(`window.__xss || 0`);
    const imgs = await page.eval(`document.querySelectorAll("#sheet img").length`);
    const shown = info.notes.concat(info.problems).some((t) => t.includes("onerror"));
    await clickEl(`document.getElementById("sheetClose")`);
    await page.eval("setSchemaDirty(false), true");
    return { note: `알림 ${J(info.notes)} · 문제 ${J(info.problems.slice(0, 2))} · 실행 ${total} · img ${imgs}`, ok: total === 0 && imgs === 0 && shown };
  }, "monster");

  // K6 — 「C# 만들기」 : 디스크 스키마로 C# 을 gen 폴더에 쓴다 (데이터 폴더 밖이라 numstat 은 없음).
  await scenario("K6", "C# 만들기", "written 에 ItemRow.cs · 파일 있음 · 없음", none, async () => {
    await clickEl(`document.getElementById("gen")`);
    await page.waitFor(`document.getElementById("sheet").open && document.querySelector("#sheet .sheet-summary")`, 20000);
    const got = await page.eval(`({ summary: document.querySelector("#sheet .sheet-summary").textContent,
      written: [...document.querySelectorAll("#sheet .gen-written li")].map((li) => li.textContent) })`);
    await page.shot(path.join(A.out, "K6-gen.png"));
    await clickEl(`document.getElementById("sheetClose")`);
    const onDisk = A.gen ? fs.existsSync(path.join(A.gen, "ItemRow.cs")) && fs.existsSync(path.join(A.gen, "Grade.cs")) : false;
    return { note: `${got.summary} · ${got.written.length}개 · 디스크 ${onDisk}`, ok: got.summary.includes("썼다") && got.written.includes("ItemRow.cs") && onDisk };
  });
}

(async () => {
  try {
    page = await open(Number(A.cdp));
  } catch (e) {
    console.error(e.message);
    process.exit(e.environment ? 2 : 1);
  }
  let crashed = null;
  try {
    await run();
  } catch (e) {
    crashed = e;
    console.error("시나리오가 도중에 죽었다:", e);
  }
  const logs = page.logs.filter((l) => !l.includes("headerSort with selectableRangeColumns"));
  fs.writeFileSync(path.join(A.out, "u6-results.json"), JSON.stringify({ results, logs, crashed: crashed ? String(crashed) : null }, null, 2));
  page.close();
  const failed = results.filter((r) => !r.pass).length;
  console.log(`U6: ${results.length - failed}/${results.length} 통과`);
  process.exit(crashed || failed ? 1 : 0);
})();
