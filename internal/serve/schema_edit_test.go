package serve

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// snapshot 은 폴더의 파일 바이트 전부다. 「한 글자도 안 썼다」를 바이트로 확인한다.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(data)
	}
	return out
}

func sameSnapshot(t *testing.T, before, after map[string]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("파일 수가 바뀌었다: %d → %d", len(before), len(after))
	}
	for name, data := range before {
		if after[name] != data {
			t.Fatalf("%s 가 바뀌었다", name)
		}
	}
}

// schemaSource 는 GET /api/schema 의 rev 와 파일 꼴(source)을 글자 그대로 준다.
// source 는 빈칸 없는 한 줄 JSON 으로 온다 (writeJSON 이 줄인다). 시험의 글자 바꾸기도 그 꼴에 맞춘다.
func schemaSource(t *testing.T, ts *httptest.Server, s *Server) (string, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/schema", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(tokenHeader, s.Token())
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct {
		Rev    string          `json:"rev"`
		Source json.RawMessage `json:"source"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil || body.Rev == "" || len(body.Source) == 0 {
		t.Fatalf("rev·source 가 없다: %v", err)
	}
	return body.Rev, string(body.Source)
}

func editBody(source string, ops string) string {
	return `{"schema":` + source + `,"ops":` + ops + `}`
}

func putSchema(t *testing.T, ts *httptest.Server, s *Server, rev, body string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/schema", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(tokenHeader, s.Token())
	req.Header.Set("Content-Type", "application/json")
	if rev != "" {
		req.Header.Set("If-Match", rev)
	}
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("답이 JSON 이 아니다: %s", raw)
	}
	return res.StatusCode, out
}

// 열 이름 바꾸기 + enum 값 이름 바꾸기 (op 둘).
const renameOps = `[{"op":"renameColumn","table":"item","from":"atk","to":"attack"},
 {"op":"renameEnumValue","enum":"Grade","from":"rare","to":"uncommon"}]`

func renamed(source string) string {
	s := strings.Replace(source, `"name":"atk"`, `"name":"attack"`, 1)
	return strings.Replace(s, `"rare"`, `"uncommon"`, 1)
}

func TestGetSchemaHasRevAndNumbers(t *testing.T) {
	ts, s, _ := newTestServer(t)
	_, body := call(t, ts, s, http.MethodGet, "/api/schema", "")
	nums, _ := body["enumNumbers"].(map[string]any)
	grade, _ := nums["Grade"].([]any)
	if len(grade) != 3 || grade[2] != float64(2) {
		t.Fatalf("enumNumbers 가 차례 번호가 아니다: %v", body["enumNumbers"])
	}
	if _, ok := body["enums"].(map[string]any)["Grade"].([]any); !ok {
		t.Fatal("옛 enums 꼴이 바뀌었다 (UI 가 쓴다)")
	}
}

func TestSchemaPlanWritesNothing(t *testing.T) {
	ts, s, dir := newTestServer(t)
	before := snapshot(t, dir)
	_, source := schemaSource(t, ts, s)

	code, body := call(t, ts, s, http.MethodPost, "/api/schema/plan", editBody(renamed(source), renameOps))
	if code != http.StatusOK || body["ok"] != true {
		t.Fatalf("plan 이 %d 다: %v", code, body)
	}
	files, _ := body["files"].([]any)
	if len(files) != 2 {
		t.Fatalf("item.json 과 schema.json 이 바뀌어야 한다: %v", files)
	}
	first, _ := files[0].(map[string]any)
	if first["file"] != "item.json" || first["rowsChanged"].(float64) < 1 {
		t.Fatalf("item.json 영향이 틀렸다: %v", first)
	}
	sameSnapshot(t, before, snapshot(t, dir))
}

func TestSchemaPutRenames(t *testing.T) {
	ts, s, dir := newTestServer(t)
	rev, source := schemaSource(t, ts, s)

	code, body := putSchema(t, ts, s, rev, editBody(renamed(source), renameOps))
	if code != http.StatusOK {
		t.Fatalf("PUT 이 %d 다: %v", code, body)
	}
	item, _ := os.ReadFile(filepath.Join(dir, "item.json"))
	if !strings.Contains(string(item), `"attack":24`) || !strings.Contains(string(item), `"grade":"uncommon"`) {
		t.Fatalf("행을 안 따라 고쳤다:\n%s", item)
	}
	if strings.Contains(string(item), `"atk"`) {
		t.Fatalf("옛 열 이름이 남았다:\n%s", item)
	}
	newRev, _ := schemaSource(t, ts, s)
	if newRev == rev || body["rev"] != newRev {
		t.Fatalf("새 rev 가 틀렸다: %v / %s", body["rev"], newRev)
	}
	// 쓴 뒤에도 validate 를 통과한다.
	if code, v := call(t, ts, s, http.MethodPost, "/api/validate", ""); code != http.StatusOK || v["ok"] != true {
		t.Fatalf("쓴 뒤 validate 가 안 통과한다: %v", v)
	}
}

func TestSchemaPutNeedsMatchingRev(t *testing.T) {
	ts, s, dir := newTestServer(t)
	before := snapshot(t, dir)
	_, source := schemaSource(t, ts, s)
	body := editBody(renamed(source), renameOps)

	if code, _ := putSchema(t, ts, s, "", body); code != http.StatusPreconditionRequired {
		t.Fatalf("If-Match 없음이 %d 다", code)
	}
	code, out := putSchema(t, ts, s, "deadbeef", body)
	if code != http.StatusConflict || out["rev"] == "" {
		t.Fatalf("다른 rev 가 %d 다: %v", code, out)
	}
	sameSnapshot(t, before, snapshot(t, dir))
}

// 검증에 걸리면 표도 스키마도 한 글자도 안 쓴다.
func TestSchemaPutRefusedWritesNothing(t *testing.T) {
	ts, s, dir := newTestServer(t)
	before := snapshot(t, dir)
	rev, source := schemaSource(t, ts, s)

	cases := map[string]string{
		// rare 를 쓰는 행이 있는데 대체 값 없이 지운다
		"대체 없는 값 지우기": editBody(strings.Replace(source, `"common","rare","epic"`, `"common","epic"`, 1),
			`[{"op":"dropEnumValue","enum":"Grade","value":"rare"}]`),
		// op 없이 열이 사라졌다
		"op 없는 열 사라짐": editBody(strings.Replace(source, `"name":"atk"`, `"name":"attack"`, 1), `[]`),
		// 필수 열(기본값 없음)을 더하면 옛 행이 V2 에 걸린다
		"필수 열 더하기": editBody(strings.Replace(source, `{"name":"id","type":"string"},`,
			`{"name":"id","type":"string"},{"name":"weight","type":"int"},`, 1), `[]`),
		// 스키마 자체가 틀렸다
		"틀린 스키마": editBody(strings.Replace(source, `"type":"int"`, `"type":"nope"`, 1), `[]`),
	}
	for name, body := range cases {
		code, out := putSchema(t, ts, s, rev, body)
		if code != http.StatusBadRequest || out["ok"] != false {
			t.Fatalf("%s: 400 이어야 하는데 %d 다: %v", name, code, out)
		}
		if problems, _ := out["problems"].([]any); len(problems) == 0 {
			t.Fatalf("%s: problems 가 비었다: %v", name, out)
		}
		sameSnapshot(t, before, snapshot(t, dir))
	}
}

func TestSchemaBodyChecks(t *testing.T) {
	ts, s, _ := newTestServer(t)
	_, source := schemaSource(t, ts, s)
	if code, _ := call(t, ts, s, http.MethodPost, "/api/schema/plan", `{"schema":`+source+`,"opz":[]}`); code != http.StatusBadRequest {
		t.Fatalf("모르는 칸이 %d 다", code)
	}
	if code, _ := call(t, ts, s, http.MethodPost, "/api/schema/plan", `{"ops":[]}`); code != http.StatusBadRequest {
		t.Fatalf("schema 없음이 %d 다", code)
	}
	// 토큰 없이는 문지기가 막는다.
	res, err := ts.Client().Post(ts.URL+"/api/schema/plan", "application/json", strings.NewReader(editBody(source, "[]")))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("토큰 없는 plan 이 %d 다", res.StatusCode)
	}
}

func TestGenAPI(t *testing.T) {
	ts, s, _ := newTestServer(t)
	if code, out := call(t, ts, s, http.MethodPost, "/api/gen", ""); code != http.StatusBadRequest {
		t.Fatalf("gen 자리 없음이 %d 다: %v", code, out)
	}
	genDir := filepath.Join(t.TempDir(), "Generated")
	if err := s.SetGenDir(genDir); err != nil {
		t.Fatal(err)
	}
	code, out := call(t, ts, s, http.MethodPost, "/api/gen", "")
	if code != http.StatusOK {
		t.Fatalf("gen 이 %d 다: %v", code, out)
	}
	if _, err := os.Stat(filepath.Join(genDir, "Grade.cs")); err != nil {
		t.Fatalf("Grade.cs 가 없다: %v", err)
	}
}
