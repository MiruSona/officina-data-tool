package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig 는 데이터 폴더에 .datatool.json 을 놓는다.
func writeConfig(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// chdir 은 시험 동안만 폴더를 옮긴다. 끝나면 제자리로 돌아온다 —
// 안 돌아오면 t.TempDir 이 쓰던 폴더를 못 지운다.
func chdir(t *testing.T, dir string) {
	t.Helper()
	saved, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(saved) })
}

// --data 를 안 줘도 하위 폴더에서 위로 올라가며 .datatool.json 을 찾는다 (설계 5장).
func TestFindsConfigFromSubdir(t *testing.T) {
	dir := validateCaseDir(t, "")
	writeConfig(t, dir, `{"gen":"Gen","export":"out/game.bytes"}`)
	deep := filepath.Join(dir, "sub", "deep")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, deep)

	if code := run([]string{"validate"}); code != exitOK {
		t.Fatalf("하위 폴더에서 설정을 못 찾았다: 종료 %d", code)
	}
}

// 설정을 못 찾으면 폴더를 지어내지 않고 종료 1 과 안내다.
func TestMissingConfigIsUsageError(t *testing.T) {
	chdir(t, t.TempDir())
	if code := run([]string{"validate"}); code != exitUsage {
		t.Fatalf("설정이 없는데 종료 1 이 아니다: %d", code)
	}
}

// .datatool.json 의 export 자리가 기본 --out 이 되고, 명령 옵션을 주면 그것이 이긴다.
func TestConfigGivesDefaultOut(t *testing.T) {
	dir := validateCaseDir(t, "")
	writeConfig(t, dir, `{"gen":"Gen","export":"out/game.bytes"}`)

	if code := run([]string{"export", "--data", dir}); code != exitOK {
		t.Fatalf("export 가 종료 0 이 아니다: %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "game.bytes")); err != nil {
		t.Fatalf("설정이 가리킨 자리에 안 구웠다: %v", err)
	}

	flagOut := filepath.Join(t.TempDir(), "flag.bytes")
	if code := run([]string{"export", "--data", dir, "--out", flagOut}); code != exitOK {
		t.Fatalf("export 가 종료 0 이 아니다: %d", code)
	}
	if _, err := os.Stat(flagOut); err != nil {
		t.Fatalf("--out 이 설정을 못 이겼다: %v", err)
	}

	if code := run([]string{"gen", "--data", dir}); code != exitOK {
		t.Fatalf("gen 이 종료 0 이 아니다: %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "Gen", "ItemRow.cs")); err != nil {
		t.Fatalf("설정이 가리킨 폴더에 C# 이 안 나왔다: %v", err)
	}
}

// 모르는 칸이 있는 설정은 조용히 넘기지 않고 읽기 실패(4)로 멈춘다.
func TestConfigUnknownKeyFails(t *testing.T) {
	dir := validateCaseDir(t, "")
	writeConfig(t, dir, `{"gen":"Gen","exprot":"out/game.bytes"}`)
	if code := run([]string{"validate", "--data", dir}); code != exitReadFail {
		t.Fatalf("모르는 칸이 종료 4 가 아니다: %d", code)
	}
}

// BOM 이 붙은 설정도 읽는다 — Windows 편집기와 PowerShell 이 붙여서 저장한다.
func TestConfigWithBOM(t *testing.T) {
	dir := validateCaseDir(t, "")
	writeConfig(t, dir, "\xef\xbb\xbf"+`{"export":"out/game.bytes"}`)
	if code := run([]string{"export", "--data", dir}); code != exitOK {
		t.Fatalf("BOM 붙은 설정에서 종료 %d 다", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "game.bytes")); err != nil {
		t.Fatalf("BOM 설정의 자리에 안 구웠다: %v", err)
	}
}
