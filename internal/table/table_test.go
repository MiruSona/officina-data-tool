package table

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirusona/officina-data-tool/internal/schema"
)

const okDir = "../../Testdata/table/ok"
const messyDir = "../../Testdata/table/messy"

func loadSchema(t *testing.T, dir string) *schema.File {
	t.Helper()
	f, err := schema.Load(filepath.Join(dir, SchemaFileName))
	if err != nil {
		t.Fatalf("스키마를 못 읽었다: %v", err)
	}
	return f
}

// 한 줄 한 행 파일은 줄 번호가 곧 행 번호다.
func TestLoadOneRowPerLine(t *testing.T) {
	tbl, err := Load(filepath.Join(okDir, "item.json"))
	if err != nil {
		t.Fatalf("못 읽었다: %v", err)
	}
	if tbl.Name != "item" {
		t.Fatalf("표 이름이 item 이 아니다: %q", tbl.Name)
	}
	if len(tbl.Rows) != 6 {
		t.Fatalf("행이 6개가 아니다: %d", len(tbl.Rows))
	}
	for i, row := range tbl.Rows {
		if want := i + 2; row.Line != want { // 1번 줄은 '['
			t.Fatalf("%d번째 행의 줄 번호가 %d 여야 하는데 %d 다", i, want, row.Line)
		}
	}
	if got := tbl.Rows[0].ID(); got != "sword_iron" {
		t.Fatalf("첫 행 id 가 틀렸다: %q", got)
	}
	if got := tbl.Rows[0].Keys; len(got) != 5 || got[0] != "id" || got[4] != "tags" {
		t.Fatalf("열 차례를 못 지켰다: %v", got)
	}
}

// 들여쓴 JSON 도 읽힌다. 그때 줄 번호는 그 행의 '{' 가 있는 줄이다.
func TestLoadIndentedJSON(t *testing.T) {
	tbl, err := Load(filepath.Join(messyDir, "item.json"))
	if err != nil {
		t.Fatalf("못 읽었다: %v", err)
	}
	want := []int{2, 10, 11, 12, 13, 14}
	for i, line := range want {
		if tbl.Rows[i].Line != line {
			t.Fatalf("%d번째 행의 줄 번호가 %d 여야 하는데 %d 다", i, line, tbl.Rows[i].Line)
		}
	}
}

// 스키마에 없는 열도 읽기 단계에서는 안 버린다. 그걸 잡는 것은 validate 몫이다.
func TestLoadKeepsUnknownColumn(t *testing.T) {
	tbl, err := Parse([]byte("[\n{\"id\":\"a\",\"atkk\":3}\n]\n"), "item.json")
	if err != nil {
		t.Fatalf("못 읽었다: %v", err)
	}
	if _, ok := tbl.Rows[0].Values["atkk"]; !ok {
		t.Fatalf("모르는 열을 버렸다: %v", tbl.Rows[0].Keys)
	}
}

func TestLoadBrokenJSON(t *testing.T) {
	cases := map[string]string{
		"쉼표 빠짐":     "[\n{\"id\":\"a\"}\n{\"id\":\"b\"}\n]\n",
		"배열이 아님":    "{\"id\":\"a\"}\n",
		"행이 객체가 아님": "[\n1,2\n]\n",
		"뒤에 딴 것":    "[\n]\n{}\n",
	}
	for name, content := range cases {
		if _, err := Parse([]byte(content), "item.json"); err == nil {
			t.Fatalf("%s: 오류가 나야 하는데 안 났다", name)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "item.json")); !os.IsNotExist(err) {
		t.Fatalf("없는 파일인데 오류가 %v 다", err)
	}
}

// 규칙대로 적힌 파일은 다시 적어도 바이트가 같다 (T2).
func TestFormatIsStableOnOkFiles(t *testing.T) {
	sch := loadSchema(t, okDir)
	for _, name := range sch.TableNames() {
		tbl, err := Load(filepath.Join(okDir, name+".json"))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out, err := tbl.Format(sch.Table(name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(out) != string(tbl.Raw) {
			t.Fatalf("%s 를 다시 적으니 달라졌다\n== 적힌 것 ==\n%s\n== 다시 적은 것 ==\n%s",
				name, tbl.Raw, out)
		}
	}
}

// 들여쓴·차례가 섞인·기본값이 든·\u 로 적힌 파일을 정리하면 ok 판과 같아진다.
// 열 차례·기본값 빼기·한글 그대로를 한 번에 본다.
func TestFormatFixesMessyFile(t *testing.T) {
	sch := loadSchema(t, messyDir)
	tbl, err := Load(filepath.Join(messyDir, "item.json"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := tbl.Format(sch.Table("item"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(okDir, "item.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(want) {
		t.Fatalf("정리 결과가 다르다\n== 바란 것 ==\n%s\n== 나온 것 ==\n%s", want, out)
	}
}

// 두 번 적어도 같다 (멱등). 한 번 fmt 한 파일이 다음 fmt 에서 또 흔들리면
// git diff 가 매번 지저분해진다.
func TestFormatIsIdempotent(t *testing.T) {
	sch := loadSchema(t, messyDir)
	tbl, err := Load(filepath.Join(messyDir, "item.json"))
	if err != nil {
		t.Fatal(err)
	}
	once, err := tbl.Format(sch.Table("item"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(once, "item.json")
	if err != nil {
		t.Fatal(err)
	}
	twice, err := again.Format(sch.Table("item"))
	if err != nil {
		t.Fatal(err)
	}
	if string(once) != string(twice) {
		t.Fatalf("두 번 적으니 달라졌다\n%s\n%s", once, twice)
	}
}

// 스키마에 없는 열은 안 적는다. 빈 표는 "[\n]\n" 이다.
func TestFormatDropsUnknownColumnAndEmptyTable(t *testing.T) {
	sch := loadSchema(t, okDir)
	tbl, err := Parse([]byte("[\n{\"id\":\"a\",\"atkk\":3}\n]\n"), "item.json")
	if err != nil {
		t.Fatal(err)
	}
	out, err := tbl.Format(sch.Table("item"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "[\n{\"id\":\"a\"}\n]\n" {
		t.Fatalf("모르는 열을 적었다: %q", out)
	}

	empty, err := Parse([]byte("[]"), "item.json")
	if err != nil {
		t.Fatal(err)
	}
	out, err = empty.Format(sch.Table("item"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "[\n]\n" {
		t.Fatalf("빈 표 꼴이 틀렸다: %q", out)
	}
}

func TestWriteFileReplacesAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.json")
	if err := WriteFile(path, []byte("[\n]\n")); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("[\n{\"id\":\"a\"}\n]\n")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "[\n{\"id\":\"a\"}\n]\n" {
		t.Fatalf("덮어쓰기가 안 됐다: %q %v", got, err)
	}
	// tmp 찌꺼기가 남으면 다음 LoadAll 이 「스키마에 없는 표」로 막는다.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("tmp 파일이 남았다: %v", entries)
	}
}

func TestLoadAllReadsEveryTable(t *testing.T) {
	sch := loadSchema(t, okDir)
	tables, err := LoadAll(okDir, sch)
	if err != nil {
		t.Fatalf("못 읽었다: %v", err)
	}
	want := map[string]int{"item": 6, "monster": 4, "drop": 8}
	for name, rows := range want {
		if tables[name] == nil {
			t.Fatalf("%s 를 안 읽었다", name)
		}
		if got := len(tables[name].Rows); got != rows {
			t.Fatalf("%s 행이 %d 이 아니라 %d 다", name, rows, got)
		}
	}
}

// 파일 이름과 표 이름이 어긋나면 막는다 (V8).
func TestLoadAllRejectsFileSetMismatch(t *testing.T) {
	sch := loadSchema(t, okDir)
	dir := t.TempDir()
	for _, name := range []string{"item.json", "drop.json"} {
		copyFile(t, filepath.Join(okDir, name), filepath.Join(dir, name))
	}
	copyFile(t, filepath.Join(okDir, "monster.json"), filepath.Join(dir, "monsters.json"))
	// 표가 아닌 두 파일은 세지 않는다.
	copyFile(t, filepath.Join(okDir, "schema.json"), filepath.Join(dir, SchemaFileName))
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadAll(dir, sch)
	setErr, ok := err.(*SetError)
	if !ok {
		t.Fatalf("*SetError 가 와야 하는데 %v 다", err)
	}
	if len(setErr.Missing) != 1 || setErr.Missing[0] != "monster.json" {
		t.Fatalf("없는 표를 못 집었다: %v", setErr.Missing)
	}
	if len(setErr.Unknown) != 1 || setErr.Unknown[0] != "monsters.json" {
		t.Fatalf("모르는 표를 못 집었다: %v", setErr.Unknown)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// 한 행에 같은 열이 두 번이면 막는다 (리뷰 H).
// encoding/json 은 뒤엣것으로 조용히 덮어써서, 고친 줄이 안 먹은 채 지나간다.
func TestDuplicateColumnInRow(t *testing.T) {
	data := []byte("[\n{\"id\":\"a\",\"atk\":1,\"atk\":2}\n]\n")
	_, err := Parse(data, "item.json")
	if err == nil {
		t.Fatal("같은 열이 두 번인데 통과했다")
	}
	if !strings.Contains(err.Error(), "item.json:2") || !strings.Contains(err.Error(), `"atk"`) {
		t.Fatalf("줄 번호·열 이름이 안 붙었다: %v", err)
	}
}

// BOM 이 붙은 파일도 읽는다 — Windows 편집기·PowerShell 이 붙여서 저장한다 (리뷰 I).
func TestLoadFileWithBOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "item.json")
	raw, err := os.ReadFile(filepath.Join(okDir, "item.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, raw...), 0o644); err != nil {
		t.Fatal(err)
	}

	tbl, err := Load(path)
	if err != nil {
		t.Fatalf("BOM 이 붙었다고 못 읽었다: %v", err)
	}
	if len(tbl.Rows) == 0 {
		t.Fatal("행을 하나도 못 읽었다")
	}
}
