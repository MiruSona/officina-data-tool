/* DataTool 표 편집기.
 *
 * 이 화면은 JSON 파일을 절대 만들지 않는다. 행 배열만 엔진에 보내고
 * 「한 줄 한 행·열 차례·기본값 빼기」는 엔진이 한다 (설계 9장).
 * 되돌리기·붙여넣기·정렬·거르기는 Tabulator 몫이라 직접 만들지 않는다.
 */
"use strict";

// 토큰은 주소로 한 번 받아서 바로 주소창에서 지운다 — 어깨너머로 새는 것을 줄인다.
const TOKEN = new URLSearchParams(location.search).get("t") || "";
history.replaceState(null, "", location.pathname);

const state = {
  schema: null,       // /api/schema 결과
  name: "",           // 지금 보고 있는 표
  grid: null,         // Tabulator 인스턴스
  dirty: false,
  active: null,       // 마지막으로 누른 행 (범위가 비었을 때 행 지우기가 쓴다)
  activeCell: null,   // 마지막으로 누른 칸 (정렬 뒤 범위를 다시 세울 때 쓴다)
  undoing: false,     // 묶음 되돌리기 중 — historyUndo·historyRedo 재진입을 막는다
  pasteNote: "",      // 붙여넣기 파서가 남긴 열 넘침 알림. pasteAction 이 행 넘침과 함께 띄운다
  ids: new Map(),     // ref 용 : 표 이름 → id 목록
  assets: null,       // /api/assetindex 결과. asset 열이 없으면 null
  assetMap: new Map(), // address → 항목 목록
};

const $ = (id) => document.getElementById(id);

/* 서버와 말하기 ------------------------------------------------------- */

// api 는 fetch 가 던져도(서버 꺼짐 등) 던지지 않고 실패 답을 돌려준다.
// 부르는 쪽의 기존 실패 길(빨간 토스트 · dirty 그대로)이 그대로 쓰인다 (설계 2026-10-03 6장).
const UNREACHABLE = "서버에 못 닿았다 — datatool serve 가 꺼졌는지 본다. 고친 것은 화면에 남아 있다";

async function api(path, options = {}) {
  const headers = Object.assign({ "X-Datatool-Token": TOKEN }, options.headers || {});
  let res;
  try {
    res = await fetch(path, Object.assign({}, options, { headers }));
  } catch (err) {
    return { status: 0, body: { ok: false, error: UNREACHABLE } };
  }
  let body = {};
  try {
    body = await res.json();
  } catch (err) {
    body = { ok: false, error: `서버가 JSON 이 아닌 것을 보냈다 (HTTP ${res.status})` };
  }
  return { status: res.status, body };
}

/* 화면 조각 ----------------------------------------------------------- */

let toastTimer = 0;

function toast(message, bad) {
  const el = $("toast");
  el.textContent = message;
  el.classList.toggle("bad", !!bad);
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.hidden = true; }, bad ? 6000 : 3000);
}

function setDirty(on) {
  state.dirty = on;
  $("dirty").hidden = !on;
}

function drawTableList(tables) {
  const list = $("tableList");
  list.textContent = "";
  tables.forEach((t) => {
    const li = document.createElement("li");
    const button = document.createElement("button");
    button.type = "button";
    button.setAttribute("aria-current", String(t.name === state.name));
    const name = document.createElement("span");
    name.textContent = t.name;
    const rows = document.createElement("span");
    rows.className = "rows";
    rows.textContent = t.rows;
    button.append(name, rows);
    button.addEventListener("click", () => openTable(t.name));
    li.append(button);
    list.append(li);
  });
}

function drawProblems(problems, warnings = []) {
  const list = $("problemList");
  $("problemCount").textContent = problems.length + warnings.length;
  list.textContent = "";
  if (problems.length === 0 && warnings.length === 0) {
    const li = document.createElement("li");
    li.className = "empty";
    li.textContent = "문제가 없다.";
    list.append(li);
    return;
  }
  const all = problems.map((p) => [p, false]).concat(warnings.map((p) => [p, true]));
  all.forEach(([p, isWarning]) => {
    const li = document.createElement("li");
    const button = document.createElement("button");
    button.type = "button";
    button.className = isWarning ? "row warn" : "row";
    const where = document.createElement("span");
    where.className = "where";
    // 줄 0 이어도 열이 있으면(스키마 default 검사) 표.열 을 같이 보인다.
    where.textContent = p.line
      ? `${fileName(p.file)}:${p.line} — ${p.table}.${p.column || "행"}`
      : p.column ? `${fileName(p.file)} — ${p.table}.${p.column}` : fileName(p.file);
    const what = document.createElement("span");
    what.className = "what";
    what.textContent = isWarning ? `경고 · ${p.message}` : p.message;
    button.append(where, what);
    button.addEventListener("click", () => jumpTo(p));
    li.append(button);
    list.append(li);
  });
}

const fileName = (path) => String(path || "").split(/[\\/]/).pop();

/* 문제 → 그 칸으로 ---------------------------------------------------- */

async function jumpTo(problem) {
  if (!problem.line && problem.column) {
    toast(`${fileName(problem.file)} 의 ${problem.table}.${problem.column} 이다 — 표가 아니라 스키마에서 고친다`, true);
    return;
  }
  if (problem.table && problem.table !== state.name) {
    await openTable(problem.table);
  }
  const row = state.grid && state.grid.getRow(problem.row);
  if (!row) {
    toast(`${problem.row || "그 행"} 을 표에서 못 찾았다`, true);
    return;
  }
  row.scrollTo();
  const cell = problem.column ? row.getCell(columnField(problem.column)) : null;
  const el = cell ? cell.getElement() : row.getElement();
  el.classList.remove("flash");
  void el.offsetWidth; // 애니메이션을 다시 태우려고 한 번 재우친다
  el.classList.add("flash");
}

function markBad(problems) {
  if (!state.grid) return;
  state.grid.getRows().forEach((row) => {
    row.getCells().forEach((cell) => cell.getElement().classList.remove("bad"));
  });
  problems.forEach((p) => {
    if (p.table !== state.name || !p.column || !p.line) return; // 줄 0(스키마 default)은 표에 칸이 없다
    const row = state.grid.getRow(p.row);
    const cell = row && row.getCell(columnField(p.column));
    if (cell) cell.getElement().classList.add("bad");
  });
}

// columnField 는 문제의 열 자리(`sfx[0]`)에서 원소 꼬리를 떼어 표의 칸 이름(`sfx`)으로 만든다.
const columnField = (column) => String(column).replace(/\[\d+\]$/, "");

/* 열 만들기 ----------------------------------------------------------- */

function columnDefs(columns) {
  const defs = columns.map((col) => {
    const def = {
      field: col.name,
      title: columnTitle(col),
      titleFormatter: "html",
      headerTooltip: col.desc || col.type,
      minWidth: 90,
      editor: "input",
    };
    if (col.isList) return listColumn(def, col);
    switch (col.base) {
      case "int":
      case "float":
        return numberColumn(def, col);
      case "bool":
        return Object.assign(def, {
          editor: "tickCross", formatter: "tickCross", hozAlign: "center", width: 80,
        });
      case "enum":
        return Object.assign(def, {
          editor: "list", editorParams: { values: state.schema.enums[col.enum] || [] },
        });
      case "ref":
        return Object.assign(def, {
          editor: "list",
          editorParams: { values: state.ids.get(col.ref) || [], autocomplete: true, freetext: true },
        });
      case "asset":
        return assetColumn(def, col);
      default:
        return def;
    }
  });
  return defs;
}

function columnTitle(col) {
  const mark = col.required ? '<span class="req" title="필수">*</span>' : "";
  return `${escapeHTML(col.name)}${mark} <span class="type">${escapeHTML(col.type)}</span>`;
}

function numberColumn(def, col) {
  const params = { selectContents: true };
  if (col.min !== undefined) params.min = col.min;
  if (col.max !== undefined) params.max = col.max;
  if (col.base === "int") params.step = 1;
  return Object.assign(def, {
    editor: "number", editorParams: params, sorter: "number",
    hozAlign: "right", cssClass: "num",
  });
}

// list<T> 는 화면에서 쉼표로 이은 한 줄로 다룬다. 배열로 되돌리는 것은 저장할 때다.
// list<asset> 은 편집은 같고 보기만 항목마다 그림·▶ 를 앞에 붙인다.
function listColumn(def, col) {
  if (col.base === "asset") def.formatter = (cell) => assetListCell(cell.getValue());
  return Object.assign(def, {
    editor: "input",
    headerTooltip: `${col.type}${col.kind ? " · " + col.kind : ""} — 쉼표로 나눠 적는다`,
  });
}

/* asset 칸 (연동 설계 3-4 의 UI 칸) ---------------------------------- */

// asset 칸 : 보기는 그림·▶·글자, 편집은 열 kind 로 거른 주소 드롭다운. freetext 라 색인이 낡아도 적힌다.
function assetColumn(def, col) {
  return Object.assign(def, {
    formatter: (cell) => assetCell(cell.getValue()),
    editor: "list",
    editorParams: {
      values: assetChoices(col.kind), autocomplete: true, freetext: true,
      allowEmpty: true, listOnEmpty: true,
    },
    headerTooltip: col.desc || `${col.type}${col.kind ? " · " + col.kind : ""} — Addressables 주소 (하위는 주소[이름])`,
  });
}

// assetChoices 는 드롭다운 값이다. 하위 이름이 있으면 주소[이름] 도 넣는다.
function assetChoices(kind) {
  if (!state.assets || !state.assets.ok) return [];
  const values = new Set();
  state.assets.entries.forEach((e) => {
    if (kind && e.kind !== kind) return;
    values.add(e.address);
    e.sub.forEach((name) => values.add(`${e.address}[${name}]`));
  });
  return [...values];
}

// splitSub 는 서버 SplitSub 와 같다 : 마지막 `[` 에서 가르고 `]` 로 끝나야 한다.
function splitSub(value) {
  const open = value.lastIndexOf("[");
  if (!value.endsWith("]") || open <= 0 || open === value.length - 2) return null;
  return [value.slice(0, open), value.slice(open + 1, -1)];
}

// findAsset 은 칸 값으로 항목과 하위 rect 를 찾는다. 서버의 Find·pickEntry 와 같은 차례다 —
// 같은 주소 여럿이면 미리보기가 되고 (하위면) 그 이름을 가진 항목을 먼저 고른다.
function findAsset(value) {
  const exact = state.assetMap.get(value);
  if (exact) return { entry: exact.find((e) => e.preview !== "none") || exact[0], rect: null };
  const parts = splitSub(value);
  if (!parts) return null;
  const list = state.assetMap.get(parts[0]);
  if (!list) return null;
  for (const e of list) {
    const sub = (e.rects || []).find((r) => r.name === parts[1]);
    if (sub && e.preview !== "none") return { entry: e, rect: sub.rect };
  }
  return { entry: list.find((e) => e.preview !== "none") || list[0], rect: null };
}

function assetCell(value) {
  const box = document.createElement("span");
  box.className = "asset";
  if (value === undefined || value === null || value === "") return box;
  const label = document.createElement("span");
  label.className = "asset-name";
  label.textContent = value;
  const found = state.assets && state.assets.ok ? findAsset(String(value)) : null;
  if (!found) {
    if (state.assets && state.assets.ok && !state.assets.missing) box.classList.add("asset-missing");
    box.append(label);
    return box;
  }
  if (found.entry.preview === "image") {
    box.append(assetThumb(value, found.rect));
  } else if (found.entry.preview === "audio") {
    box.append(assetPlay(value));
  }
  const tag = document.createElement("span");
  tag.className = "kindtag";
  tag.textContent = found.entry.kind;
  box.append(label, tag);
  return box;
}

// list<asset> 칸에 미리보기를 그리는 항목 수. 넘치면 「+N」 을 붙인다.
const ASSET_LIST_PREVIEWS = 3;

// assetListCell 은 항목마다 그림·▶ 를 나란히 두고 칸 글자를 그대로 뒤에 둔다. 못 찾은 항목이 있으면 빨갛게.
function assetListCell(value) {
  const box = document.createElement("span");
  box.className = "asset asset-list";
  if (value === undefined || value === null || value === "") return box;
  const items = listValue(value, "asset");
  const ready = state.assets && state.assets.ok;
  let shown = 0;
  let more = 0;
  let missing = false;
  (Array.isArray(items) ? items : []).forEach((item) => {
    const found = ready && typeof item === "string" && item !== "" ? findAsset(item) : null;
    if (!found) {
      missing = true;
      return;
    }
    const preview = found.entry.preview;
    if (preview !== "image" && preview !== "audio") return;
    if (shown >= ASSET_LIST_PREVIEWS) {
      more += 1;
      return;
    }
    box.append(preview === "image" ? assetThumb(item, found.rect) : assetPlay(item));
    shown += 1;
  });
  if (more > 0) {
    const tag = document.createElement("span");
    tag.className = "asset-more";
    tag.textContent = `+${more}`;
    tag.title = `미리보기 ${more}개 더`;
    box.append(tag);
  }
  if (ready && !state.assets.missing && (missing || !Array.isArray(items))) box.classList.add("asset-missing");
  const label = document.createElement("span");
  label.className = "asset-name";
  label.textContent = value;
  box.append(label);
  return box;
}

// 파일은 토큰 머리를 실어 받아 blob 주소로 둔다. 주소창·DOM 에 토큰을 안 남긴다.
const assetBlobs = new Map();

// 못 받은 것(null)은 캐시하지 않는다 — 색인을 고친 뒤 다시 받을 수 있게.
function assetBlob(address) {
  if (!assetBlobs.has(address)) {
    const pending = fetch(`/api/asset?address=${encodeURIComponent(address)}`, {
      headers: { "X-Datatool-Token": TOKEN },
    }).then((res) => {
      const type = res.headers.get("Content-Type") || "";
      if (!res.ok || type.startsWith("application/json")) return null;
      return res.blob().then((b) => URL.createObjectURL(b));
    }).catch(() => null).then((url) => {
      if (!url && assetBlobs.get(address) === pending) assetBlobs.delete(address);
      return url;
    });
    assetBlobs.set(address, pending);
  }
  return assetBlobs.get(address);
}

// clearAssetBlobs 는 색인을 다시 읽을 때 받아 둔 파일을 버린다.
function clearAssetBlobs() {
  assetBlobs.forEach((pending) => pending.then((url) => { if (url) URL.revokeObjectURL(url); }));
  assetBlobs.clear();
}

// assetThumb 는 32px 그림이다. 하위 에셋이면 rect 칸만 보인다 — rect 의 y 는 아래에서 재므로 뒤집는다.
function assetThumb(value, rect) {
  const SIZE = 32;
  const thumb = document.createElement("span");
  thumb.className = "asset-thumb";
  assetBlob(String(value)).then((url) => {
    if (!url) return;
    const img = new Image();
    img.onload = () => {
      const r = rect || { x: 0, y: 0, w: img.naturalWidth, h: img.naturalHeight };
      const scale = SIZE / Math.max(r.w, r.h, 1);
      const top = img.naturalHeight - (r.y + r.h);
      thumb.style.backgroundImage = `url("${url}")`;
      thumb.style.backgroundSize = `${img.naturalWidth * scale}px ${img.naturalHeight * scale}px`;
      thumb.style.backgroundPosition = `${-r.x * scale}px ${-top * scale}px`;
      thumb.style.width = `${r.w * scale}px`;
      thumb.style.height = `${r.h * scale}px`;
    };
    img.src = url;
  });
  return thumb;
}

// 소리는 <audio> 하나로 튼다. 다른 칸을 누르면 앞 소리는 멈춘다.
const player = { audio: new Audio(), button: null };

function assetPlay(value) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "asset-play";
  button.textContent = "▶";
  button.title = "들어 보기";
  button.addEventListener("click", (e) => {
    e.stopPropagation();
    playAsset(String(value), button);
  });
  return button;
}

async function playAsset(address, button) {
  const same = player.button === button && !player.audio.paused;
  player.audio.pause();
  if (player.button) player.button.textContent = "▶";
  player.button = null;
  if (same) return;
  const url = await assetBlob(address);
  if (!url) {
    toast(`${address} 를 못 받았다`, true);
    return;
  }
  player.audio.src = url;
  player.button = button;
  button.textContent = "■";
  player.audio.onended = () => { button.textContent = "▶"; };
  player.audio.play().catch((err) => toast(`재생 못 함: ${err.message}`, true));
}

// loadAssets 는 asset 열이 있을 때만 색인 요약을 받는다. 부를 때마다 서버가 파일을 다시 읽는다.
async function loadAssets(columns) {
  if (!columns.some((c) => c.base === "asset")) {
    drawAssetBar(null);
    return;
  }
  const res = await api("/api/assetindex");
  clearAssetBlobs();
  state.assets = res.body;
  state.assetMap = new Map();
  (res.body.entries || []).forEach((e) => {
    if (!state.assetMap.has(e.address)) state.assetMap.set(e.address, []);
    state.assetMap.get(e.address).push(e);
  });
  drawAssetBar(res.body);
}

// drawAssetBar 는 색인이 없거나 낡았거나 깨졌을 때 표 위에 한 줄 띠를 그린다.
function drawAssetBar(info) {
  let bar = $("assetBar");
  if (!bar) {
    bar = document.createElement("p");
    bar.id = "assetBar";
    bar.className = "assetbar";
    bar.setAttribute("role", "status");
    document.querySelector(".layout").before(bar);
  }
  const lines = [];
  if (info && !info.ok) lines.push(`AssetTool 색인을 못 읽었다 — ${info.error}`);
  if (info && info.ok && info.missing) lines.push("AssetTool 색인이 없다 — asset 칸을 검사·미리보기 못 한다. assettool index 를 돌린다");
  if (info && info.ok && info.stale) lines.push("AssetTool 색인이 낡았다 — Addressables 설정이 더 새롭다. assettool index 를 다시 돌린다");
  if (info && info.ok) (info.notes || []).forEach((n) => lines.push(n));
  bar.textContent = lines.join(" · ");
  bar.hidden = lines.length === 0;
  bar.classList.toggle("bad", !!info && !info.ok);
}

const escapeHTML = (s) => String(s).replace(/[&<>"]/g, (c) => (
  { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]
));

/* 값 옮기기 (서버 ↔ 화면) --------------------------------------------- */

function toGrid(rows, columns) {
  const lists = columns.filter((c) => c.isList).map((c) => c.name);
  return rows.map((row) => {
    const out = Object.assign({}, row);
    lists.forEach((name) => {
      if (Array.isArray(out[name])) out[name] = listText(out[name]);
    });
    return out;
  });
}

function fromGrid(rows, columns) {
  const byName = new Map(columns.map((c) => [c.name, c]));
  return rows.map((row) => {
    const out = {};
    Object.keys(row).forEach((name) => {
      const value = cellValue(row[name], byName.get(name));
      if (value !== undefined) out[name] = value;
    });
    return out;
  });
}

// cellValue 는 화면의 한 칸을 파일에 적을 값으로 바꾼다.
//
// 규칙 셋이다 (표 위 안내문과 같은 말이다).
//   ① 빈 칸 — **필수 열이면 빈 문자열 그대로 보내** 검증에 걸리게 한다.
//      기본값이 있는 열이면 「열 없음」으로 보내 기본값을 쓰게 한다.
//      둘을 안 가르면 필수 열을 지운 것이 조용히 넘어간다.
//   ② bool — `true`·`false` 만 인정한다. 그 밖은 **원문 그대로** 보내 validate 가 잡게 한다.
//      예전처럼 `value === "true"` 로 접으면 오타 `ture` 가 조용히 false 로 저장된다.
//   ③ list — 쉼표로 쪼개고 앞뒤 공백은 턴다. 빈 원소는 버린다.
//
// UI 가 값을 **고쳐 주지 않는 것**이 핵심이다. 고쳐 주면 검증이 못 보고 파일에 들어간다.
function cellValue(value, col) {
  if (value === undefined || value === null) return undefined;
  if (!col) return value; // 스키마에 없는 열 — 그대로 보내 validate 가 잡게 둔다
  if (col.isList) return listValue(value, col.base);
  if (value === "") return col.required ? "" : undefined;
  if (col.base === "int" || col.base === "float") {
    const n = Number(value);
    return Number.isNaN(n) ? value : n;
  }
  if (col.base === "bool") return boolValue(value);
  return value;
}

// boolValue 는 true·false 만 참거짓으로 받는다. 그 밖은 손대지 않고 그대로 넘긴다.
function boolValue(value) {
  if (value === true || value === false) return value;
  if (value === "true") return true;
  if (value === "false") return false;
  return value;
}

// listText 는 배열을 칸에 보일 글자로 만든다.
//
// 보통은 쉼표로 잇는다. 다만 **쉼표를 품은 원소**나 앞뒤 공백이 있는 원소가 하나라도 있으면
// 쉼표로 이었다가 다시 쪼갤 때 원소가 쪼개져 버리므로 JSON 배열 글자 그대로 보인다.
// 안 건드린 칸의 값이 저장할 때 바뀌는 일을 막는 자리다.
function listText(items) {
  const risky = items.some((v) => typeof v === "string" && (v.includes(",") || v.trim() !== v || v === ""));
  return risky ? JSON.stringify(items) : items.join(", ");
}

function listValue(value, base) {
  if (Array.isArray(value)) return value;
  const text = String(value).trim();
  // listText 가 JSON 으로 내보낸 칸은 JSON 으로 되읽는다. 사람이 직접 적어도 된다.
  if (text.startsWith("[")) {
    try {
      const parsed = JSON.parse(text);
      if (Array.isArray(parsed)) return parsed;
    } catch (e) {
      return value; // 깨진 JSON 은 고쳐 주지 않는다 — 그대로 보내 validate 가 잡게 둔다
    }
  }
  const parts = String(value).split(",").map((s) => s.trim()).filter((s) => s !== "");
  if (base === "int" || base === "float") {
    return parts.map((s) => (Number.isNaN(Number(s)) ? s : Number(s)));
  }
  if (base === "bool") return parts.map(boolValue);
  return parts;
}

/* 붙여넣기 (설계 2026-10-03 6장 · D3) --------------------------------- */

// cleanPaste 는 클립보드 글의 줄끝을 고른다.
//   ① \r\n 과 홀로 선 \r 을 \n 으로 — 안 하면 마지막 칸에 \r 이 붙어 파일까지 간다
//   ② 끝 줄바꿈은 **하나만** 뗀다 — 엑셀이 붙이는 것이 하나다. 둘 이상이면 나머지는 사용자가 고른 빈 행이다
// 칸 안에 줄바꿈이 든 엑셀 따옴표 칸("a\nb")은 안 푼다 (게임 데이터 문자열엔 드물다).
function cleanPaste(text) {
  const lf = String(text).replace(/\r\n?/g, "\n");
  return lf.endsWith("\n") ? lf.slice(0, -1) : lf;
}

// pasteParser 는 줄끝을 고른 뒤 Tabulator 기본 range 파서에 맡긴다.
// 부르는 this 는 Tabulator clipboard 모듈이고, 기본 파서는 그 클래스의 pasteParsers.range 에 있다.
function pasteParser(text) {
  const base = this.constructor.pasteParsers && this.constructor.pasteParsers.range;
  if (!base) {
    toast("붙여넣기 파서를 못 찾았다 — Tabulator 판이 바뀌었는지 본다", true);
    return false;
  }
  const clean = cleanPaste(text);
  state.pasteNote = columnOverflow(this.table, clean);
  return base.call(this, clean);
}

// columnOverflow 는 붙인 글의 열 수가 들어갈 자리보다 많으면 알림 글을, 아니면 "" 를 돌려준다.
// 기본 파서는 남는 열을 말없이 버린다 — 행 넘침과 같은 꼴로 알린다 (D3).
// 들어갈 자리 : 한 칸만 골랐으면 그 칸부터 오른쪽 끝까지 보이는 열, 범위를 골랐으면 범위 너비.
function columnOverflow(table, text) {
  const range = table.modules.selectRange && table.modules.selectRange.activeRange;
  if (!range) return "";
  const bounds = range.getBounds();
  if (!bounds.start) return "";
  const visible = table.columnManager.getVisibleColumnsByIndex();
  const from = visible.indexOf(bounds.start.column);
  if (from < 0) return "";
  const single = bounds.start === bounds.end;
  const room = single ? visible.length - from : visible.indexOf(bounds.end.column) - from + 1;
  const width = Math.max(...text.split("\n").map((line) => line.split("\t").length));
  if (width <= room) return "";
  const where = single ? "표 오른쪽 끝을" : "고른 범위를";
  return `붙인 ${width}열 중 ${width - room}열이 ${where} 넘어 버렸다 — 넘친 열은 안 들어갔다`;
}

// pasteAction 은 기본 range 동작으로 붙인 뒤, 들어가지 못한 행이 있으면 알린다.
// 행을 늘리지는 않는다 — 새 행엔 id 가 있어야 하는데 붙인 칸에 id 가 없으면 지어내야 하고,
// 말없이 행이 늘면 git diff 를 읽는 사람이 놀란다 (「UI 는 값을 고쳐 주지 않는다」).
function pasteAction(rows) {
  const base = this.constructor.pasteActions && this.constructor.pasteActions.range;
  if (!base) {
    toast("붙여넣기 동작을 못 찾았다 — Tabulator 판이 바뀌었는지 본다", true);
    return [];
  }
  const range = this.table.modules.selectRange.activeRange;
  const single = range && range.getBounds().start === range.getBounds().end;
  const updated = base.call(this, rows) || [];
  const lost = rows.length - updated.length;
  const notes = [];
  if (lost > 0) {
    const where = single ? "표 끝을" : "고른 범위를";
    notes.push(`붙인 ${rows.length}행 중 ${lost}행이 ${where} 넘어 버렸다 — 행 추가 뒤 다시 붙여라`);
  }
  if (state.pasteNote) notes.push(state.pasteNote);
  state.pasteNote = "";
  if (notes.length) toast(notes.join(" · "), true);
  return updated;
}

/* 표 열기·저장 -------------------------------------------------------- */

async function openTable(name) {
  if (state.dirty && !confirm("저장 안 한 변경이 있다. 버리고 옮길까?")) return;

  const res = await api(`/api/table/${encodeURIComponent(name)}`);
  if (!res.body.ok) {
    toast(res.body.error || "표를 못 읽었다", true);
    return;
  }
  state.name = name;
  await loadRefIDs(res.body.columns);
  await loadAssets(res.body.columns);

  if (state.grid) state.grid.destroy();
  state.active = null;
  state.activeCell = null;
  state.grid = new Tabulator("#table", {
    data: toGrid(res.body.rows, res.body.columns),
    columns: columnDefs(res.body.columns),
    index: "id",
    height: "100%",
    layout: "fitDataStretch",
    renderVertical: "virtual",
    history: true,
    clipboard: true,
    clipboardCopyRowRange: "range",
    clipboardPasteParser: pasteParser,
    clipboardPasteAction: pasteAction,
    selectableRange: 1,
    selectableRangeColumns: true,
    selectableRangeRows: true,
    selectableRangeClearCells: true,
    // 정렬은 머리의 화살표만 누른다. 머리 글자를 누르면 열 범위가 서는데, 그 클릭이 정렬까지 하면
    // 정렬하려던 클릭이 「전 행 범위」를 남겨 행 지우기가 표 전체를 노린다 (리뷰 B · 조사 후보 9).
    headerSortClickElement: "icon",
    editTriggerEvent: "dblclick",
    rowHeader: { formatter: "rownum", headerSort: false, hozAlign: "center", frozen: true, width: 46, resizable: false },
  });
  state.grid.on("dataChanged", () => setDirty(true));
  // 한 번에 지운 행 묶음은 Ctrl+Z 한 번에 되살린다 (markDeleteGroup).
  // 되살린 행은 Tabulator 가 행 위치 번호를 다시 안 매겨서, 그대로 두면 그 행을 눌러 고른 범위가 비어 버린다
  // (다시 고른 3행을 지우면 1행만 지워졌다 — U6 R2). refreshFilter 가 위치를 다시 매긴다.
  state.grid.on("historyUndo", (type, component, data) => {
    if (state.undoing) return; // 아래 replayGroup 의 반복문이 부른 undo 다
    replayGroup(() => state.grid.undo(), data ? data.groupLeft : 0, type === "rowDelete");
  });
  state.grid.on("historyRedo", (type, component, data) => {
    if (state.undoing) return;
    replayGroup(() => state.grid.redo(), data ? data.groupRight : 0, type === "rowAdd");
  });
  state.grid.on("cellClick", (e, cell) => { state.active = cell.getRow(); state.activeCell = cell; });
  // 범위는 행이 아니라 「몇째 줄」을 기억한다. 정렬하면 그 줄에 딴 행이 와서, 그대로 두면
  // 행 지우기가 누르지 않은 행을 지운다 (U6 R4 에서 drop_1000 을 누르고 정렬했더니 drop_2 가 지워졌다).
  // 정렬 뒤엔 범위를 마지막으로 누른 칸으로 다시 세운다. dataSorted 는 줄 번호를 다시 매기기 전에 오므로 한 박자 미룬다.
  state.grid.on("dataSorted", () => {
    const cell = state.activeCell;
    setTimeout(() => {
      if (cell && state.grid && state.grid.getRow(cell.getRow().getIndex()) && state.grid.addRange) {
        state.grid.addRange(cell, cell);
      }
    }, 0);
  });
  state.grid.columns = res.body.columns;

  setDirty(false);
  await refreshList();
}

// replayGroup 은 묶음 기록 하나를 되돌린(다시 한) 뒤 남은 n 개를 반복문으로 마저 돌린다.
// 재귀로 부르면 2,000행 묶음에서 호출 깊이가 2,000 이 된다. state.undoing 으로 처리기 재진입을 막고,
// 다시 그리기는 끝에 한 번, 위치 다시 매기기(refreshFilter)도 끝에 한 번만 한다.
function replayGroup(step, remaining, renumber) {
  const n = remaining > 0 ? remaining : 0;
  if (n > 0) {
    state.undoing = true;
    state.grid.blockRedraw();
    try {
      for (let i = 0; i < n; i++) step();
    } finally {
      state.grid.restoreRedraw();
      state.undoing = false;
    }
  }
  if (renumber || n > 0) state.grid.refreshFilter();
}

// ref 열이 가리키는 표의 id 목록을 미리 받아 둔다. 드롭다운이 이것으로 뜬다.
// 못 받았으면 캐시에 안 넣는다 — 다음에 표를 열 때 다시 받는다.
async function loadRefIDs(columns) {
  const wanted = [...new Set(columns.filter((c) => c.base === "ref").map((c) => c.ref))];
  for (const name of wanted) {
    if (state.ids.has(name)) continue;
    const res = await api(`/api/table/${encodeURIComponent(name)}`);
    if (res.body.ok) state.ids.set(name, res.body.rows.map((r) => r.id).filter(Boolean));
  }
}

async function refreshList() {
  const res = await api("/api/tables");
  if (res.body.ok) drawTableList(res.body.tables);
}

// commitOpenEditor 는 표 안 편집 칸이 열려 있으면 닫아 입력을 확정한다.
// Tabulator 의 input·number 편집기는 blur 에 값을 넣는다. 다음 프레임까지 기다려야 getData 에 들어간다.
// 안 하면 편집 중 Ctrl+S 가 고친 글자를 빼고 저장하고 dirty 까지 꺼 버린다 (U6 S7).
async function commitOpenEditor() {
  const el = document.activeElement;
  if (!el || !el.closest || !el.closest("#table .tabulator-cell")) return;
  if (!["INPUT", "SELECT", "TEXTAREA"].includes(el.tagName)) return;
  el.blur();
  await new Promise((done) => requestAnimationFrame(() => setTimeout(done, 0)));
}

async function save() {
  if (!state.grid) return;
  await commitOpenEditor();
  const columns = state.grid.columns;
  const rows = fromGrid(state.grid.getData(), columns);
  const res = await api(`/api/table/${encodeURIComponent(state.name)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(rows),
  });
  const warnings = res.body.warnings || [];
  if (res.body.ok) {
    setDirty(false);
    drawProblems([], warnings);
    markBad(warnings);
    state.ids.delete(state.name); // id 가 바뀌었을 수 있다
    const v10 = warnings.some((w) => w.rule === "asset" || w.rule === "asset_kind");
    let message = `${state.name}.json 을 저장했다 (${res.body.rows}행)`;
    if (warnings.length) message += ` — 경고 ${warnings.length}건`;
    if (v10) message += ", asset 주소 경고는 CLI validate 에서는 오류다";
    toast(message, warnings.length > 0);
    await refreshList();
    return;
  }
  const problems = res.body.problems || [];
  drawProblems(problems, warnings);
  markBad(problems);
  toast(problems.length
    ? `검증에 걸려 안 썼다 — ${countText(problems.length, warnings.length)}`
    : (res.body.error || "저장하지 못했다"), true);
}

// countText 는 알림의 건수 글이다. 문제 목록 머리(오류+경고)와 헷갈리지 않게 둘을 나눠 적고, 경고가 없으면 짧게.
const countText = (errors, warns) => (warns ? `오류 ${errors} · 경고 ${warns}` : `오류 ${errors}건`);

async function validateAll() {
  const res = await api("/api/validate", { method: "POST", headers: { "Content-Type": "application/json" } });
  if (res.body.problems === undefined) {
    toast(res.body.error || "검증하지 못했다", true);
    return;
  }
  drawProblems(res.body.problems, res.body.warnings || []);
  markBad(res.body.problems);
  const { tables, rows, errors } = res.body.counts;
  const warns = (res.body.warnings || []).length;
  toast(errors === 0
    ? (warns ? `오류 없다 · 경고 ${warns} — 표 ${tables}개 · ${rows}행` : `문제 없다 — 표 ${tables}개 · ${rows}행`)
    : `${countText(errors, warns)} — 아래 목록을 본다`, errors > 0);
}

function addRow() {
  if (!state.grid) return;
  state.grid.addRow({ id: nextID() }, false).then((row) => {
    row.scrollTo();
    state.active = row;
    // 행 지우기는 범위의 행을 지운다. 범위를 새 행으로 옮겨 「방금 더한 행 지우기」가 그대로 되게 한다.
    const first = row.getCells().find((c) => c.getField());
    if (first && state.grid.addRange) state.grid.addRange(first, first);
    state.activeCell = first || null;
    setDirty(true);
  });
}

function nextID() {
  const used = new Set(state.grid.getData().map((r) => r.id));
  for (let i = 1; ; i++) {
    const id = `${state.name}_${i}`;
    if (!used.has(id)) return id;
  }
}

// deleteRow 는 범위 선택에 든 행을 전부 지운다. 범위가 비었으면 마지막으로 누른 한 행(그것도 없으면 맨 끝 행)을 지운다.
// 저장 전엔 파일이 안 바뀌고 Ctrl+Z 한 번으로 묶음째 되살아난다 (설계 D4).
// 다만 범위가 열 하나 전체(전 행)이거나 DELETE_CONFIRM 행을 넘으면 한 번 묻는다 —
// 머리 클릭 한 번이 표 전체 지우기로 이어지는 사고를 막는다 (리뷰 B).
const DELETE_CONFIRM = 20;

function deleteRow() {
  if (!state.grid) return;
  // 범위는 늘 하나 있다 (안 눌러도 첫 칸에 기본 범위가 선다). 한 행짜리여도 그 범위의 행을 쓴다.
  let rows = rangeRows();
  if (rows.length === 0) {
    const one = state.active || state.grid.getRows().slice(-1)[0];
    rows = one ? [one] : [];
  }
  if (rows.length === 0) return;
  const wholeColumn = rows.length > 1 && rows.length === state.grid.getRows("active").length;
  if ((wholeColumn || rows.length > DELETE_CONFIRM) && !confirm(`${rows.length}행을 지울까?`)) return;
  const first = rows[0].getData().id || "이름 없는 행";

  const history = state.grid.modules && state.grid.modules.history;
  const start = history ? history.index + 1 : -1;
  state.grid.blockRedraw();
  try {
    rows.forEach((row) => row.delete());
  } finally {
    state.grid.restoreRedraw();
  }
  if (history) markDeleteGroup(history.history.slice(start, history.index + 1));

  state.active = null;
  state.activeCell = null;
  setDirty(true);
  toast(rows.length === 1
    ? `${first} 을 지웠다 — 되돌리려면 Ctrl+Z, 저장 전까지는 파일이 안 바뀐다`
    : `${rows.length}행을 지웠다 — 되돌리려면 Ctrl+Z, 저장 전까지는 파일이 안 바뀐다`);
}

// rangeRows 는 범위 선택에 든 행을 겹치지 않게 모은다.
function rangeRows() {
  const seen = new Set();
  const rows = [];
  (state.grid.getRanges ? state.grid.getRanges() : []).forEach((range) => {
    range.getRows().forEach((row) => {
      if (seen.has(row)) return;
      seen.add(row);
      rows.push(row);
    });
  });
  return rows;
}

// markDeleteGroup 은 한 번에 지운 행들의 history 기록에 「앞뒤로 몇 개 더」를 적는다.
// historyUndo·historyRedo 가 이것을 보고 묶음 끝까지 이어서 되돌린다 (openTable 의 on 두 개).
function markDeleteGroup(entries) {
  entries.forEach((entry, i) => {
    if (entry.type !== "rowDelete" || !entry.data) return;
    entry.data.groupLeft = i;
    entry.data.groupRight = entries.length - 1 - i;
  });
}

/* 시작 ---------------------------------------------------------------- */

async function boot() {
  const res = await api("/api/schema");
  if (!res.body.ok) {
    toast(res.body.error || "스키마를 못 읽었다", true);
    return;
  }
  state.schema = res.body;
  $("dataPath").textContent = `${res.body.namespace} · 표 ${res.body.tables.length}개`;

  const tables = res.body.tables.map((t) => t.name);
  if (tables.length === 0) {
    toast("스키마에 표가 하나도 없다", true);
    return;
  }
  await openTable(tables[0]);
}

$("save").addEventListener("click", save);
$("validate").addEventListener("click", validateAll);
$("addRow").addEventListener("click", addRow);
$("delRow").addEventListener("click", deleteRow);

document.addEventListener("keydown", (e) => {
  // e.key 는 한글 자판·Caps Lock 에서 "ㄴ"·"S" 가 된다. 자리(e.code)로 본다.
  if ((e.ctrlKey || e.metaKey) && e.code === "KeyS") {
    e.preventDefault();
    save();
    return;
  }
  // 「행 지우기」 단추를 누른 뒤엔 초점이 표 밖이라 Tabulator 의 Ctrl+Z 가 안 듣는다. 표 밖에서도 되돌린다.
  // 표 안은 Tabulator 가, 글 입력 칸은 브라우저가 맡으므로 건드리지 않는다.
  const outside = !(e.target.closest && e.target.closest(".tabulator, input, select, textarea"));
  if (outside && state.grid && (e.ctrlKey || e.metaKey) && (e.code === "KeyZ" || e.code === "KeyY")) {
    e.preventDefault();
    if (e.code === "KeyZ") state.grid.undo(); else state.grid.redo();
  }
});

window.addEventListener("beforeunload", (e) => {
  if (!state.dirty) return;
  e.preventDefault();
  e.returnValue = "";
});

boot();
