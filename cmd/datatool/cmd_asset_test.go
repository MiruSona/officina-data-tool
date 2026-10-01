package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mirusona/officina-data-tool/internal/assettest"
	"github.com/mirusona/officina-data-tool/internal/serve"
)

// captureStderr 는 명령이 stderr 에 찍은 것을 받아 온다. 경고 줄을 보려면 읽어야 한다.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// 읽기를 따로 돌린다 — 많이 찍으면 파이프가 차서 fn 이 멈춘다.
	done := make(chan []byte)
	go func() {
		out, _ := io.ReadAll(r)
		done <- out
	}()
	saved := os.Stderr
	os.Stderr = w
	fn()
	os.Stderr = saved
	_ = w.Close()
	return string(<-done)
}

// jsonWarnings 는 --json 결과의 warnings 칸에서 규칙 이름만 뽑는다.
func jsonWarnings(t *testing.T, out string) []string {
	t.Helper()
	var body struct {
		Warnings []struct {
			Rule string `json:"rule"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("--json 이 JSON 이 아니다: %v\n%s", err, out)
	}
	if body.Warnings == nil {
		t.Fatalf("warnings 칸이 없다: %s", out)
	}
	rules := []string{}
	for _, w := range body.Warnings {
		rules = append(rules, w.Rule)
	}
	return rules
}

// A2. 바른 판은 종료 0, 색인에 없는 주소는 종료 2 다.
func TestAssetValidateExitCodes(t *testing.T) {
	l := assettest.New(t)
	if code := run([]string{"validate", "--data", l.GameData}); code != exitOK {
		t.Fatalf("바른 asset 판이 종료 %d 다", code)
	}
	assettest.Write(t, filepath.Join(l.GameData, "card.json"), "[\n"+`{"id":"a","icon":"icns"}`+"\n]\n")
	stderr := captureStderr(t, func() {
		if code := run([]string{"validate", "--data", l.GameData}); code != exitData {
			t.Errorf("없는 주소인데 종료 %d 다", code)
		}
	})
	if !strings.Contains(stderr, "card.json:2: card.icon — ") || !strings.Contains(stderr, `"icons"`) {
		t.Fatalf("오류 줄이 다르다:\n%s", stderr)
	}
}

// A3. 색인이 없으면 경고 한 줄 · 종료 0 · --json 에 warnings. --require-asset-index 면 종료 4.
func TestAssetIndexMissing(t *testing.T) {
	l := assettest.New(t)
	if err := os.Remove(l.Index); err != nil {
		t.Fatal(err)
	}
	stderr := captureStderr(t, func() {
		if code := run([]string{"validate", "--data", l.GameData}); code != exitOK {
			t.Errorf("색인이 없을 뿐인데 종료 %d 다", code)
		}
	})
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "경고: ") || !strings.Contains(lines[0], "assettool index") {
		t.Fatalf("경고 한 줄이 아니다:\n%s", stderr)
	}

	for _, command := range []string{"validate", "export", "gen"} {
		args := []string{command, "--data", l.GameData, "--json"}
		if command != "validate" {
			args = append(args, "--out", filepath.Join(t.TempDir(), "out"))
		}
		out := captureStdout(t, func() {
			if code := run(args); code != exitOK {
				t.Errorf("%s --json 이 종료 %d 다", command, code)
			}
		})
		if rules := jsonWarnings(t, out); strings.Join(rules, ",") != "asset_index" {
			t.Errorf("%s 의 warnings 가 %v 다", command, rules)
		}
		if code := run(append(args, "--require-asset-index")); code != exitReadFail {
			t.Errorf("%s --require-asset-index 인데 종료 %d 다", command, code)
		}
	}
}

// A5. 모르는 계약 버전 · 깨진 JSON · 필수 칸 빠짐은 셋 다 종료 4 (validate·export·gen).
func TestAssetIndexBroken(t *testing.T) {
	raw, err := os.ReadFile(assettest.Path(t, "address-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	bodies := map[string]string{
		"version": strings.Replace(src, `"version": 1`, `"version": 9`, 1),
		"json":    src[:len(src)/2],
		"missing": strings.Replace(src, `"settingsPath"`, `"settingsPathX"`, 1),
	}
	for name, body := range bodies {
		l := assettest.New(t)
		assettest.Write(t, l.Index, body)
		for _, command := range []string{"validate", "export", "gen"} {
			args := []string{command, "--data", l.GameData}
			if command != "validate" {
				args = append(args, "--out", filepath.Join(t.TempDir(), "out"))
			}
			if code := run(args); code != exitReadFail {
				t.Errorf("%s 색인으로 %s 가 종료 %d 다", name, command, code)
			}
		}
	}
}

// A4. 설정 .asset 이 색인보다 새로우면 경고하고, 검사는 그대로 돈다.
func TestAssetIndexStale(t *testing.T) {
	l := assettest.New(t)
	settings := filepath.Join(l.Root, "Assets", "AddressableAssetsData", "AddressableAssetSettings.asset")
	later := time.Date(2100, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(settings, later, later); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if code := run([]string{"validate", "--data", l.GameData, "--json"}); code != exitOK {
			t.Errorf("낡았을 뿐인데 종료 %d 다", code)
		}
	})
	if rules := jsonWarnings(t, out); strings.Join(rules, ",") != "asset_stale" {
		t.Fatalf("낡음 경고가 없다: %v", rules)
	}

	assettest.Write(t, filepath.Join(l.GameData, "card.json"), "[\n"+`{"id":"a","icon":"icns"}`+"\n]\n")
	if code := run([]string{"validate", "--data", l.GameData}); code != exitData {
		t.Fatalf("낡은 색인이어도 검사는 돌아야 한다: 종료 %d", code)
	}
}

// asset 열이 없는 스키마는 색인을 읽지도 않는다 — 깨진 색인을 가리켜도 종료 0.
func TestNoAssetColumnSkipsIndex(t *testing.T) {
	dir := validateCaseDir(t, "")
	broken := filepath.Join(t.TempDir(), "broken.json")
	assettest.Write(t, broken, "{깨짐")
	writeConfig(t, dir, `{"assetIndex":`+jsonString(broken)+`}`)
	stderr := captureStderr(t, func() {
		if code := run([]string{"validate", "--data", dir, "--require-asset-index"}); code != exitOK {
			t.Errorf("asset 열이 없는데 종료 %d 다", code)
		}
	})
	if stderr != "" {
		t.Fatalf("asset 열이 없는데 무언가 찍었다: %s", stderr)
	}
}

// .datatool.json 의 assetIndex : 상대경로는 데이터 폴더 기준, 절대 경로면 stderr 에 한 줄.
func TestConfigAssetIndex(t *testing.T) {
	l := assettest.New(t)
	moved := filepath.Join(l.GameData, "idx", "address-index.json")
	assettest.Copy(t, l.Index, moved)
	if err := os.Remove(l.Index); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, l.GameData, `{"assetIndex":"idx/address-index.json"}`)
	out := captureStdout(t, func() { run([]string{"validate", "--data", l.GameData, "--json"}) })
	if rules := jsonWarnings(t, out); len(rules) != 0 {
		t.Fatalf("옮긴 자리의 색인을 못 읽었다: %v", rules)
	}
	card := filepath.Join(l.GameData, "card.json")
	assettest.Copy(t, card, card+".bak")
	assettest.Write(t, card, "[\n"+`{"id":"a","icon":"icns"}`+"\n]\n")
	if code := run([]string{"validate", "--data", l.GameData}); code != exitData {
		t.Fatalf("옮긴 색인으로 V10 이 안 돌았다: 종료 %d", code)
	}
	assettest.Copy(t, card+".bak", card)
	if err := os.Remove(card + ".bak"); err != nil {
		t.Fatal(err)
	}

	writeConfig(t, l.GameData, `{"assetIndex":`+jsonString(moved)+`}`)
	stderr := captureStderr(t, func() {
		if code := run([]string{"validate", "--data", l.GameData}); code != exitOK {
			t.Errorf("절대 경로 색인으로 종료 %d 다", code)
		}
	})
	if !strings.Contains(stderr, "색인을 읽는다: "+filepath.ToSlash(moved)) {
		t.Fatalf("절대 경로 알림이 없다:\n%s", stderr)
	}
}

func jsonString(s string) string {
	out, _ := json.Marshal(s)
	return string(out)
}

// A9. serve 로 없는 주소를 저장하면 저장은 되고, 같은 파일을 CLI validate 로 보면 종료 2 다.
func TestServeSavedAssetFailsCLI(t *testing.T) {
	l := assettest.New(t)
	server, err := serve.New(l.GameData, l.Index)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/table/card", strings.NewReader(`[{"id":"sword","icon":"icns"}]`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Datatool-Token", server.Token())
	req.Header.Set("Content-Type", "application/json")
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("serve 저장이 %d 다", res.StatusCode)
	}
	if code := run([]string{"validate", "--data", l.GameData}); code != exitData {
		t.Fatalf("serve 가 저장한 틀린 주소를 CLI 가 종료 %d 로 봤다", code)
	}
}
