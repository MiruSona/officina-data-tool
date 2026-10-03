package serve

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestServer 는 시험 자료를 **사본에** 풀고 서버를 띄운다.
// 원본 Testdata 는 절대 안 건드린다 (PUT 이 파일을 쓴다).
func newTestServer(t *testing.T) (*httptest.Server, *Server, string) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join("..", "..", "Testdata", "table", "ok")
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	server, err := New(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return ts, server, dir
}

func call(t *testing.T, ts *httptest.Server, s *Server, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(tokenHeader, s.Token())
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s %s 의 답이 JSON 이 아니다: %s", method, path, raw)
	}
	return res.StatusCode, out
}

func TestTableListAndRead(t *testing.T) {
	ts, s, _ := newTestServer(t)

	code, body := call(t, ts, s, http.MethodGet, "/api/tables", "")
	if code != http.StatusOK {
		t.Fatalf("목록이 %d 다", code)
	}
	list, _ := body["tables"].([]any)
	if len(list) != 3 {
		t.Fatalf("표가 셋이어야 하는데 %d 개다", len(list))
	}

	code, body = call(t, ts, s, http.MethodGet, "/api/table/item", "")
	if code != http.StatusOK {
		t.Fatalf("item 읽기가 %d 다", code)
	}
	rows, _ := body["rows"].([]any)
	if len(rows) != 6 {
		t.Fatalf("item 이 6행이어야 하는데 %d 행이다", len(rows))
	}
	// 기본값이 빠진 열을 서버가 채워 주는가 (sword_iron 의 grade 는 파일에 없다).
	first, _ := rows[0].(map[string]any)
	if first["grade"] != "common" {
		t.Fatalf("기본값을 안 채웠다: %v", first["grade"])
	}
}

func TestPutWritesFormatted(t *testing.T) {
	ts, s, dir := newTestServer(t)
	path := filepath.Join(dir, "monster.json")
	before, _ := os.ReadFile(path)

	// 열 차례를 일부러 섞고 기본값(hp 는 기본값이 없으니 element)을 넣어 보낸다.
	rows := `[{"name":"초록 슬라임","sfx":["Sfx/hit.wav"],"id":"slime_green","hp":30,"icon":"icons[icon_sword]"},
	{"id":"slime_blue","name":"파랑 슬라임","hp":45,"element":"ice"},
	{"id":"wolf_gray","name":"잿빛 늑대","hp":120},
	{"id":"golem_fire","name":"불의 골렘","hp":900}]`
	code, body := call(t, ts, s, http.MethodPut, "/api/table/monster", rows)
	if code != http.StatusOK {
		t.Fatalf("저장이 %d 다: %v", code, body)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// 같은 내용을 보냈으니 fmt 규칙대로 쓰면 바이트가 원래와 같아야 한다.
	if !bytes.Equal(before, after) {
		t.Fatalf("저장한 파일이 fmt 규칙과 다르다:\n원본:\n%s\n쓴 것:\n%s", before, after)
	}
}

func TestPutRejectsInvalid(t *testing.T) {
	ts, s, dir := newTestServer(t)
	path := filepath.Join(dir, "item.json")
	before, _ := os.ReadFile(path)

	// atk 가 문자열이다 (V3). 한 글자도 쓰면 안 된다.
	rows := `[{"id":"sword_iron","name":"철검","atk":"12"}]`
	code, body := call(t, ts, s, http.MethodPut, "/api/table/item", rows)
	if code != http.StatusBadRequest {
		t.Fatalf("검증 실패인데 %d 다: %v", code, body)
	}
	problems, _ := body["problems"].([]any)
	if len(problems) == 0 {
		t.Fatalf("문제 목록이 비었다: %v", body)
	}

	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("검증에 걸렸는데 파일이 바뀌었다")
	}
}

func TestUnknownTableAndPathEscape(t *testing.T) {
	ts, s, _ := newTestServer(t)

	if code, _ := call(t, ts, s, http.MethodGet, "/api/table/없는표", ""); code != http.StatusNotFound {
		t.Fatalf("스키마 밖 표 이름이 %d 다 (404 여야 한다)", code)
	}
	// 폴더 밖으로 나가려는 이름들. 스키마에 없으니 404 로 먼저 막힌다.
	// ".." 처럼 라우터가 경로를 정리해 버리는 것도 있어 답의 꼴은 안 보고 코드만 본다.
	for _, name := range []string{"..%2f..%2fschema", "..", "C:%5CWindows%5Cwin", "sub%2fitem"} {
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/table/"+name, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set(tokenHeader, s.Token())
		res, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusBadRequest {
			t.Fatalf("%q 가 %d 다 (막혀야 한다)", name, res.StatusCode)
		}
	}
	// 이름 검사 자체도 따로 본다 — 404 가 이름 검사를 가리지 않게.
	if _, err := s.tablePath("../schema"); err == nil {
		t.Fatal("../schema 가 경로 감옥을 통과했다")
	}
	if _, err := s.tablePath("item"); err != nil {
		t.Fatalf("멀쩡한 이름이 막혔다: %v", err)
	}
}

func TestTokenAndValidate(t *testing.T) {
	ts, s, _ := newTestServer(t)

	res, err := ts.Client().Get(ts.URL + "/api/tables") // 토큰 없이
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("토큰 없이 %d 다 (401 이어야 한다)", res.StatusCode)
	}

	code, body := call(t, ts, s, http.MethodPost, "/api/validate", "{}")
	if code != http.StatusOK || body["ok"] != true {
		t.Fatalf("멀쩡한 자료인데 검증이 %d %v 다", code, body)
	}
}

func TestStaticUI(t *testing.T) {
	ts, _, _ := newTestServer(t)
	res, err := ts.Client().Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	page, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || !bytes.Contains(page, []byte("DataTool")) {
		t.Fatalf("화면이 안 나온다: %d %.60s", res.StatusCode, page)
	}
}

// PUT 본문이 UTF-8 이 아니면 400 이다 (리뷰 J).
// 깨진 바이트를 그대로 데이터 파일에 옮겨 적으면 git diff 와 편집기가 같이 망가진다.
func TestPutRejectsInvalidUTF8(t *testing.T) {
	ts, s, dir := newTestServer(t)
	before, err := os.ReadFile(filepath.Join(dir, "item.json"))
	if err != nil {
		t.Fatal(err)
	}

	body := "[{\"id\":\"sword_iron\",\"name\":\"\xff\xfe\",\"atk\":1}]"
	code, out := call(t, ts, s, http.MethodPut, "/api/table/item", body)
	if code != http.StatusBadRequest {
		t.Fatalf("깨진 바이트인데 %d 다: %v", code, out)
	}
	after, err := os.ReadFile(filepath.Join(dir, "item.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("400 인데 파일을 고쳤다")
	}
}

// PUT 은 fmt 와 같은 숫자 철자로 쓴다 — 브라우저가 보낸 12.5 는 12.5 로, 1.25e1 도 12.5 로.
// 파일에 300.0 으로 적혀 있던 값은 UI 가 300 으로 보내므로 300 이 된다 (설계 2026-10-03 8장).
func TestPutWritesJSNumberSpelling(t *testing.T) {
	ts, s, dir := newTestServer(t)
	path := filepath.Join(dir, "item.json")
	rows := `[{"id":"sword_iron","name":"철검","atk":12,"price":300.0,"tags":["weapon","melee"]},
	{"id":"sword_steel","name":"강철검","atk":24,"price":12.5,"grade":"rare","tags":["weapon","melee"]},
	{"id":"bow_short","name":"단궁","atk":9,"price":1.25e1,"tags":["weapon","ranged"]},
	{"id":"potion_hp","name":"체력 물약","price":0.0,"usable":true,"tags":["consume"]},
	{"id":"potion_mp","name":"마나 물약","price":60,"usable":true,"tags":["consume"]},
	{"id":"gem_fire","name":"불의 보석","price":1200,"grade":"epic"}]`
	code, body := call(t, ts, s, http.MethodPut, "/api/table/item", rows)
	if code != http.StatusOK {
		t.Fatalf("저장이 %d 다: %v", code, body)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"price":300,`, `"atk":24,"price":12.5,`, `"atk":9,"price":12.5,`, `{"id":"potion_hp","name":"체력 물약","usable":true`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("%s 가 없다:\n%s", want, got)
		}
	}
	if strings.Contains(string(got), "300.0") || strings.Contains(string(got), "e1") {
		t.Fatalf("숫자 철자가 안 바뀌었다:\n%s", got)
	}
}
