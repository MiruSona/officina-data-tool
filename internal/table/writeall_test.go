package table

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAllWritesInOrder(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	if err := os.WriteFile(a, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	written, err := WriteAll([]Pending{{Path: a, Data: []byte("A")}, {Path: b, Data: []byte("B")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 2 || written[0] != a || written[1] != b {
		t.Fatalf("쓴 차례가 다르다: %v", written)
	}
	for path, want := range map[string]string{a: "A", b: "B"} {
		got, _ := os.ReadFile(path)
		if string(got) != want {
			t.Fatalf("%s: %q", path, got)
		}
	}
	assertNoTemp(t, dir)
}

// tmp 단계에서 하나라도 실패하면 아무 파일도 안 바뀌고 tmp 도 안 남는다.
func TestWriteAllFailsBeforeAnyRename(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	if err := os.WriteFile(a, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "no-such-dir", "b.json")
	written, err := WriteAll([]Pending{{Path: a, Data: []byte("A")}, {Path: missing, Data: []byte("B")}})
	if err == nil {
		t.Fatal("없는 폴더인데 성공했다")
	}
	typed, ok := err.(*WriteAllError)
	if !ok || len(typed.Written) != 0 || len(written) != 0 {
		t.Fatalf("아무것도 안 썼어야 한다: %v", err)
	}
	if got, _ := os.ReadFile(a); string(got) != "old" {
		t.Fatalf("옛 파일이 바뀌었다: %q", got)
	}
	assertNoTemp(t, dir)
}

func assertNoTemp(t *testing.T, dir string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			t.Fatalf("tmp 가 남았다: %s", e.Name())
		}
	}
}
