package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const okDataDir = "../../Testdata/table/ok"
const messyDataDir = "../../Testdata/table/messy"

// copyDir 은 시험 자료를 임시 폴더로 옮긴다. fmt 는 파일을 쓰는 명령이라
// Testdata 를 직접 건드리게 두면 안 된다.
func copyDir(t *testing.T, from string) string {
	t.Helper()
	to := t.TempDir()
	entries, err := os.ReadDir(from)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(from, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(to, entry.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return to
}

// 이미 규칙대로인 폴더는 --check 가 통과하고 fmt 가 아무것도 안 바꾼다.
func TestFmtOnCleanData(t *testing.T) {
	dir := copyDir(t, okDataDir)
	if code := run([]string{"fmt", "--check", "--data", dir}); code != exitOK {
		t.Fatalf("--check 가 종료 0 이 아니다: %d", code)
	}
	before := mustRead(t, filepath.Join(dir, "item.json"))
	if code := run([]string{"fmt", "--data", dir}); code != exitOK {
		t.Fatalf("fmt 가 종료 0 이 아니다: %d", code)
	}
	if after := mustRead(t, filepath.Join(dir, "item.json")); after != before {
		t.Fatalf("안 고쳐도 될 파일을 고쳤다")
	}
}

// 지저분한 폴더는 --check 가 종료 2 로 막고, fmt 가 고치면 다시 통과한다.
func TestFmtFixesMessyData(t *testing.T) {
	dir := copyDir(t, messyDataDir)
	if code := run([]string{"fmt", "--check", "--data", dir}); code != exitData {
		t.Fatalf("--check 가 종료 2 가 아니다: %d", code)
	}
	// --check 는 한 글자도 안 쓴다.
	if got := mustRead(t, filepath.Join(dir, "item.json")); got != mustRead(t, filepath.Join(messyDataDir, "item.json")) {
		t.Fatalf("--check 가 파일을 고쳤다")
	}

	if code := run([]string{"fmt", "--data", dir}); code != exitOK {
		t.Fatalf("fmt 가 종료 0 이 아니다: %d", code)
	}
	if got, want := mustRead(t, filepath.Join(dir, "item.json")), mustRead(t, filepath.Join(okDataDir, "item.json")); got != want {
		t.Fatalf("정리 결과가 다르다\n== 바란 것 ==\n%s\n== 나온 것 ==\n%s", want, got)
	}
	if code := run([]string{"fmt", "--check", "--data", dir}); code != exitOK {
		t.Fatalf("고친 뒤 --check 가 종료 0 이 아니다: %d", code)
	}
}

// 종료 코드가 고칠 파일을 가리켜야 한다 — 스키마 3 · 데이터 2 · 읽기 4 · 인자 1.
func TestFmtExitCodes(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "GameData")
	if code := run([]string{"fmt", "--data", missing, "--json"}); code != exitReadFail {
		t.Fatalf("없는 폴더인데 종료 %d 다", code)
	}

	broken := copyDir(t, okDataDir)
	if err := os.WriteFile(filepath.Join(broken, "item.json"), []byte("[\n{\"id\":\"a\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"fmt", "--data", broken, "--json"}); code != exitReadFail {
		t.Fatalf("깨진 JSON 인데 종료 %d 다", code)
	}

	extra := copyDir(t, okDataDir)
	if err := os.WriteFile(filepath.Join(extra, "items.json"), []byte("[\n]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"fmt", "--data", extra, "--json"}); code != exitData {
		t.Fatalf("스키마에 없는 표 파일인데 종료 %d 다", code)
	}

	badSchema := copyDir(t, okDataDir)
	if err := os.WriteFile(filepath.Join(badSchema, "schema.json"),
		[]byte(`{"version":1,"namespace":"A.B","tables":[{"name":"item","columns":[{"name":"id","type":"nope"}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"fmt", "--data", badSchema, "--json"}); code != exitSchema {
		t.Fatalf("모르는 타입인데 종료 %d 다", code)
	}

	if code := run([]string{"fmt", "--data", okDataDir, "--nope", "--json"}); code != exitUsage {
		t.Fatalf("모르는 인자인데 종료 %d 다", code)
	}
}

// 스키마에 없는 열(오타)이 있으면 fmt 는 한 글자도 안 쓰고 종료 2 다 (리뷰 D1).
// Format 은 그런 열을 안 적으므로, 그냥 다시 쓰면 오타 난 열의 값이 영영 사라진다.
func TestFmtRefusesUnknownColumn(t *testing.T) {
	dir := copyDir(t, okDataDir)
	path := filepath.Join(dir, "item.json")
	before := mustRead(t, path)
	if err := os.WriteFile(path, []byte(strings.Replace(before,
		`{"id":"sword_iron","name":"철검","atk":12`,
		`{"id":"sword_iron","name":"철검","atkk":12`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	messed := mustRead(t, path)
	if messed == before {
		t.Fatal("시험 자료를 못 고쳤다")
	}

	if code := run([]string{"fmt", "--data", dir}); code != exitData {
		t.Fatalf("스키마에 없는 열인데 종료 %d 다", code)
	}
	if got := mustRead(t, path); got != messed {
		t.Fatalf("막았는데 파일을 고쳤다: %s", got)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
