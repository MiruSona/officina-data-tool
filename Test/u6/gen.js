// U6 시험 데이터 만들기 — item 2,000 · monster 50 · drop 2,000 행 + 숫자 철자 시험 행(N1).
//
//   node gen.js <출력 폴더> [item 행 수]
//
// 출력 폴더에는 Testdata/table/ok/schema.json 이 먼저 복사돼 있어야 한다 (run.ps1 이 한다).
// 파일은 일부러 fmt 전 꼴로 쓴다. run.ps1 이 datatool fmt 를 돌린 뒤 첫 커밋을 만든다.
"use strict";

const fs = require("fs");
const path = require("path");

const out = process.argv[2];
const N = Number(process.argv[3] || 2000);
if (!out || !fs.existsSync(path.join(out, "schema.json"))) {
  console.error("사용법: node gen.js <schema.json 이 든 폴더> [행 수]");
  process.exit(2);
}

// N1 — 설계 2026-10-03 5장 표 1~12 의 철자. fmt(Go) 가 고친 결과가 JS String(Number(x)) 와 같아야 한다.
const N1_SPELLINGS = [
  "0", "-0", "-0.0", "1", "300.0", "1.25e1", "1.25E1", "0.05", "0.050",
  "1e21", "1e20", "1e-7", "0.000001", "123456789012345680000", "0.30000000000000004",
];

const grades = ["common", "rare", "epic"];
const names = ["철검", "강철검", "단궁", "체력 물약", "마나 물약", "불의 보석", "얼음 지팡이", "가죽 갑옷"];
const tagSets = [["weapon", "melee"], ["weapon", "ranged"], ["consume"], [], ["armor"], ["a,b", "c"]];

const itemLines = [];
const itemIDs = [];
for (let i = 0; i < N; i++) {
  const r = { id: `item_${String(i).padStart(4, "0")}`, name: `${names[i % names.length]} ${i}` };
  if (i % 3) r.atk = (i * 7) % 9999;
  if (i % 4) r.price = [300, 12.5, 0.05, 1200, 99.99][i % 5];
  if (i % 5 === 0) r.usable = true;
  if (i % 6) r.grade = grades[i % 3];
  if (i % 2) r.tags = tagSets[i % tagSets.length];
  itemLines.push(JSON.stringify(r));
  itemIDs.push(r.id);
}
// N1 행은 숫자를 글자 그대로 넣어야 해서 JSON.stringify 를 안 쓴다 (300.0 이 300 으로 바뀌어 버린다).
// 이름 칸에 원래 철자를 적어 둔다 — u6.js 가 그것으로 JS 기대값을 셈한다.
N1_SPELLINGS.forEach((spelling, i) => {
  const id = `n1_${String(i + 1).padStart(2, "0")}`;
  itemLines.push(`{"id":"${id}","name":"N1 ${spelling}","price":${spelling}}`);
});

const monster = [];
for (let i = 0; i < 50; i++) {
  monster.push(JSON.stringify({ id: `mon_${i}`, name: `몬스터 ${i}`, hp: 10 + i * 13, element: i % 2 ? "ice" : "fire" }));
}

const drop = [];
for (let i = 0; i < N; i++) {
  const r = { id: `drop_${i}`, monster_id: `mon_${i % 50}`, item_id: itemIDs[(i * 13) % N], rate: [0.5, 0.01, 0.25, 0.125][i % 4] };
  if (i % 7 === 0) r.counts = [1, 2];
  drop.push(JSON.stringify(r));
}

const write = (name, lines) => fs.writeFileSync(path.join(out, name + ".json"), "[\n" + lines.join(",\n") + "\n]\n");
write("item", itemLines);
write("monster", monster);
write("drop", drop);
console.log(`gen: item ${itemLines.length} (N1 ${N1_SPELLINGS.length} 포함) · monster ${monster.length} · drop ${drop.length}`);
