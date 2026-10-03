package serve

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mirusona/officina-data-tool/internal/assettest"
)

// newAssetServer 는 Testdata/asset 판(Unity 뿌리 흉내)에 서버를 띄운다.
func newAssetServer(t *testing.T) (*httptest.Server, *Server, assettest.Layout) {
	t.Helper()
	l := assettest.New(t)
	server, err := New(l.GameData, l.Index)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return ts, server, l
}

// getRaw 는 답을 JSON 으로 풀지 않고 그대로 받는다. 파일 바이트를 보려면 이것을 쓴다.
func getRaw(t *testing.T, ts *httptest.Server, s *Server, path string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s != nil {
		req.Header.Set(tokenHeader, s.Token())
	}
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res, body
}

// makeJunction 은 Windows junction 을 만든다. 못 만들면 시험을 건너뛴다.
func makeJunction(t *testing.T, link, target string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("junction 은 Windows 에만 있다")
	}
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		t.Skipf("junction 을 못 만들었다: %v %s", err, out)
	}
}

// A6 첫 확인. os.Root 가 뿌리 밖을 가리키는 Windows junction 을 막는가 (연동 설계 6절의 미확인).
func TestOSRootBlocksJunction(t *testing.T) {
	l := assettest.New(t)
	outside := t.TempDir()
	assettest.Write(t, filepath.Join(outside, "secret.png"), "비밀")
	makeJunction(t, filepath.Join(l.Root, "Assets", "Link"), outside)

	root, err := os.OpenRoot(l.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	f, err := root.Open("Assets/Link/secret.png")
	if err == nil {
		f.Close()
		t.Fatal("os.Root 가 뿌리 밖 junction 을 열었다")
	}
	t.Logf("os.Root 가 막았다: %v", err)
}

// A5. 깨진 색인 · 모르는 계약 버전이면 /api/assetindex 는 200 + ok:false 다.
func TestAssetIndexAPIBroken(t *testing.T) {
	ts, s, l := newAssetServer(t)
	code, body := call(t, ts, s, http.MethodGet, "/api/assetindex", "")
	if code != http.StatusOK || body["ok"] != true || len(body["entries"].([]any)) != 20 {
		t.Fatalf("바른 색인이 %d %v 다", code, body["ok"])
	}
	for _, broken := range []string{`{"version": 2}`, `{깨짐`, `{"version": 1}`} {
		assettest.Write(t, l.Index, broken)
		code, body = call(t, ts, s, http.MethodGet, "/api/assetindex", "")
		if code != http.StatusOK || body["ok"] != false || body["error"] == "" {
			t.Errorf("%s 색인이 %d %v 다 (200 + ok:false 여야 한다)", broken, code, body)
		}
	}
	if err := os.Remove(l.Index); err != nil {
		t.Fatal(err)
	}
	if code, body = call(t, ts, s, http.MethodGet, "/api/assetindex", ""); body["missing"] != true {
		t.Errorf("색인이 없는데 missing 이 아니다: %d %v", code, body)
	}
}

// 요약 칸 : 미리보기 종류 · 하위 이름 · 썸네일 있음.
func TestAssetIndexAPIEntries(t *testing.T) {
	ts, s, l := newAssetServer(t)
	assettest.Write(t, filepath.Join(l.Root, "Library", "AssetTool", "thumbs", "1234567890abcdef1234567890abcdef.png"), "썸네일")
	_, body := call(t, ts, s, http.MethodGet, "/api/assetindex", "")
	got := map[string]map[string]any{}
	for _, one := range body["entries"].([]any) {
		e := one.(map[string]any)
		got[e["address"].(string)] = e
	}
	if got["icons"]["preview"] != "image" || got["Sfx/hit.wav"]["preview"] != "audio" || got["hero_art"]["preview"] != "none" {
		t.Fatalf("미리보기 종류가 틀렸다: %v %v %v", got["icons"]["preview"], got["Sfx/hit.wav"]["preview"], got["hero_art"]["preview"])
	}
	if subs := got["icons"]["sub"].([]any); len(subs) != 2 || subs[0] != "icon_sword" {
		t.Fatalf("하위 이름이 틀렸다: %v", subs)
	}
	if got["Sfx/hit.wav"]["thumb"] != true || got["icons"]["thumb"] != false {
		t.Fatal("썸네일 있음을 잘못 봤다")
	}
}

// A7. 없는 주소 404 · png 는 image/png + nosniff · psd 는 200 + preview:false · 토큰 없으면 401.
func TestAssetAPI(t *testing.T) {
	ts, s, l := newAssetServer(t)
	want, err := os.ReadFile(filepath.Join(l.Root, "Assets", "Art", "icons.png"))
	if err != nil {
		t.Fatal(err)
	}

	if res, _ := getRaw(t, ts, s, "/api/asset?address=nope"); res.StatusCode != http.StatusNotFound {
		t.Errorf("없는 주소가 %d 다", res.StatusCode)
	}
	for _, address := range []string{"icons", "icons%5Bicon_sword%5D"} {
		res, body := getRaw(t, ts, s, "/api/asset?address="+address)
		if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/png" ||
			res.Header.Get("X-Content-Type-Options") != "nosniff" || string(body) != string(want) {
			t.Errorf("%s png 가 %d %q %q (%d바이트)", address, res.StatusCode,
				res.Header.Get("Content-Type"), res.Header.Get("X-Content-Type-Options"), len(body))
		}
	}
	if res, _ := getRaw(t, ts, s, "/api/asset?address=Sfx/hit.wav"); res.Header.Get("Content-Type") != "audio/wav" {
		t.Errorf("wav 가 %q 다", res.Header.Get("Content-Type"))
	}

	code, body := call(t, ts, s, http.MethodGet, "/api/asset?address=hero_art", "")
	if code != http.StatusOK || body["preview"] != false || !strings.Contains(body["reason"].(string), "psd") {
		t.Errorf("psd 가 %d %v 다", code, body)
	}
	// 같은 주소가 여럿이면 미리보기가 되는 항목을 연다 (dup 의 첫 항목은 prefab).
	if res, body := getRaw(t, ts, s, "/api/asset?address=dup"); res.StatusCode != http.StatusOK || string(body) != string(want) {
		t.Errorf("dup 이 %d (%d바이트) 다 — png 를 열어야 한다", res.StatusCode, len(body))
	}
	// 못 여는 형식은 파일이 없어도 200 + preview:false 다 (설계 차례: 형식 표가 열기보다 앞).
	if err := os.Remove(filepath.Join(l.Root, "Assets", "Art", "hero.psd")); err != nil {
		t.Fatal(err)
	}
	if code, body = call(t, ts, s, http.MethodGet, "/api/asset?address=hero_art", ""); code != http.StatusOK || body["preview"] != false {
		t.Errorf("없는 psd 가 %d %v 다", code, body)
	}
	if code, body = call(t, ts, s, http.MethodGet, "/api/asset?address=pkg_icon", ""); body["preview"] != false {
		t.Errorf("경로 없는 항목이 %d %v 다", code, body)
	}
	if res, _ := getRaw(t, ts, nil, "/api/asset?address=icons"); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("토큰 없이 %d 다", res.StatusCode)
	}
}

// A6. 경로 탈출 다섯 가지 → 403, 파일 바이트를 한 바이트도 안 준다.
func TestAssetAPIPathEscape(t *testing.T) {
	ts, s, l := newAssetServer(t)
	secret := "비밀-한-바이트도-주면-안-된다"
	assettest.Write(t, filepath.Join(filepath.Dir(l.Root), "x.png"), secret)
	assettest.Write(t, filepath.Join(l.Root, "ProjectSettings", "x.png"), secret)
	outside := t.TempDir()
	assettest.Write(t, filepath.Join(outside, "secret.png"), secret)

	cases := []string{"esc_dotdot", "esc_inner", "esc_abs", "esc_prefix"}
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(l.Root, "Assets", "Link"), outside).CombinedOutput(); err == nil {
			cases = append(cases, "esc_link")
		} else {
			t.Logf("junction 을 못 만들어 esc_link 는 건너뜀: %v %s", err, out)
		}
	}
	for _, address := range cases {
		res, body := getRaw(t, ts, s, "/api/asset?address="+address)
		if res.StatusCode != http.StatusForbidden || strings.Contains(string(body), secret) {
			t.Errorf("%s 가 %d 다 (403 이어야 한다): %s", address, res.StatusCode, body)
		}
	}

	// unityRoot 를 뿌리 밖으로 고친 색인 — 맞는 주소여도 403.
	raw, err := os.ReadFile(l.Index)
	if err != nil {
		t.Fatal(err)
	}
	assettest.Write(t, l.Index, strings.Replace(string(raw), `"unityRoot": "../.."`, `"unityRoot": "../../.."`, 1))
	if res, body := getRaw(t, ts, s, "/api/asset?address=icons"); res.StatusCode != http.StatusForbidden || len(body) > 0 && body[0] == 0x89 {
		t.Errorf("뿌리 밖 unityRoot 인데 %d 다", res.StatusCode)
	}
}

// 아틀라스 `주소[이름]` 은 sub.path 파일을 같은 막기로 준다. 없는 sub 는 404, 감옥 밖 sub.path 는 403.
func TestAssetAPIAtlas(t *testing.T) {
	ts, s, l := newAssetServer(t)
	for address, file := range map[string]string{"ui_atlas%5Bbtn_ok%5D": "Sprites/btn_ok.png", "ui_atlas%5Bicon_potion%5D": "icons.png"} {
		want, err := os.ReadFile(filepath.Join(l.Root, "Assets", "Art", filepath.FromSlash(file)))
		if err != nil {
			t.Fatal(err)
		}
		res, body := getRaw(t, ts, s, "/api/asset?address="+address)
		if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/png" ||
			res.Header.Get("X-Content-Type-Options") != "nosniff" || string(body) != string(want) {
			t.Errorf("%s 가 %d %q (%d바이트) 다 — %s 를 줘야 한다", address, res.StatusCode, res.Header.Get("Content-Type"), len(body), file)
		}
	}
	for _, address := range []string{"ui_atlas%5Bnope%5D", "icons%5Bicon_axe%5D"} {
		if res, body := getRaw(t, ts, s, "/api/asset?address="+address); res.StatusCode != http.StatusNotFound {
			t.Errorf("없는 sub %s 가 %d 다: %s", address, res.StatusCode, body)
		}
	}
	if res, _ := getRaw(t, ts, s, "/api/asset?address=ui_atlas%5Bgone%5D"); res.StatusCode != http.StatusNotFound {
		t.Errorf("파일 없는 sub 가 %d 다", res.StatusCode)
	}
	// 맨 아틀라스는 .spriteatlasv2 파일이라 미리보기가 없다.
	if code, body := call(t, ts, s, http.MethodGet, "/api/asset?address=ui_atlas", ""); code != http.StatusOK || body["preview"] != false {
		t.Errorf("맨 아틀라스가 %d %v 다", code, body)
	}
	if res, _ := getRaw(t, ts, nil, "/api/asset?address=ui_atlas%5Bbtn_ok%5D"); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("토큰 없이 %d 다", res.StatusCode)
	}

	secret := "비밀-한-바이트도-주면-안-된다"
	assettest.Write(t, filepath.Join(filepath.Dir(l.Root), "x.png"), secret)
	assettest.Write(t, filepath.Join(l.Root, "ProjectSettings", "x.png"), secret)
	outside := t.TempDir()
	assettest.Write(t, filepath.Join(outside, "secret.png"), secret)
	cases := []string{"dotdot", "inner", "abs", "prefix"}
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(l.Root, "Assets", "Link"), outside).CombinedOutput(); err == nil {
			cases = append(cases, "link")
		} else {
			t.Logf("junction 을 못 만들어 link 는 건너뜀: %v %s", err, out)
		}
	}
	for _, sub := range cases {
		res, body := getRaw(t, ts, s, "/api/asset?address=esc_atlas%5B"+sub+"%5D")
		if res.StatusCode != http.StatusForbidden || strings.Contains(string(body), secret) {
			t.Errorf("esc_atlas[%s] 가 %d 다 (403 이어야 한다): %s", sub, res.StatusCode, body)
		}
	}
}

// /api/assetindex 는 sub 마다 미리보기 종류를 싣는다. 아틀라스 항목은 none 이어도 그 sub 는 image 다.
func TestAssetIndexAPIAtlas(t *testing.T) {
	ts, s, _ := newAssetServer(t)
	_, body := call(t, ts, s, http.MethodGet, "/api/assetindex", "")
	got := map[string]map[string]any{}
	for _, one := range body["entries"].([]any) {
		e := one.(map[string]any)
		got[e["address"].(string)] = e
	}
	atlas := got["ui_atlas"]
	if atlas["preview"] != "none" || atlas["atlas"] != true || got["icons"]["atlas"] != false {
		t.Fatalf("아틀라스 항목 칸이 틀렸다: %v", atlas)
	}
	previews := map[string]any{}
	srcs := map[string]any{}
	for _, one := range atlas["rects"].([]any) {
		r := one.(map[string]any)
		previews[r["name"].(string)] = r["preview"]
		srcs[r["name"].(string)] = r["src"]
		if _, leaked := r["path"]; leaked {
			t.Errorf("sub 경로는 UI 에 안 보낸다: %v", r)
		}
	}
	if previews["btn_ok"] != "image" || previews["icon_potion"] != "image" {
		t.Fatalf("아틀라스 sub 미리보기가 틀렸다: %v", previews)
	}
	// src 는 그림 파일의 guid 다 — 같은 원본을 여러 타일이 쓰면 UI 가 한 번만 받는다. 경로는 안 싣는다.
	if srcs["btn_ok"] != "c0000000000000000000000000000002" || srcs["icon_potion"] != "9a8b7c6d5e4f30211203f4e5d6c7b8a9" {
		t.Fatalf("sub src 가 틀렸다: %v", srcs)
	}
	if r := got["icons"]["rects"].([]any)[1].(map[string]any); r["src"] != "9a8b7c6d5e4f30211203f4e5d6c7b8a9" {
		t.Fatalf("시트 sub 의 src 는 항목 guid 여야 한다: %v", r)
	}
	if r := got["icons"]["rects"].([]any)[0].(map[string]any); r["preview"] != "image" || r["rect"].(map[string]any)["y"] != float64(64) {
		t.Fatalf("시트 sub 칸이 틀렸다: %v", r)
	}
}

// A9. 없는 주소를 넣은 행은 저장되고 응답에 경고가 온다. 같은 파일을 CLI 로 보면 오류다(cmd 시험).
func TestPutAssetWarnsButSaves(t *testing.T) {
	ts, s, l := newAssetServer(t)
	rows := `[{"id":"sword","icon":"icns"},{"id":"hero","icon":"Hero"}]`
	code, body := call(t, ts, s, http.MethodPut, "/api/table/card", rows)
	if code != http.StatusOK {
		t.Fatalf("저장이 %d 다: %v", code, body)
	}
	rules := []string{}
	for _, one := range body["warnings"].([]any) {
		rules = append(rules, one.(map[string]any)["rule"].(string))
	}
	if strings.Join(rules, ",") != "asset,asset_kind" {
		t.Fatalf("경고가 다르다: %v", rules)
	}
	saved, _ := os.ReadFile(filepath.Join(l.GameData, "card.json"))
	if !strings.Contains(string(saved), `"icns"`) {
		t.Fatalf("저장 안 됐다:\n%s", saved)
	}

	// 같은 데이터를 /api/validate 로 보면 CLI 와 같이 오류다.
	code, body = call(t, ts, s, http.MethodPost, "/api/validate", "{}")
	if code != http.StatusOK || body["ok"] != false || len(body["problems"].([]any)) != 2 {
		t.Fatalf("validate 는 V10 을 오류로 봐야 한다: %d %v", code, body)
	}
	// 그 밖의 오류(V3)는 저장 때도 막는다.
	if code, _ = call(t, ts, s, http.MethodPut, "/api/table/card", `[{"id":"sword","icon":3}]`); code != http.StatusBadRequest {
		t.Fatalf("타입 오류인데 %d 다", code)
	}
}

// 리뷰 필수 1 재현 : 없는 주소 101행을 저장해도 400 이 아니고, 경고는 따로 잘린다.
func TestPutManyAssetWarnings(t *testing.T) {
	ts, s, l := newAssetServer(t)
	rows := []string{}
	for i := 0; i < 101; i++ {
		rows = append(rows, fmt.Sprintf(`{"id":"r%d","icon":"nope%d"}`, i, i))
	}
	code, body := call(t, ts, s, http.MethodPut, "/api/table/card", "["+strings.Join(rows, ",")+"]")
	if code != http.StatusOK {
		t.Fatalf("저장이 %d 다: %v", code, body["problems"])
	}
	warnings := body["warnings"].([]any)
	last := warnings[len(warnings)-1].(map[string]any)
	if len(warnings) != 101 || last["rule"] != "too_many" {
		t.Fatalf("경고가 100건 + 「외 1건」 이 아니다: %d건, 끝 %v", len(warnings), last["rule"])
	}
	saved, _ := os.ReadFile(filepath.Join(l.GameData, "card.json"))
	if !strings.Contains(string(saved), `"nope100"`) {
		t.Fatal("저장 안 됐다")
	}
}

// 표 열에 kind 가 실린다. UI 가 드롭다운을 거르는 데 쓴다.
func TestColumnsCarryKind(t *testing.T) {
	ts, s, _ := newAssetServer(t)
	_, body := call(t, ts, s, http.MethodGet, "/api/table/card", "")
	cols := body["columns"].([]any)
	if cols[1].(map[string]any)["kind"] != "image" || cols[3].(map[string]any)["kind"] != nil {
		t.Fatalf("열 kind 가 틀렸다: %v", cols)
	}
}
