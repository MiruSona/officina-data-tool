package bake

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mirusona/officina-data-tool/internal/mpack"
	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/table"
)

const okDataDir = "../../Testdata/table/ok"

// 굳은 시각 하나로 굽는다. 시각이 흔들리면 「두 번 구우면 같다」를 볼 수가 없다.
var fixedTime = time.Date(2026, 9, 18, 21, 0, 0, 0, time.UTC)

func loadOK(t *testing.T) (*schema.File, map[string]*table.Table) {
	t.Helper()
	sch, err := schema.Load(filepath.Join(okDataDir, table.SchemaFileName))
	if err != nil {
		t.Fatal(err)
	}
	tables, err := table.LoadAll(okDataDir, sch)
	if err != nil {
		t.Fatal(err)
	}
	return sch, tables
}

func bakeOK(t *testing.T) (*schema.File, []byte) {
	t.Helper()
	sch, tables := loadOK(t)
	data, err := Bake(sch, tables, fixedTime)
	if err != nil {
		t.Fatal(err)
	}
	return sch, data
}

// 머리 16바이트가 규격대로다 (T6).
func TestHeaderBytes(t *testing.T) {
	_, data := bakeOK(t)

	if len(data) <= HeaderSize {
		t.Fatalf("본문이 없다: %d바이트", len(data))
	}
	if string(data[0:4]) != Magic {
		t.Fatalf("매직이 틀렸다: %q", data[0:4])
	}
	if data[4] != FormatVersion {
		t.Fatalf("형식판이 틀렸다: %d", data[4])
	}
	if data[5] != 0 {
		t.Fatalf("1차는 플래그가 0 이어야 한다: 0x%02x", data[5])
	}
	if data[6] != 0 || data[7] != 0 {
		t.Fatalf("예약 [6..7] 이 0 이 아니다")
	}
	for i := 12; i < 16; i++ {
		if data[i] != 0 {
			t.Fatalf("예약 [12..15] 가 0 이 아니다")
		}
	}

	want := uint32(len(data) - HeaderSize)
	got := uint32(data[8]) | uint32(data[9])<<8 | uint32(data[10])<<16 | uint32(data[11])<<24
	if got != want {
		t.Fatalf("본문 길이가 %d 인데 머리는 %d 라고 한다", want, got)
	}

	head, err := ReadHeader(data)
	if err != nil {
		t.Fatal(err)
	}
	if head.Version != FormatVersion || head.Flags != 0 || head.BodyLength != want {
		t.Fatalf("ReadHeader 가 다른 것을 읽었다: %+v", head)
	}
}

// 잘리거나 매직이 틀린 파일은 ReadHeader 가 막는다 — 반쯤 읽지 않는다.
func TestReadHeaderRejectsBroken(t *testing.T) {
	_, data := bakeOK(t)

	if _, err := ReadHeader(data[:8]); err == nil {
		t.Fatalf("짧은 파일을 통과시켰다")
	}
	broken := append([]byte(nil), data...)
	broken[0] = 'X'
	if _, err := ReadHeader(broken); err == nil {
		t.Fatalf("매직이 틀린 파일을 통과시켰다")
	}
	short := append([]byte(nil), data...)
	if _, err := ReadHeader(short[:len(short)-1]); err == nil {
		t.Fatalf("본문 길이가 안 맞는 파일을 통과시켰다")
	}
}

// 본문을 되읽어 _meta 와 표·행 수를 본다 (T6).
func TestBodyMetaAndRowCounts(t *testing.T) {
	sch, data := bakeOK(t)
	root := decodeBody(t, data)

	meta, ok := root[MetaKey].(map[string]any)
	if !ok {
		t.Fatalf("_meta 가 맵이 아니다")
	}
	if meta["schemaHash"] != sch.Hash() {
		t.Fatalf("schemaHash 가 %v 인데 스키마는 %s 다", meta["schemaHash"], sch.Hash())
	}
	if meta["builtAt"] != "2026-09-18T21:00:00Z" {
		t.Fatalf("builtAt 이 틀렸다: %v", meta["builtAt"])
	}

	want := map[string]int{"item": 6, "monster": 4, "drop": 8}
	for name, count := range want {
		rows := rowsOf(t, root, name)
		if len(rows) != count {
			t.Fatalf("표 %s 가 %d행인데 %d행이다", name, count, len(rows))
		}
	}
	// 표 셋 + _meta 말고는 아무것도 안 들어간다.
	if len(root) != len(want)+1 {
		t.Fatalf("본문 맵에 %d개가 들어 있다", len(root))
	}
}

// 행은 스키마 열 차례의 배열이고, 값이 빠진 열은 기본값으로 채워진다 (설계 7장).
func TestRowIsArrayWithDefaults(t *testing.T) {
	sch, data := bakeOK(t)
	root := decodeBody(t, data)
	item := sch.Table("item")

	// sword_iron : grade 를 안 적었으므로 기본값 "common" 이 들어가야 한다.
	row := rowsOf(t, root, "item")[0].([]any)
	if len(row) != len(item.Columns) {
		t.Fatalf("행 길이가 %d 인데 열은 %d 개다", len(row), len(item.Columns))
	}
	if row[item.ColumnIndex("id")] != "sword_iron" {
		t.Fatalf("id 가 틀렸다: %v", row[0])
	}
	if row[item.ColumnIndex("grade")] != "common" {
		t.Fatalf("빠진 enum 열이 기본값으로 안 채워졌다: %v", row[item.ColumnIndex("grade")])
	}
	if row[item.ColumnIndex("usable")] != false {
		t.Fatalf("빠진 bool 열이 기본값으로 안 채워졌다: %v", row[item.ColumnIndex("usable")])
	}

	// gem_fire : tags 를 안 적었으므로 기본값 빈 배열이다.
	last := rowsOf(t, root, "item")[5].([]any)
	tags, ok := last[item.ColumnIndex("tags")].([]any)
	if !ok || len(tags) != 0 {
		t.Fatalf("빠진 list 열이 빈 배열이 아니다: %#v", last[item.ColumnIndex("tags")])
	}

	// potion_hp : atk 를 안 적었으므로 기본값 0 이다.
	potion := rowsOf(t, root, "item")[3].([]any)
	if potion[item.ColumnIndex("atk")] != int64(0) {
		t.Fatalf("빠진 int 열이 기본값 0 이 아니다: %#v", potion[item.ColumnIndex("atk")])
	}
}

// 타입이 제대로 좁혀졌는지 본다 — enum·ref 는 이름 문자열, float 은 float32, int 는 int64.
func TestValueTypes(t *testing.T) {
	sch, data := bakeOK(t)
	root := decodeBody(t, data)

	item := sch.Table("item")
	row := rowsOf(t, root, "item")[1].([]any) // sword_steel
	if v, ok := row[item.ColumnIndex("atk")].(int64); !ok || v != 24 {
		t.Fatalf("int 가 int64 24 가 아니다: %#v", row[item.ColumnIndex("atk")])
	}
	if v, ok := row[item.ColumnIndex("price")].(float32); !ok || v != 900 {
		t.Fatalf("float 이 float32 900 이 아니다: %#v", row[item.ColumnIndex("price")])
	}
	if row[item.ColumnIndex("grade")] != "rare" {
		t.Fatalf("enum 이 이름 문자열이 아니다: %#v", row[item.ColumnIndex("grade")])
	}
	if row[item.ColumnIndex("name")] != "강철검" {
		t.Fatalf("한글 문자열이 깨졌다: %#v", row[item.ColumnIndex("name")])
	}

	drop := sch.Table("drop")
	dropRow := rowsOf(t, root, "drop")[2].([]any) // drop_slime_b_potion
	if dropRow[drop.ColumnIndex("item_id")] != "potion_mp" {
		t.Fatalf("ref 가 id 문자열이 아니다: %#v", dropRow[drop.ColumnIndex("item_id")])
	}
	if v, ok := dropRow[drop.ColumnIndex("rate")].(float32); !ok || v != float32(0.4) {
		t.Fatalf("float32 로 안 낮췄다: %#v", dropRow[drop.ColumnIndex("rate")])
	}
	counts, ok := dropRow[drop.ColumnIndex("counts")].([]any)
	if !ok || len(counts) != 2 || counts[0] != int64(1) || counts[1] != int64(2) {
		t.Fatalf("list<int> 가 틀렸다: %#v", dropRow[drop.ColumnIndex("counts")])
	}
}

// 같은 데이터·같은 시각이면 몇 번을 구워도 바이트가 같다 (T6).
func TestBakeIsDeterministic(t *testing.T) {
	sch, tables := loadOK(t)
	first, err := Bake(sch, tables, fixedTime)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Bake(sch, tables, fixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("두 번 구웠더니 바이트가 다르다 (%d vs %d)", len(first), len(second))
	}
}

// 타입이 안 맞으면 조용히 0 을 넣지 않고 오류를 낸다. 줄 번호·열 이름이 붙는다.
func TestTypeMismatchIsAnError(t *testing.T) {
	sch, tables := loadOK(t)
	tables["item"].Rows[0].Values["atk"] = []byte(`"12"`)

	_, err := Bake(sch, tables, fixedTime)
	if err == nil {
		t.Fatalf("문자열 \"12\" 를 int 로 받아 버렸다")
	}
	typed, ok := err.(*Error)
	if !ok {
		t.Fatalf("*bake.Error 가 아니다: %T", err)
	}
	if typed.Table != "item" || typed.Column != "atk" || typed.Line != 2 {
		t.Fatalf("오류 자리가 틀렸다: %+v", typed)
	}
}

// 필수 열이 비면 막는다 — 배열의 그 자리를 nil 로 채워 C# 이 조용히 null 을 받게 두지 않는다.
func TestMissingRequiredIsAnError(t *testing.T) {
	sch, tables := loadOK(t)
	delete(tables["monster"].Rows[0].Values, "hp")

	if _, err := Bake(sch, tables, fixedTime); err == nil {
		t.Fatalf("필수 열이 빠졌는데 구웠다")
	}
}

func decodeBody(t *testing.T, data []byte) map[string]any {
	t.Helper()
	if _, err := ReadHeader(data); err != nil {
		t.Fatal(err)
	}
	v, err := mpack.Decode(data[HeaderSize:])
	if err != nil {
		t.Fatal(err)
	}
	root, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("본문이 맵이 아니다: %T", v)
	}
	return root
}

func rowsOf(t *testing.T, root map[string]any, name string) []any {
	t.Helper()
	rows, ok := root[name].([]any)
	if !ok {
		t.Fatalf("표 %s 가 배열이 아니다: %T", name, root[name])
	}
	return rows
}

// null 은 값이 아니다. 검증이 이미 막지만 굽는 자리에서도 막는다 (이중 방어 · 리뷰 A).
//
// Go 의 encoding/json 은 null 을 string·bool 에 넣어도 오류를 안 낸다 — 그냥 지나간다.
// 그래서 막지 않으면 `{"name":null}` 이 조용히 "" 로, `{"usable":null}` 이 false 로 구워진다.
func TestNullIsAnError(t *testing.T) {
	cases := map[string]string{"name": "name", "usable": "usable", "atk": "atk", "tags": "tags"}
	for column := range cases {
		t.Run(column, func(t *testing.T) {
			sch, tables := loadOK(t)
			tables["item"].Rows[0].Values[column] = []byte(`null`)

			_, err := Bake(sch, tables, fixedTime)
			if err == nil {
				t.Fatalf("%s 에 null 을 넣었는데 구웠다", column)
			}
			typed, ok := err.(*Error)
			if !ok || typed.Column != column {
				t.Fatalf("오류 자리가 틀렸다: %v", err)
			}
		})
	}
}

// float32 로 못 담는 수는 「수가 아니다」가 아니라 「담을 수 없다」로 알린다 (리뷰 F).
// 고칠 방법이 다르다 — 앞은 오타를 고치는 것이고 뒤는 값을 줄이는 것이다.
func TestFloatOutOfFloat32RangeIsAnError(t *testing.T) {
	sch, tables := loadOK(t)
	tables["item"].Rows[0].Values["price"] = []byte(`1e39`)

	_, err := Bake(sch, tables, fixedTime)
	if err == nil {
		t.Fatal("1e39 를 float32 로 받아 버렸다")
	}
	if !strings.Contains(err.Error(), "float32 로 담을 수 없다") {
		t.Fatalf("메시지가 「담을 수 없다」가 아니다: %v", err)
	}
}

// A8. asset 은 address 문자열로, list<asset> 은 문자열 배열로 굽는다. 빠지면 기본값이다.
func TestAssetIsString(t *testing.T) {
	sch, data := bakeOK(t)
	root := decodeBody(t, data)
	monster := sch.Table("monster")

	first := rowsOf(t, root, "monster")[0].([]any) // slime_green
	if first[monster.ColumnIndex("icon")] != "icons[icon_sword]" {
		t.Fatalf("asset 이 문자열이 아니다: %#v", first[monster.ColumnIndex("icon")])
	}
	sfx, ok := first[monster.ColumnIndex("sfx")].([]any)
	if !ok || len(sfx) != 1 || sfx[0] != "Sfx/hit.wav" {
		t.Fatalf("list<asset> 이 문자열 배열이 아니다: %#v", first[monster.ColumnIndex("sfx")])
	}
	blue := rowsOf(t, root, "monster")[1].([]any)
	if blue[monster.ColumnIndex("icon")] != "" {
		t.Fatalf("빠진 asset 이 기본값 빈 문자열이 아니다: %#v", blue[monster.ColumnIndex("icon")])
	}
}
