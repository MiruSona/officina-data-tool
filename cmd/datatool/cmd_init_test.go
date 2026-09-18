package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mirusona/officina-data-tool/internal/schema"
)

// init 이 만든 예제 스키마가 실제로 검사를 통과해야 한다.
// 여기가 깨지면 새로 시작한 사람이 첫 명령부터 막힌다.
func TestInitMakesValidSchema(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "GameData")
	if code := run([]string{"init", "--data", dir}); code != exitOK {
		t.Fatalf("init 이 실패했다: 종료 %d", code)
	}
	for _, name := range []string{"schema.json", ".datatool.json", "item.json", "drop.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s 가 안 생겼다: %v", name, err)
		}
	}

	f, err := schema.Load(filepath.Join(dir, "schema.json"))
	if err != nil {
		t.Fatalf("예제 스키마가 검사를 통과 못 한다:\n%v", err)
	}
	if f.Table("item") == nil || f.Table("drop") == nil {
		t.Fatalf("예제 표가 모자라다: %v", f.TableNames())
	}
}

// 두 번째 init 은 아무것도 안 덮고 사용법 오류(1)로 멈춘다.
func TestInitDoesNotOverwrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "GameData")
	if code := run([]string{"init", "--data", dir}); code != exitOK {
		t.Fatalf("첫 init 이 실패했다: 종료 %d", code)
	}
	path := filepath.Join(dir, "item.json")
	if err := os.WriteFile(path, []byte("[\n]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"init", "--data", dir}); code != exitUsage {
		t.Fatalf("두 번째 init 이 종료 1 이 아니다: %d", code)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "[\n]\n" {
		t.Fatalf("init 이 있는 파일을 덮었다: %q %v", string(got), err)
	}
}

func TestCommandRouting(t *testing.T) {
	cases := map[string]int{
		"version": exitOK,
		"help":    exitOK,
		"없는명령":    exitUsage,
	}
	for name, want := range cases {
		if got := run([]string{name, "--json"}); got != want {
			t.Fatalf("%s 의 종료 코드가 %d 여야 하는데 %d 다", name, want, got)
		}
	}
}
