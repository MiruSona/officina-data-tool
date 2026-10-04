/* DataTool 스키마 · Enum 편집 (설계 2026-10-04 5장).
 *
 * GET /api/schema 의 source(파일 꼴)를 사본으로 고치고, 사람이 한 일 중 데이터 행을 따라 바꿔야 하는 것
 * (열 이름 바꾸기·지우기, enum 이름 바꾸기, 값 이름 바꾸기·지우기)만 ops 로 차례대로 쌓는다.
 * 저장은 늘 미리보기(plan)를 거친다 — 바뀔 파일·잃는 값·문제를 보인 뒤에야 PUT(If-Match) 을 보낸다.
 * 서버에서 온 글자는 전부 textContent 로 넣는다 (토큰을 쥔 페이지다).
 * app.js 다음에 읽힌다. api · toast · state · $ · drawProblems · openTable 을 같이 쓴다.
 */
"use strict";

const TYPE_CHOICES = ["string", "int", "float", "bool", "enum", "ref", "asset",
  "list<string>", "list<int>", "list<float>", "list<bool>", "list<enum>", "list<ref>", "list<asset>"];
const KIND_CHOICES = ["image", "audio", "prefab", "scene", "other"];
const RE_LOWER = /^[a-z][a-z0-9_]*$/;      // 열 이름 · enum 값 (서버 reLowerName 과 같다)
const RE_ENUM = /^[A-Za-z][A-Za-z0-9_]*$/; // enum 이름 (서버 reEnumName 과 같다)

const schemaState = {
  tab: "data",
  rev: "",       // 읽어 온 schema.json 의 rev. PUT 의 If-Match 다
  draft: null,   // 고치는 중인 사본 — unpackSource 를 본다
  ops: [],
  dirty: false,
  table: "",     // 스키마 탭에서 고른 표
  enumName: "",  // Enum 탭에서 고른 enum
};

/* 작은 DOM 도우미 ----------------------------------------------------- */

// el 은 글자를 textContent 로만 넣는 요소 만들기다. 서버 글자도 이것으로 넣는다.
function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined && text !== null) node.textContent = String(text);
  return node;
}

function smallButton(text, title, onClick, disabled = false) {
  const b = el("button", "btn mini", text);
  b.type = "button";
  b.title = title;
  b.disabled = disabled;
  b.addEventListener("click", onClick);
  return b;
}

/* source ↔ 사본 ------------------------------------------------------- */

// unpackSource 는 파일 꼴 source 를 고치기 쉬운 꼴로 편다.
//   tables : [{raw, name, columns: [{orig, col}]}] — orig 는 저장된 이름(새 열은 null), col 은 파일 꼴 열 그대로
//   enums  : [{name, orig, top, values: [{name, num, orig}]}] — top 은 저장된 숫자 중 최댓값(지운 숫자 재사용 막기)
// orig 가 있는 것만 ops 를 낸다 — 저장 안 한 새 것의 편집은 op 가 없다 (설계 3-1).
function unpackSource(source) {
  const copy = JSON.parse(JSON.stringify(source));
  const rest = Object.assign({}, copy);
  delete rest.tables;
  delete rest.enums;
  const enums = Object.keys(copy.enums || {}).map((name) => {
    const raw = copy.enums[name];
    const values = Array.isArray(raw)
      ? raw.map((v, i) => ({ name: v, num: i, orig: v }))
      : Object.keys(raw).map((v) => ({ name: v, num: raw[v], orig: v }));
    return { name, orig: name, values, top: Math.max(-1, ...values.map((v) => v.num)) };
  });
  const tables = (copy.tables || []).map((t) => ({
    raw: t,
    name: t.name,
    columns: (t.columns || []).map((c) => ({ orig: c.name, col: c })),
  }));
  return { rest, tables, enums };
}

// packSource 는 사본을 PUT 의 schema 칸으로 되돌린다. enum 은 늘 객체 꼴(숫자 박음)로 보낸다 —
// 숫자가 0,1,2… 면 서버가 배열 꼴로 적는다 (설계 2장).
function packSource(d) {
  const enums = {};
  d.enums.forEach((e) => {
    const obj = {};
    e.values.forEach((v) => { obj[v.name] = v.num; });
    enums[e.name] = obj;
  });
  const tables = d.tables.map((t) => Object.assign({}, t.raw, { name: t.name, columns: t.columns.map((c) => c.col) }));
  return Object.assign({}, d.rest, { enums, tables });
}

const baseOf = (type) => {
  const t = String(type || "");
  return t.startsWith("list<") && t.endsWith(">") ? t.slice(5, -1) : t;
};

/* 읽기 · 상태 --------------------------------------------------------- */

// loadSchemaEdit 는 디스크 스키마를 새로 읽어 사본을 다시 만든다. 고치던 것은 버린다.
async function loadSchemaEdit() {
  const res = await api("/api/schema");
  if (!res.body.ok || !res.body.source) {
    toast(res.body.error || "스키마를 못 읽었다", true);
    return false;
  }
  state.schema = res.body;
  schemaState.rev = res.body.rev;
  schemaState.draft = unpackSource(res.body.source);
  schemaState.ops = [];
  setSchemaDirty(false);
  const names = schemaState.draft.tables.map((t) => t.name);
  if (!names.includes(schemaState.table)) schemaState.table = names[0] || "";
  const enums = schemaState.draft.enums.map((e) => e.name);
  if (!enums.includes(schemaState.enumName)) schemaState.enumName = enums[0] || "";
  renderSchemaView();
  return true;
}

function setSchemaDirty(on) {
  schemaState.dirty = on;
  $("schemaDirty").hidden = !on;
  syncLocks();
}

// edited 는 고친 뒤 부른다. op 가 있으면 쌓고, 다시 그린다.
function edited(op) {
  if (op) schemaState.ops.push(op);
  setSchemaDirty(true);
  renderSchemaView();
}

// schemaBlockReason 은 행 편집을 막아야 할 까닭이다. app.js 의 save·addRow·deleteRow 가 묻는다.
function schemaBlockReason() {
  return schemaState.dirty ? "저장 안 한 스키마 변경이 있다 — 스키마를 저장하거나 「되돌리기」 한 뒤 행을 고친다" : "";
}

// syncLocks 는 두 저장(표 PUT · 스키마 PUT)이 서로 덮지 않게 한쪽이 고쳐져 있으면 다른 쪽을 막는다 (설계 5장).
function syncLocks() {
  const rowsDirty = !!state.dirty;
  const rowLock = $("rowLock");
  const schemaLock = $("schemaLock");
  if (!rowLock || !schemaLock) return;
  rowLock.textContent = schemaBlockReason();
  rowLock.hidden = !schemaState.dirty;
  $("table").classList.toggle("locked", schemaState.dirty);
  ["addRow", "delRow", "save"].forEach((id) => { $(id).disabled = schemaState.dirty; });
  schemaLock.textContent = rowsDirty
    ? "표에 저장 안 한 행 편집이 있다 — 「표」 탭에서 저장하거나 되돌린 뒤 스키마를 고친다 (두 저장이 서로 덮지 않게)"
    : "";
  schemaLock.hidden = !rowsDirty;
  $("schemaFields").disabled = rowsDirty;
  $("preview").disabled = rowsDirty;
}

/* 탭 ------------------------------------------------------------------ */

async function setTab(tab) {
  schemaState.tab = tab;
  document.body.dataset.tab = tab;
  document.querySelectorAll(".tabs [data-tab]").forEach((b) => b.setAttribute("aria-selected", String(b.dataset.tab === tab)));
  $("dataView").hidden = tab !== "data";
  $("schemaView").hidden = tab === "data";
  if (tab === "data") {
    if (state.grid) state.grid.redraw(true);
    return;
  }
  if (!schemaState.draft) await loadSchemaEdit();
  else renderSchemaView();
}

/* 그리기 -------------------------------------------------------------- */

function renderSchemaView() {
  if (!schemaState.draft || schemaState.tab === "data") return;
  const isEnum = schemaState.tab === "enum";
  $("schemaHint").textContent = isEnum
    ? "값을 더하면 C# 숫자는 지금 최댓값+1 로 붙는다 · 지운 숫자는 다시 안 쓴다 · 쓰는 행이 있는 값을 지우려면 대체 값을 고른다"
    : "default 는 JSON 으로 적는다 (따옴표 없이 적으면 글자) · 빈 default = 필수 열 · 표 추가·지우기는 다음 판이다";
  $("opCount").textContent = schemaState.ops.length ? `데이터를 따라 고칠 것 ${schemaState.ops.length}건` : "";
  if (isEnum) renderEnumTab(); else renderSchemaTab();
  syncLocks();
}

function pickList(items, current, onPick) {
  const list = $("pickList");
  list.textContent = "";
  items.forEach((item) => {
    const li = document.createElement("li");
    const button = el("button");
    button.type = "button";
    button.setAttribute("aria-current", String(item.name === current));
    button.append(el("span", "", item.name), el("span", "rows", item.count));
    button.addEventListener("click", () => onPick(item.name));
    li.append(button);
    list.append(li);
  });
}

/* 스키마 탭 ----------------------------------------------------------- */

function renderSchemaTab() {
  const d = schemaState.draft;
  $("pickLabel").textContent = "표";
  $("enumAdd").hidden = true;
  pickList(d.tables.map((t) => ({ name: t.name, count: t.columns.length })), schemaState.table, (name) => {
    schemaState.table = name;
    renderSchemaView();
  });
  const t = d.tables.find((x) => x.name === schemaState.table);
  const main = $("editMain");
  main.textContent = "";
  if (!t) return;
  main.append(el("h2", "edit-title", `${t.name} · 열 ${t.columns.length}개`));

  const grid = el("table", "edit-table");
  grid.id = "columnTable";
  const head = el("tr");
  ["이름", "형", "enum", "ref", "kind", "min", "max", "default", "loc", "설명", ""].forEach((h) => head.append(el("th", "", h)));
  const thead = el("thead");
  thead.append(head);
  const body = el("tbody");
  t.columns.forEach((entry, i) => body.append(columnRow(t, entry, i)));
  grid.append(thead, body);
  main.append(grid);
  main.append(addColumnForm(t));
}

function columnRow(t, entry, i) {
  const c = entry.col;
  const isId = i === 0 && c.name === "id";
  const base = baseOf(c.type);
  const tr = el("tr");
  tr.dataset.column = c.name;
  if (entry.orig === null) tr.classList.add("new");

  const name = textInput(c.name, "열 이름", (v, input) => renameColumn(t, entry, v, input));
  name.disabled = isId;
  name.classList.add("col-name");
  const type = textInput(c.type, "형", (v) => setType(entry, v));
  type.setAttribute("list", "typeChoices");
  type.disabled = isId;
  tr.append(cell(name), cell(type));

  tr.append(cell(base === "enum" ? selectInput(schemaState.draft.enums.map((e) => e.name), c.enum, (v) => setField(c, "enum", v)) : dash()));
  tr.append(cell(base === "ref" ? selectInput(schemaState.draft.tables.map((x) => x.name), c.ref, (v) => setField(c, "ref", v)) : dash()));
  tr.append(cell(base === "asset" ? selectInput(KIND_CHOICES, c.kind, (v) => setField(c, "kind", v), true) : dash()));
  const numeric = base === "int" || base === "float";
  tr.append(cell(numeric ? textInput(numText(c.min), "min", (v) => setNumber(c, "min", v)) : dash()));
  tr.append(cell(numeric ? textInput(numText(c.max), "max", (v) => setNumber(c, "max", v)) : dash()));
  const def = textInput(c.default === undefined ? "" : JSON.stringify(c.default), "default", (v) => setDefault(c, v));
  def.disabled = isId;
  def.placeholder = isId ? "" : "필수";
  tr.append(cell(def));
  const loc = document.createElement("input");
  loc.type = "checkbox";
  loc.checked = !!c.loc;
  loc.title = "loc — 번역할 글자";
  loc.disabled = isId;
  loc.addEventListener("change", () => { if (loc.checked) c.loc = true; else delete c.loc; edited(); });
  tr.append(cell(loc));
  tr.append(cell(textInput(c.desc || "", "설명", (v) => setField(c, "desc", v))));

  const cols = t.columns;
  const tools = el("td", "row-tools");
  tools.append(
    smallButton("↑", "위로", () => moveItem(cols, i, -1), isId || i <= 1),
    smallButton("↓", "아래로", () => moveItem(cols, i, 1), isId || i === cols.length - 1),
    smallButton("지우기", "열 지우기 — 데이터 행의 이 열 값도 지워진다", () => dropColumn(t, entry), isId),
  );
  tr.append(tools);
  return tr;
}

function cell(child) {
  const td = el("td");
  td.append(child);
  return td;
}

const dash = () => el("span", "muted", "—");
const numText = (n) => (n === undefined ? "" : String(n));

// textInput 은 change(Enter·초점 나감)에 한 번 onChange 를 부른다. 글자마다 부르지 않는다.
function textInput(value, label, onChange) {
  const input = document.createElement("input");
  input.type = "text";
  input.value = value;
  input.setAttribute("aria-label", label);
  input.spellcheck = false;
  input.addEventListener("change", () => onChange(input.value.trim(), input));
  return input;
}

function selectInput(choices, value, onChange, allowEmpty = false) {
  const select = document.createElement("select");
  const list = allowEmpty ? [""].concat(choices) : choices.slice();
  if (value && !list.includes(value)) list.push(value);
  list.forEach((v) => {
    const opt = el("option", "", v === "" ? "(없음)" : v);
    opt.value = v;
    select.append(opt);
  });
  select.value = value || "";
  select.addEventListener("change", () => onChange(select.value));
  return select;
}

function setField(c, key, value) {
  if (value === "") delete c[key]; else c[key] = value;
  edited();
}

function setNumber(c, key, text) {
  if (text === "") delete c[key];
  else c[key] = Number.isNaN(Number(text)) ? text : Number(text); // 숫자가 아니면 그대로 보내 서버가 잡게 둔다
  edited();
}

// setDefault : 빈 칸 = 필수(default 없음). JSON 으로 읽히면 그 값, 아니면 글자 그대로.
function setDefault(c, text) {
  if (text === "") {
    delete c.default;
  } else {
    try {
      c.default = JSON.parse(text);
    } catch (e) {
      c.default = text;
    }
  }
  edited();
}

// setType 은 형을 바꾸고, 새 형에 안 맞는 곁칸(enum·ref·kind·min·max)을 뗀다. 값은 안 바꾼다 (결정 8 — 안 맞으면 V3).
function setType(entry, type) {
  const c = entry.col;
  c.type = type;
  const base = baseOf(type);
  if (base !== "enum") delete c.enum;
  else if (!c.enum && schemaState.draft.enums.length) c.enum = schemaState.draft.enums[0].name;
  if (base !== "ref") delete c.ref;
  else if (!c.ref) c.ref = schemaState.draft.tables[0].name;
  if (base !== "asset") delete c.kind;
  if (base !== "int" && base !== "float") { delete c.min; delete c.max; }
  edited();
}

function renameColumn(t, entry, name, input) {
  const from = entry.col.name;
  if (name === from) return;
  const why = !RE_LOWER.test(name) ? "열 이름은 소문자로 시작하고 소문자·숫자·_ 만 쓴다"
    : t.columns.some((x) => x !== entry && x.col.name === name) ? `${t.name} 에 ${name} 열이 이미 있다` : "";
  if (why) {
    toast(why, true);
    input.value = from;
    return;
  }
  entry.col.name = name;
  edited(entry.orig === null ? null : { op: "renameColumn", table: t.name, from, to: name });
}

function dropColumn(t, entry) {
  t.columns.splice(t.columns.indexOf(entry), 1);
  edited(entry.orig === null ? null : { op: "dropColumn", table: t.name, column: entry.col.name });
}

function moveItem(list, i, step) {
  const j = i + step;
  if (j < 0 || j >= list.length) return;
  [list[i], list[j]] = [list[j], list[i]];
  edited();
}

function addColumnForm(t) {
  const form = el("form", "add-form");
  form.id = "addColumn";
  const name = document.createElement("input");
  name.type = "text";
  name.placeholder = "새 열 이름";
  name.setAttribute("aria-label", "새 열 이름");
  name.spellcheck = false;
  const type = selectInput(TYPE_CHOICES, "string", () => {});
  type.setAttribute("aria-label", "새 열 형");
  const add = el("button", "btn", "열 추가");
  add.type = "submit";
  form.append(name, type, add);
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    const n = name.value.trim();
    if (!RE_LOWER.test(n)) { toast("열 이름은 소문자로 시작하고 소문자·숫자·_ 만 쓴다", true); return; }
    if (t.columns.some((x) => x.col.name === n)) { toast(`${t.name} 에 ${n} 열이 이미 있다`, true); return; }
    // 새 열은 기본값을 준다 — 없으면 필수 열이 돼 기존 행이 모두 검증에 걸린다.
    const col = { name: n, type: type.value, default: newDefault(type.value) };
    t.columns.push({ orig: null, col });
    setType(t.columns[t.columns.length - 1], type.value);
  });
  return form;
}

function newDefault(type) {
  if (type.startsWith("list<")) return [];
  switch (type) {
    case "int": case "float": return 0;
    case "bool": return false;
    case "enum": {
      const e = schemaState.draft.enums[0];
      return e && e.values[0] ? e.values[0].name : "";
    }
    default: return "";
  }
}

/* Enum 탭 ------------------------------------------------------------- */

function renderEnumTab() {
  const d = schemaState.draft;
  $("pickLabel").textContent = "Enum";
  $("enumAdd").hidden = false;
  pickList(d.enums.map((e) => ({ name: e.name, count: e.values.length })), schemaState.enumName, (name) => {
    schemaState.enumName = name;
    renderSchemaView();
  });
  const main = $("editMain");
  main.textContent = "";
  const e = d.enums.find((x) => x.name === schemaState.enumName);
  if (!e) {
    main.append(el("p", "muted", "enum 이 없다 — 왼쪽 아래에서 더한다"));
    return;
  }

  const title = el("div", "edit-head");
  const name = textInput(e.name, "enum 이름", (v, input) => renameEnum(e, v, input));
  name.id = "enumName";
  title.append(el("span", "edit-title", "enum"), name);
  if (e.orig === null) title.append(smallButton("enum 지우기", "저장 안 한 새 enum 만 지운다", () => dropNewEnum(e)));
  const users = usersOf(e.name);
  title.append(el("span", "muted", users.length ? `쓰는 열 : ${users.join(", ")}` : "쓰는 열 없음"));
  main.append(title);

  const grid = el("table", "edit-table");
  grid.id = "valueTable";
  const head = el("tr");
  ["값", "C# 숫자", ""].forEach((h) => head.append(el("th", "", h)));
  const thead = el("thead");
  thead.append(head);
  const body = el("tbody");
  e.values.forEach((v, i) => {
    const tr = el("tr");
    tr.dataset.value = v.name;
    if (v.orig === null) tr.classList.add("new");
    const input = textInput(v.name, "값 이름", (text, box) => renameValue(e, v, text, box));
    input.classList.add("value-name");
    tr.append(cell(input), el("td", "num", v.num));
    const tools = el("td", "row-tools");
    tools.append(
      smallButton("↑", "위로", () => moveItem(e.values, i, -1), i === 0),
      smallButton("↓", "아래로", () => moveItem(e.values, i, 1), i === e.values.length - 1),
      smallButton("지우기", "값 지우기", () => askDropValue(e, v)),
    );
    tr.append(tools);
    body.append(tr);
  });
  grid.append(thead, body);
  main.append(grid);

  const form = el("form", "add-form");
  form.id = "addValue";
  const input = document.createElement("input");
  input.type = "text";
  input.placeholder = "새 값 이름";
  input.setAttribute("aria-label", "새 값 이름");
  input.spellcheck = false;
  const add = el("button", "btn", `값 추가 (숫자 ${nextNumber(e)})`);
  add.type = "submit";
  form.append(input, add);
  form.addEventListener("submit", (ev) => {
    ev.preventDefault();
    const n = input.value.trim();
    const why = !RE_LOWER.test(n) ? "enum 값은 소문자로 시작하고 소문자·숫자·_ 만 쓴다"
      : e.values.some((x) => x.name === n) ? `${e.name} 에 ${n} 이 이미 있다` : "";
    if (why) { toast(why, true); return; }
    e.values.push({ name: n, num: nextNumber(e), orig: null });
    edited();
  });
  main.append(form);
}

// nextNumber 는 값을 더할 때 붙일 C# 숫자다 : 저장된 최댓값과 지금 값들 중 큰 것 + 1 (설계 결정 3).
function nextNumber(e) {
  return Math.max(e.top, ...e.values.map((v) => v.num)) + 1;
}

// usersOf 는 이 enum 을 쓰는 열(표.열) 목록이다. 사본 기준이다.
function usersOf(enumName) {
  const out = [];
  schemaState.draft.tables.forEach((t) => t.columns.forEach((c) => {
    if (c.col.enum === enumName) out.push(`${t.name}.${c.col.name}`);
  }));
  return out;
}

function renameEnum(e, name, input) {
  const from = e.name;
  if (name === from) return;
  const why = !RE_ENUM.test(name) ? "enum 이름은 글자로 시작하고 글자·숫자·_ 만 쓴다"
    : schemaState.draft.enums.some((x) => x !== e && x.name === name) ? `enum ${name} 이 이미 있다` : "";
  if (why) {
    toast(why, true);
    input.value = from;
    return;
  }
  // 열이 가리키는 이름은 사본에서 같이 고친다 — 서버는 ops 로 행만 옮기고, 스키마 본문은 보낸 그대로다.
  schemaState.draft.tables.forEach((t) => t.columns.forEach((c) => { if (c.col.enum === from) c.col.enum = name; }));
  e.name = name;
  schemaState.enumName = name;
  edited(e.orig === null ? null : { op: "renameEnum", from, to: name });
}

function renameValue(e, v, name, input) {
  const from = v.name;
  if (name === from) return;
  const why = !RE_LOWER.test(name) ? "enum 값은 소문자로 시작하고 소문자·숫자·_ 만 쓴다"
    : e.values.some((x) => x !== v && x.name === name) ? `${e.name} 에 ${name} 이 이미 있다` : "";
  if (why) {
    toast(why, true);
    input.value = from;
    return;
  }
  v.name = name;
  edited(v.orig === null || e.orig === null ? null : { op: "renameEnumValue", enum: e.name, from, to: name });
}

function dropNewEnum(e) {
  const users = usersOf(e.name);
  if (users.length) { toast(`${users.join(", ")} 가 이 enum 을 쓴다 — 열부터 고친다`, true); return; }
  const list = schemaState.draft.enums;
  list.splice(list.indexOf(e), 1);
  schemaState.enumName = list.length ? list[0].name : "";
  edited();
}

// askDropValue 는 값 지우기 창이다. 저장된 값이면 대체 값을 고를 수 있다 —
// 쓰는 행이 있는데 대체 값이 없으면 미리보기가 막는다 (결정 2).
function askDropValue(e, v) {
  if (v.orig === null || e.orig === null) {
    e.values.splice(e.values.indexOf(v), 1);
    edited();
    return;
  }
  const s = openSheet(`${e.name}.${v.name} 지우기`);
  s.body.append(el("p", "", "이 값을 쓰는 행이 있으면 대체 값이 있어야 저장된다. 쓰는 행이 없으면 (없음) 그대로 둔다."));
  const others = e.values.filter((x) => x !== v).map((x) => x.name);
  const label = el("label", "sheet-field", "쓰던 행은 → ");
  const select = selectInput(others, "", () => {}, true);
  select.id = "replaceWith";
  label.append(select);
  s.body.append(label);
  const ok = el("button", "btn primary", "지우기");
  ok.type = "button";
  ok.id = "dropValueOk";
  ok.addEventListener("click", () => {
    const replaceWith = select.value;
    e.values.splice(e.values.indexOf(v), 1);
    const op = { op: "dropEnumValue", enum: e.name, value: v.name };
    if (replaceWith) op.replaceWith = replaceWith;
    closeSheet();
    edited(op);
  });
  s.buttons.append(ok, sheetClose());
}

function addEnum(name) {
  const why = !RE_ENUM.test(name) ? "enum 이름은 글자로 시작하고 글자·숫자·_ 만 쓴다"
    : schemaState.draft.enums.some((x) => x.name === name) ? `enum ${name} 이 이미 있다` : "";
  if (why) { toast(why, true); return false; }
  schemaState.draft.enums.push({ name, orig: null, values: [], top: -1 });
  schemaState.enumName = name;
  edited();
  return true;
}

/* 창 (미리보기 · 결과) ------------------------------------------------- */

function openSheet(title) {
  const dlg = $("sheet");
  dlg.textContent = "";
  const body = el("div", "sheet-body");
  const buttons = el("div", "sheet-buttons");
  dlg.append(el("h2", "sheet-title", title), body, buttons);
  if (!dlg.open) dlg.showModal();
  return { dlg, body, buttons };
}

function closeSheet() {
  const dlg = $("sheet");
  if (dlg.open) dlg.close();
}

function sheetClose(text = "닫기") {
  const b = el("button", "btn", text);
  b.type = "button";
  b.id = "sheetClose";
  b.addEventListener("click", closeSheet);
  return b;
}

// 창 안 목록 하나에 그리는 줄 수. 쓰는 행이 많은 값을 지우면 문제가 행마다 하나씩 수백 건 온다.
const SHEET_LIST_MAX = 30;

function sheetList(parent, title, items, className, draw) {
  if (!items || items.length === 0) return;
  parent.append(el("h3", "sheet-sub", `${title} ${items.length}`));
  const ul = el("ul", `sheet-list ${className}`);
  items.slice(0, SHEET_LIST_MAX).forEach((item) => {
    const li = el("li");
    draw(li, item);
    ul.append(li);
  });
  if (items.length > SHEET_LIST_MAX) ul.append(el("li", "more", `… 그 밖에 ${items.length - SHEET_LIST_MAX}건`));
  parent.append(ul);
}

// problemLine 은 validate.Problem 한 줄이다. 자리(표.열·줄)가 있으면 앞에 붙인다.
function problemLine(li, p) {
  const where = [p.rule, p.table && p.column ? `${p.table}.${p.column}` : p.table, p.line ? `${fileName(p.file)}:${p.line}` : ""]
    .filter(Boolean).join(" · ");
  if (where) li.append(el("span", "where", where));
  li.append(el("span", "what", p.message));
}

function opText(op) {
  switch (op.op) {
    case "renameColumn": return `열 이름 ${op.table}.${op.from} → ${op.to}`;
    case "dropColumn": return `열 지우기 ${op.table}.${op.column}`;
    case "renameEnumValue": return `값 이름 ${op.enum}.${op.from} → ${op.to}`;
    case "dropEnumValue": return `값 지우기 ${op.enum}.${op.value}${op.replaceWith ? ` — 쓰던 행은 ${op.replaceWith} 로` : ""}`;
    case "renameEnum": return `enum 이름 ${op.from} → ${op.to}`;
    default: return op.op;
  }
}

function schemaBody() {
  return JSON.stringify({ schema: packSource(schemaState.draft), ops: schemaState.ops });
}

// previewSchema 는 plan 을 불러 미리보기 창을 연다. 저장은 이 창의 「저장」 으로만 한다.
async function previewSchema() {
  if (!schemaState.draft || state.dirty) return;
  document.activeElement && document.activeElement.blur && document.activeElement.blur(); // 입력 중 칸을 확정한다
  const res = await api("/api/schema/plan", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: schemaBody(),
  });
  const b = res.body;
  const s = openSheet("스키마 저장 미리보기");
  if (!Array.isArray(b.files)) {
    s.body.append(el("p", "sheet-summary bad", b.error || `미리보기를 못 했다 (HTTP ${res.status})`));
    s.buttons.append(sheetClose());
    return;
  }
  const summary = !b.ok ? `막혔다 — 문제 ${b.problems.length}건. 고친 뒤 다시 미리보기 한다`
    : b.files.length ? `파일 ${b.files.length}개가 바뀐다` : "바뀌는 파일이 없다";
  s.body.append(el("p", `sheet-summary${b.ok ? "" : " bad"}`, summary));
  if (b.rev && b.rev !== schemaState.rev) {
    s.body.append(el("p", "sheet-summary bad", "읽은 뒤 디스크의 schema.json 이 바뀌었다 — 저장하면 막힌다. 「되돌리기」 로 다시 읽는다"));
  }
  drawPlan(s.body, b);
  const save = el("button", "btn primary", "저장");
  save.type = "button";
  save.id = "sheetSave";
  save.disabled = !b.ok || b.files.length === 0;
  save.addEventListener("click", () => putSchema(s));
  s.buttons.append(save, sheetClose());
}

function drawPlan(parent, b) {
  sheetList(parent, "데이터를 따라 고칠 것", schemaState.ops, "plan-ops", (li, op) => li.append(el("span", "what", opText(op))));
  if (b.files && b.files.length) {
    parent.append(el("h3", "sheet-sub", `바뀔 파일 ${b.files.length}`));
    const table = el("table", "plan-files");
    const head = el("tr");
    ["파일", "바뀌는 행", "잃는 값"].forEach((h) => head.append(el("th", "", h)));
    table.append(head);
    b.files.forEach((f) => {
      const tr = el("tr");
      const isSchema = f.file === "schema.json"; // 스키마 파일은 행이 없다
      tr.append(el("td", "", f.file), el("td", "num", isSchema ? "—" : f.rowsChanged),
        el("td", f.valuesLost ? "num bad" : "num", isSchema ? "—" : f.valuesLost));
      table.append(tr);
    });
    parent.append(table);
  }
  sheetList(parent, "문제", b.problems, "plan-problems", problemLine);
  sheetList(parent, "경고", b.warnings, "plan-warnings", problemLine);
  sheetList(parent, "알림", b.notes, "plan-notes", (li, n) => li.append(el("span", "what", n)));
}

// putSchema 는 미리보기 창에서 「저장」 을 눌렀을 때다. 응답 코드마다 창 안에 그대로 알린다 (설계 3-3).
async function putSchema(s) {
  const save = $("sheetSave");
  if (save) save.disabled = true;
  const res = await api("/api/schema", {
    method: "PUT",
    headers: { "Content-Type": "application/json", "If-Match": schemaState.rev },
    body: schemaBody(),
  });
  const b = res.body;
  if (res.status === 200 && b.ok) {
    closeSheet();
    const notes = (b.notes || []).length;
    toast(`스키마를 저장했다 — 파일 ${(b.written || []).length}개${notes ? ` · 알림 ${notes}건` : ""}`);
    await afterSchemaSaved();
    return;
  }
  const out = openSheet("스키마를 저장하지 못했다");
  if (res.status === 409) {
    out.body.append(el("p", "sheet-summary bad", "그사이 디스크의 schema.json 이 바뀌었다 — 아무것도 안 썼다. 「다시 읽기」 는 고친 것을 버리고 디스크 것을 읽는다"));
    const reload = el("button", "btn primary", "다시 읽기");
    reload.type = "button";
    reload.id = "sheetReload";
    reload.addEventListener("click", async () => { closeSheet(); await loadSchemaEdit(); });
    out.buttons.append(reload, sheetClose("고친 것 두고 닫기"));
    return;
  }
  out.body.append(el("p", "sheet-summary bad", b.error || `저장하지 못했다 (HTTP ${res.status})`));
  if (res.status === 500) {
    // 쓰다 멈춤 — 어디까지 썼나를 그대로 보인다. 디스크와 화면이 어긋났으니 다시 읽어야 한다.
    sheetList(out.body, "쓴 파일", b.written, "put-written", (li, f) => li.append(el("span", "what", f)));
    sheetList(out.body, "못 쓴 파일", b.notWritten, "put-not-written", (li, f) => li.append(el("span", "what", f)));
    out.body.append(el("p", "", "파일 일부만 바뀌었다 — git 으로 확인하고, 「다시 읽기」 로 디스크 것을 읽는다"));
    const reload = el("button", "btn", "다시 읽기");
    reload.type = "button";
    reload.addEventListener("click", async () => { closeSheet(); await loadSchemaEdit(); });
    out.buttons.append(reload);
  } else if (Array.isArray(b.files)) {
    drawPlan(out.body, b);
  }
  out.buttons.append(sheetClose());
}

// afterSchemaSaved 는 저장한 스키마로 사본과 표 화면을 다시 세운다. 열 이름이 바뀌었으니 표도 새로 연다.
async function afterSchemaSaved() {
  if (!(await loadSchemaEdit())) return;
  state.ids.clear();
  $("dataPath").textContent = `${state.schema.namespace} · 표 ${state.schema.tables.length}개`;
  const names = state.schema.tables.map((t) => t.name);
  if (names.length) await openTable(names.includes(state.name) ? state.name : names[0]);
}

/* C# 만들기 ----------------------------------------------------------- */

// genCode 는 디스크의 스키마·데이터로 C# 을 만든다 (POST /api/gen). 저장 안 한 편집은 안 들어간다.
async function genCode() {
  const button = $("gen");
  button.disabled = true;
  const res = await api("/api/gen", { method: "POST", headers: { "Content-Type": "application/json" } });
  button.disabled = false;
  const b = res.body;
  const s = openSheet("C# 만들기");
  if (state.dirty || schemaState.dirty) {
    s.body.append(el("p", "sheet-summary warn", "저장 안 한 편집은 안 들어갔다 — 디스크의 스키마·데이터로 만들었다"));
  }
  if (b.ok) {
    s.body.append(el("p", "sheet-summary", `C# 파일 ${(b.written || []).length}개를 썼다`));
    s.body.append(el("p", "path gen-dir", b.dir));
    sheetList(s.body, "쓴 파일", b.written, "gen-written", (li, f) => li.append(el("span", "what", f)));
    sheetList(s.body, "스키마에 없는 옛 파일 (안 지운다 — 사람이 지운다)", b.stale, "gen-stale", (li, f) => li.append(el("span", "what", f)));
    sheetList(s.body, "경고", b.warnings, "plan-warnings", problemLine);
  } else {
    s.body.append(el("p", "sheet-summary bad", b.error || `만들지 못했다 (HTTP ${res.status})`));
    if (res.status === 400 && !b.problems && String(b.error || "").includes("gen 칸")) {
      s.body.append(el("p", "", "데이터 폴더의 .datatool.json 에 \"gen\": \"<C# 을 쓸 폴더>\" 를 적고 datatool serve 를 다시 띄운다. CLI datatool gen 은 --out 으로도 된다"));
    }
    sheetList(s.body, "문제", b.problems, "plan-problems", problemLine);
    sheetList(s.body, "경고", b.warnings, "plan-warnings", problemLine);
    if (b.problems) drawProblems(b.problems, b.warnings || []);
  }
  s.buttons.append(sheetClose());
}

/* 단추 잇기 ----------------------------------------------------------- */

document.querySelectorAll(".tabs [data-tab]").forEach((b) => b.addEventListener("click", () => setTab(b.dataset.tab)));
$("gen").addEventListener("click", genCode);
$("preview").addEventListener("click", previewSchema);
$("resetSchema").addEventListener("click", async () => {
  if (schemaState.dirty && !confirm("저장 안 한 스키마 변경을 버리고 디스크 것을 다시 읽을까?")) return;
  await loadSchemaEdit();
});
$("enumAdd").addEventListener("submit", (e) => {
  e.preventDefault();
  const input = $("enumAddName");
  if (addEnum(input.value.trim())) input.value = "";
});
window.addEventListener("beforeunload", (e) => {
  if (!schemaState.dirty) return;
  e.preventDefault();
  e.returnValue = "";
});
syncLocks();
