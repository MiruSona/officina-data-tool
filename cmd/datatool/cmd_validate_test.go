package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

const validateDataRoot = "../../Testdata/validate"

// validateCaseDir 은 깨끗한 판을 임시 폴더에 깔고 그 위에 깨진 파일만 덮는다.
// 이름이 "" 면 깨끗한 판 그대로다.
func validateCaseDir(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	copyOne(t, dir, filepath.Join(validateDataRoot, "schema.json"))
	for _, file := range []string{"item.json", "monster.json", "drop.json"} {
		copyOne(t, dir, filepath.Join(validateDataRoot, "ok", file))
	}
	if name == "" {
		return dir
	}
	entries, err := os.ReadDir(filepath.Join(validateDataRoot, name))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		copyOne(t, dir, filepath.Join(validateDataRoot, name, entry.Name()))
	}
	return dir
}

func copyOne(t *testing.T, dir, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.Base(path)), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// captureStdout 은 명령이 찍은 것을 받아 온다. --json 꼴을 보려면 실제 출력을 읽어야 한다.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = saved
	_ = w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// 깨끗한 데이터는 종료 0 이고, 깨진 데이터는 종료 2 다.
func TestValidateExitCodes(t *testing.T) {
	cases := []struct {
		name string
		code int
	}{
		{"", exitOK},
		{"many", exitData},
		{"v6-ref", exitData},
		{"v7-dup", exitData},
	}
	for _, c := range cases {
		dir := validateCaseDir(t, c.name)
		if code := run([]string{"validate", "--data", dir}); code != c.code {
			t.Errorf("%q 판의 종료 코드가 %d 다 (기대 %d)", c.name, code, c.code)
		}
	}
}

// 스키마가 틀리면 3, 표 파일이 모자라면 2, JSON 이 깨졌으면 4 다 (설계 5장).
// 고칠 파일이 다르므로 번호가 갈린다.
func TestValidateSeparatesFailureKinds(t *testing.T) {
	schemaBroken := t.TempDir()
	copyOne(t, schemaBroken, "../../Testdata/schema/broken/first-column-not-id.json")
	if err := os.Rename(filepath.Join(schemaBroken, "first-column-not-id.json"),
		filepath.Join(schemaBroken, "schema.json")); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"validate", "--data", schemaBroken}); code != exitSchema {
		t.Errorf("스키마가 틀렸는데 종료 %d 다", code)
	}

	missing := t.TempDir()
	copyOne(t, missing, filepath.Join(validateDataRoot, "schema.json"))
	if code := run([]string{"validate", "--data", missing}); code != exitData {
		t.Errorf("표 파일이 없는데 종료 %d 다", code)
	}

	brokenJSON := validateCaseDir(t, "")
	if err := os.WriteFile(filepath.Join(brokenJSON, "item.json"), []byte("[\n{\"id\":\n]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"validate", "--data", brokenJSON}); code != exitReadFail {
		t.Errorf("JSON 이 깨졌는데 종료 %d 다", code)
	}

	if code := run([]string{"validate", "--data", brokenJSON, "--check"}); code != exitUsage {
		t.Errorf("모르는 인자인데 종료 %d 다", code)
	}
}

// --json 은 한 덩어리 JSON 이다 (설계 5장의 보기).
func TestValidateJSON(t *testing.T) {
	dir := validateCaseDir(t, "many")
	var code int
	out := captureStdout(t, func() {
		code = run([]string{"validate", "--data", dir, "--json"})
	})
	if code != exitData {
		t.Fatalf("종료 코드가 %d 다", code)
	}

	var got struct {
		OK     bool `json:"ok"`
		Exit   int  `json:"exit"`
		Errors []struct {
			File    string `json:"file"`
			Line    int    `json:"line"`
			Table   string `json:"table"`
			Row     string `json:"row"`
			Column  string `json:"column"`
			Rule    string `json:"rule"`
			Message string `json:"message"`
		} `json:"errors"`
		Counts struct {
			Tables int `json:"tables"`
			Rows   int `json:"rows"`
			Errors int `json:"errors"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON 이 아니다: %v\n%s", err, out)
	}
	if got.OK || got.Exit != exitData {
		t.Errorf("ok·exit 이 틀렸다: %+v", got)
	}
	if got.Counts.Tables != 3 || got.Counts.Errors != len(got.Errors) || got.Counts.Errors != 7 {
		t.Errorf("counts 가 틀렸다: %+v", got.Counts)
	}
	first := got.Errors[0]
	if first.File != "item.json" {
		t.Errorf("file 칸이 데이터 폴더 기준 상대경로가 아니다: %q", first.File)
	}
	if first.Line != 8 || first.Column != "id" || first.Rule != "id_form" ||
		first.Table != "item" || first.Row != "Bad_Id" || first.Message == "" {
		t.Errorf("첫 문제의 칸이 틀렸다: %+v", first)
	}

	clean := validateCaseDir(t, "")
	out = captureStdout(t, func() {
		code = run([]string{"validate", "--data", clean, "--json"})
	})
	if code != exitOK {
		t.Fatalf("깨끗한 판의 종료 코드가 %d 다", code)
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON 이 아니다: %v\n%s", err, out)
	}
	if !got.OK || len(got.Errors) != 0 || got.Counts.Rows != 18 {
		t.Errorf("깨끗한 판의 결과가 틀렸다: %+v", got)
	}
}
