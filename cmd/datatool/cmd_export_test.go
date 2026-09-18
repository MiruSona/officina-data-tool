package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mirusona/officina-data-tool/internal/bake"
)

// captureJSONOutput 은 --json 으로 찍힌 한 줄을 받아 푼다.
// 표준 출력을 잠깐 파이프로 돌려 놓는다 — 명령이 stdout 에 쓰기 때문이다.
func captureJSONOutput(t *testing.T, args []string) (int, map[string]any) {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = write

	code := run(args)

	os.Stdout = saved
	// 읽기 전에 먼저 닫는다 — 안 그러면 파이프가 안 끝나 읽기가 멈춘다.
	write.Close()
	buf := make([]byte, 64*1024)
	n, _ := read.Read(buf)
	read.Close()

	var got map[string]any
	if err := json.Unmarshal(buf[:n], &got); err != nil {
		t.Fatalf("--json 출력이 JSON 이 아니다: %q", buf[:n])
	}
	return code, got
}

// export 가 파일을 굽고 머리가 규격대로다.
func TestExportWritesFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sub", "gamedata.bytes")
	if code := run([]string{"export", "--data", okDataDir, "--out", out}); code != exitOK {
		t.Fatalf("export 가 종료 0 이 아니다: %d", code)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bake.ReadHeader(data); err != nil {
		t.Fatalf("구운 파일 머리가 규격과 다르다: %v", err)
	}
}

// --json 이 out·bytes·tables·rows·schemaHash 를 다 낸다.
func TestExportJSON(t *testing.T) {
	out := filepath.Join(t.TempDir(), "gamedata.bytes")
	code, got := captureJSONOutput(t, []string{"export", "--data", okDataDir, "--out", out, "--json"})
	if code != exitOK {
		t.Fatalf("종료 코드가 %d 다", code)
	}
	for _, key := range []string{"out", "bytes", "tables", "rows", "schemaHash"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("--json 에 %s 가 없다: %#v", key, got)
		}
	}
	if got["rows"] != float64(18) { // item 6 + monster 4 + drop 8
		t.Fatalf("행 수가 18 이 아니다: %v", got["rows"])
	}
	if got["tables"] != float64(3) {
		t.Fatalf("표 수가 3 이 아니다: %v", got["tables"])
	}
}

// 없는 데이터 폴더는 읽기 실패(4), 모르는 인자는 사용법 잘못(1)이다.
func TestExportExitCodes(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "없는폴더")
	if code := run([]string{"export", "--data", missing}); code != exitReadFail {
		t.Fatalf("없는 폴더가 종료 4 가 아니다: %d", code)
	}
	if code := run([]string{"export", "--data", okDataDir, "--wat"}); code != exitUsage {
		t.Fatalf("모르는 인자가 종료 1 이 아니다: %d", code)
	}
}

// 검증에 걸리는 데이터로 export 하면 종료 2 이고 **파일이 안 생긴다** (설계 5장 「먼저 validate」).
func TestExportStopsOnInvalidData(t *testing.T) {
	dir := validateCaseDir(t, "v6-ref") // ref 가 없는 id 를 가리키는 판
	out := filepath.Join(t.TempDir(), "gamedata.bytes")

	if code := run([]string{"export", "--data", dir, "--out", out}); code != exitData {
		t.Fatalf("검증 실패 데이터가 종료 2 가 아니다: %d", code)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatalf("검증에 걸렸는데 %s 를 구웠다", out)
	}
}
