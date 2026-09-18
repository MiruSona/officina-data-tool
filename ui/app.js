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
  active: null,       // 마지막으로 누른 행 (행 지우기가 쓴다)
  ids: new Map(),     // ref 용 : 표 이름 → id 목록
};

const $ = (id) => document.getElementById(id);

/* 서버와 말하기 ------------------------------------------------------- */

async function api(path, options = {}) {
  const headers = Object.assign({ "X-Datatool-Token": TOKEN }, options.headers || {});
  const res = await fetch(path, Object.assign({}, options, { headers }));
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

function drawProblems(problems) {
  const list = $("problemList");
  $("problemCount").textContent = problems.length;
  list.textContent = "";
  if (problems.length === 0) {
    const li = document.createElement("li");
    li.className = "empty";
    li.textContent = "문제가 없다.";
    list.append(li);
    return;
  }
  problems.forEach((p) => {
    const li = document.createElement("li");
    const button = document.createElement("button");
    button.type = "button";
    button.className = "row";
    const where = document.createElement("span");
    where.className = "where";
    where.textContent = `${fileName(p.file)}:${p.line} — ${p.table}.${p.column || "행"}`;
    const what = document.createElement("span");
    what.className = "what";
    what.textContent = p.message;
    button.append(where, what);
    button.addEventListener("click", () => jumpTo(p));
    li.append(button);
    list.append(li);
  });
}

const fileName = (path) => String(path || "").split(/[\\/]/).pop();

/* 문제 → 그 칸으로 ---------------------------------------------------- */

async function jumpTo(problem) {
  if (problem.table && problem.table !== state.name) {
    await openTable(problem.table);
  }
  const row = state.grid && state.grid.getRow(problem.row);
  if (!row) {
    toast(`${problem.row || "그 행"} 을 표에서 못 찾았다`, true);
    return;
  }
  row.scrollTo();
  const cell = problem.column ? row.getCell(problem.column) : null;
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
    if (p.table !== state.name || !p.column) return;
    const row = state.grid.getRow(p.row);
    const cell = row && row.getCell(p.column);
    if (cell) cell.getElement().classList.add("bad");
  });
}

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
function listColumn(def, col) {
  return Object.assign(def, {
    editor: "input",
    headerTooltip: `${col.type} — 쉼표로 나눠 적는다`,
  });
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

  if (state.grid) state.grid.destroy();
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
    clipboardPasteParser: "range",
    clipboardPasteAction: "range",
    selectableRange: 1,
    selectableRangeColumns: true,
    selectableRangeRows: true,
    selectableRangeClearCells: true,
    editTriggerEvent: "dblclick",
    rowHeader: { formatter: "rownum", headerSort: false, hozAlign: "center", frozen: true, width: 46, resizable: false },
  });
  state.grid.on("dataChanged", () => setDirty(true));
  state.grid.on("cellClick", (e, cell) => { state.active = cell.getRow(); });
  state.grid.columns = res.body.columns;

  setDirty(false);
  await refreshList();
}

// ref 열이 가리키는 표의 id 목록을 미리 받아 둔다. 드롭다운이 이것으로 뜬다.
async function loadRefIDs(columns) {
  const wanted = [...new Set(columns.filter((c) => c.base === "ref").map((c) => c.ref))];
  for (const name of wanted) {
    if (state.ids.has(name)) continue;
    const res = await api(`/api/table/${encodeURIComponent(name)}`);
    state.ids.set(name, res.body.ok ? res.body.rows.map((r) => r.id).filter(Boolean) : []);
  }
}

async function refreshList() {
  const res = await api("/api/tables");
  if (res.body.ok) drawTableList(res.body.tables);
}

async function save() {
  if (!state.grid) return;
  const columns = state.grid.columns;
  const rows = fromGrid(state.grid.getData(), columns);
  const res = await api(`/api/table/${encodeURIComponent(state.name)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(rows),
  });
  if (res.body.ok) {
    setDirty(false);
    drawProblems([]);
    markBad([]);
    state.ids.delete(state.name); // id 가 바뀌었을 수 있다
    toast(`${state.name}.json 을 저장했다 (${res.body.rows}행)`);
    await refreshList();
    return;
  }
  const problems = res.body.problems || [];
  drawProblems(problems);
  markBad(problems);
  toast(problems.length
    ? `검증에 걸려 안 썼다 — 문제 ${problems.length}건`
    : (res.body.error || "저장하지 못했다"), true);
}

async function validateAll() {
  const res = await api("/api/validate", { method: "POST", headers: { "Content-Type": "application/json" } });
  if (res.body.problems === undefined) {
    toast(res.body.error || "검증하지 못했다", true);
    return;
  }
  drawProblems(res.body.problems);
  markBad(res.body.problems);
  const { tables, rows, errors } = res.body.counts;
  toast(errors === 0
    ? `문제 없다 — 표 ${tables}개 · ${rows}행`
    : `문제 ${errors}건 — 아래 목록을 본다`, errors > 0);
}

function addRow() {
  if (!state.grid) return;
  state.grid.addRow({ id: nextID() }, false).then((row) => {
    row.scrollTo();
    state.active = row;
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

function deleteRow() {
  if (!state.grid) return;
  const row = state.active || state.grid.getRows().slice(-1)[0];
  if (!row) return;
  const id = row.getData().id || "이름 없는 행";
  row.delete();
  state.active = null;
  setDirty(true);
  toast(`${id} 을 지웠다 — 되돌리려면 Ctrl+Z, 저장 전까지는 파일이 안 바뀐다`);
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
  if ((e.ctrlKey || e.metaKey) && e.key === "s") {
    e.preventDefault();
    save();
  }
});

window.addEventListener("beforeunload", (e) => {
  if (!state.dirty) return;
  e.preventDefault();
  e.returnValue = "";
});

boot();
