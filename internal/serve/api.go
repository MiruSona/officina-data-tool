package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/mirusona/officina-data-tool/internal/assetindex"
	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/table"
	"github.com/mirusona/officina-data-tool/internal/textfile"
	"github.com/mirusona/officina-data-tool/internal/validate"
)

// handleTables 는 표 이름과 행 수만 준다. 왼쪽 목록이 쓰는 것이다.
func (s *Server) handleTables(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	sch, tables, err := s.loadAll()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	list := []map[string]any{}
	for _, name := range sch.TableNames() {
		list = append(list, map[string]any{"name": name, "rows": len(tables[name].Rows)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tables": list})
}

// handleTable 은 표 하나를 읽거나(GET) 통째로 저장한다(PUT).
func (s *Server) handleTable(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/table/")
	switch r.Method {
	case http.MethodGet:
		s.getTable(w, name)
	case http.MethodPut:
		s.putTable(w, r, name)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"ok": false, "error": "GET 이나 PUT 만 받는다",
		})
	}
}

// getTable 은 행 배열을 준다. **기본값이 빠진 열은 서버가 채워서** 보낸다 —
// 저장할 때 table.Format 이 같은 기본값을 도로 빼므로 왕복해도 파일이 그대로다.
// UI 가 스키마를 보고 따로 채우면 「채우는 규칙」이 둘로 갈린다.
func (s *Server) getTable(w http.ResponseWriter, name string) {
	code, body := s.readTable(name)
	writeJSON(w, code, body) // 응답 쓰기는 잠금 밖에서 — 느린 클라이언트가 잠금을 붙들지 않게
}

// readTable 은 getTable 의 몸통이다. 잠금 안에서 읽기·파싱·본문 만들기까지만 한다.
func (s *Server) readTable(name string) (int, map[string]any) {
	// 스키마와 표 바이트를 같은 잠금 안에서 읽는다 — 스키마 PUT 과 겨뤄 옛 열 + 새 표가 짝이 되지 않게.
	s.mu.Lock()
	defer s.mu.Unlock()
	sch, schemaRev, err := s.readSchema()
	if err != nil {
		return http.StatusBadRequest, errorBody(err)
	}
	st := sch.Table(name)
	if st == nil {
		return http.StatusNotFound, map[string]any{
			"ok": false, "error": fmt.Sprintf("스키마에 없는 표다: %q", name),
		}
	}
	path, err := s.tablePath(name)
	if err != nil {
		return http.StatusBadRequest, errorBody(err)
	}

	// rev 와 행이 같은 판이도록 한 번 읽은 바이트에서 둘 다 낸다.
	raw, rev, err := fileRev(path)
	if err != nil {
		return http.StatusBadRequest, errorBody(err)
	}
	t, err := table.Parse(textfile.TrimBOM(raw), path)
	if err != nil {
		return http.StatusBadRequest, errorBody(err)
	}
	return http.StatusOK, map[string]any{
		"ok":        true,
		"name":      name,
		"columns":   columnsJSON(st),
		"rows":      rowsJSON(t, st),
		"rev":       rev,
		"schemaRev": schemaRev,
	}
}

// putTable 은 행 배열을 통째로 받아 **검증을 통과했을 때만** 파일을 다시 쓴다.
//
// 걸리면 한 글자도 안 쓰고 문제 목록을 400 으로 돌려준다 — 우회하는 문(--force)을 안 만든다.
// If-Match 가 디스크 rev 와 다르면 409 로 안 쓴다. 차례는 Docs/Design/2026-10-12-표저장rev설계.md 2-4.
// X-Schema-Rev 를 실었으면 그것도 디스크 schema.json rev 와 견준다 (표 409 다음).
func (s *Server) putTable(w http.ResponseWriter, r *http.Request, name string) {
	if ct := r.Header.Get("Content-Type"); !isJSONType(ct) {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{
			"ok": false, "error": "Content-Type 이 application/json 이어야 한다",
		})
		return
	}
	want := ifMatchRev(r)
	if want == "" {
		writeJSON(w, http.StatusPreconditionRequired, map[string]any{
			"ok": false, "error": "If-Match 머리에 GET /api/table/" + name + " 의 rev 를 실어 보낸다",
		})
		return
	}
	rows, err := readRows(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	wantSchema := revHeader(r, schemaRevHeader)
	code, body := s.saveTable(name, want, wantSchema, rows)
	writeJSON(w, code, body) // getTable 과 같이 응답 쓰기는 잠금 밖에서
}

// saveTable 은 putTable 의 잠금 안 몸통이다. wantSchema 가 "" 면 스키마 rev 는 안 본다.
func (s *Server) saveTable(name, want, wantSchema string, rows []json.RawMessage) (int, map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sch, schemaRev, err := s.readSchema()
	if err != nil {
		return http.StatusBadRequest, errorBody(err)
	}
	// 이 표는 아래 fileRev 로 한 번만 읽는다. 받은 행으로 갈아 끼우니 LoadAll 이 읽을 까닭이 없다.
	tables, err := table.LoadAllExcept(s.root, sch, name)
	if err != nil {
		return http.StatusBadRequest, errorBody(err)
	}
	st := sch.Table(name)
	if st == nil {
		return http.StatusNotFound, map[string]any{
			"ok": false, "error": fmt.Sprintf("스키마에 없는 표다: %q", name),
		}
	}
	path, err := s.tablePath(name)
	if err != nil {
		return http.StatusBadRequest, errorBody(err)
	}
	raw, rev, err := fileRev(path)
	if err != nil {
		return http.StatusBadRequest, errorBody(err)
	}
	// rev 견줌이 먼저다 — 디스크 표를 파싱하지 않는다. 디스크 파일이 깨졌으면 GET 이 400 이라
	// 클라이언트는 그 rev 를 가질 수 없으니 409 가 된다 (받은 행만 아래 parseRows 로 본다).
	if want != rev {
		return http.StatusConflict, map[string]any{
			"ok": false, "conflict": "table", "rev": rev, "schemaRev": schemaRev,
			"error": "그사이 " + name + ".json 이 바뀌었다 — 다시 읽고 고친다",
		}
	}
	// 열 이름·형이 그대로인 스키마 변경(기본값·범위)은 검증에 안 걸린다 — 보던 판인지 rev 로 따로 본다.
	if wantSchema != "" && wantSchema != schemaRev {
		return http.StatusConflict, map[string]any{
			"ok": false, "conflict": "schema", "rev": rev, "schemaRev": schemaRev,
			"error": "그사이 schema.json 이 바뀌었다 — 다시 읽고 고친다",
		}
	}

	t, err := parseRows(rows, path)
	if err != nil {
		return http.StatusBadRequest, errorBody(err)
	}
	tables[name] = t // 참조(ref)는 다른 표까지 봐야 하므로 이 표만 갈아 끼운다
	problems, warnings := s.validateForSave(sch, tables)
	if len(problems) > 0 {
		return http.StatusBadRequest, map[string]any{
			"ok": false, "error": "검증에 걸려 안 썼다", "problems": problems, "warnings": warnings,
		}
	}

	out, err := t.Format(st)
	if err != nil {
		return http.StatusBadRequest, errorBody(err)
	}
	// 디스크와 같은 내용이면 안 쓴다 (BOM 은 걷고 CRLF 는 안 걷는 견줌 — textfile.SameContent).
	// 안 썼으면 rev 는 디스크(BOM 포함) 그대로, 썼으면 다시 읽지 않고 쓴 바이트로 낸다 — 그사이 밖의 변경이 섞이지 않게.
	if !textfile.SameContent(out, raw) {
		if err := table.WriteFile(path, out); err != nil {
			return http.StatusInternalServerError, errorBody(err)
		}
		rev = revOf(out)
	}
	return http.StatusOK, map[string]any{
		"ok": true, "name": name, "rows": len(t.Rows),
		"path":      filepath.ToSlash(filepath.Base(path)),
		"warnings":  warnings,
		"rev":       rev,
		"schemaRev": schemaRev,
	}
}

// validateForSave 는 저장 때의 검증이다. V10 오류(asset·asset_kind)는 처음부터 경고로 모은다 —
// 색인이 낡았을 때 새 주소를 먼저 적을 수 있어야 한다. 막는 것은 CLI 뿐이다 (연동 설계 3-3).
func (s *Server) validateForSave(sch *schema.File, tables map[string]*table.Table) ([]*validate.Problem, []*validate.Problem) {
	assets, warnings, err := s.assetsFor(sch)
	if err != nil {
		warnings = append(warnings, &validate.Problem{File: filepath.ToSlash(s.indexPath),
			Rule: validate.RuleAssetIndex, Message: "색인을 못 읽어 asset 검사를 건너뜀 — " + err.Error()})
	}
	problems, rowWarnings := validate.RunWith(sch, tables, validate.Options{Assets: assets, AssetsAsWarnings: true})
	return problems, append(warnings, rowWarnings...)
}

// assetsFor 는 asset 열이 있을 때만 색인을 연다. 깨진 색인은 오류로 돌려준다.
func (s *Server) assetsFor(sch *schema.File) (*assetindex.Index, []*validate.Problem, error) {
	if !sch.UsesAsset() {
		return nil, []*validate.Problem{}, nil
	}
	ix, status, err := s.openIndex()
	if err != nil {
		return nil, []*validate.Problem{}, err
	}
	return ix, validate.IndexWarnings(status), nil
}

// handleValidate 는 CLI 의 validate 와 같은 일을 한다. 한 글자도 안 쓴다.
func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}
	s.mu.Lock()
	sch, tables, err := s.loadAll()
	s.mu.Unlock()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	problems, warnings, err := s.validateStrict(sch, tables)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	rows := 0
	for _, t := range tables {
		rows += len(t.Rows)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       len(problems) == 0,
		"problems": problems,
		"warnings": warnings,
		"counts": map[string]any{
			"tables": len(tables), "rows": rows, "errors": len(problems),
		},
	})
}

// validateStrict 는 CLI validate 와 같은 판정이다 — V10 오류는 오류다. `validate` · `gen` 이 쓴다.
// 깨진 색인은 CLI 의 종료 4 처럼 error 로 돌려준다.
func (s *Server) validateStrict(sch *schema.File, tables map[string]*table.Table) ([]*validate.Problem, []*validate.Problem, error) {
	assets, warnings, err := s.assetsFor(sch)
	if err != nil {
		return nil, nil, err
	}
	problems, rowWarnings := validate.RunWithIndex(sch, tables, assets)
	return problems, append(warnings, rowWarnings...), nil
}

func (s *Server) loadSchema() (*schema.File, error) {
	sch, _, err := s.readSchema()
	return sch, err
}

// readSchema 는 schema.json 을 한 번 읽어 그 바이트로 파싱과 rev 를 같이 낸다 — 열과 rev 가 다른 판이 되지 않게.
// 파싱은 schema.Load 와 같다 (BOM 걷고 Parse). rev 는 BOM 까지 든 원본 바이트로 낸다.
func (s *Server) readSchema() (*schema.File, string, error) {
	raw, rev, err := s.readSchemaFile()
	if err != nil {
		return nil, "", err
	}
	sch, err := schema.Parse(textfile.TrimBOM(raw), filepath.Join(s.root, table.SchemaFileName))
	if err != nil {
		return nil, "", err
	}
	return sch, rev, nil
}

func (s *Server) loadAll() (*schema.File, map[string]*table.Table, error) {
	sch, err := s.loadSchema()
	if err != nil {
		return nil, nil, err
	}
	tables, err := table.LoadAll(s.root, sch)
	if err != nil {
		return nil, nil, err
	}
	return sch, tables, nil
}

// tablePath 는 표 이름을 데이터 폴더 안의 파일 경로로 바꾼다.
//
// 이름이 곧 파일 이름이라(설계 3장) 이름 꼴을 먼저 막고, 그다음 만든 경로가
// 정말 뿌리 안인지 접두로 다시 본다 — 이름 검사만 믿지 않는다.
func (s *Server) tablePath(name string) (string, error) {
	if !validTableName(name) {
		return "", fmt.Errorf("표 이름이 %q 꼴이 아니다: %q", "^[a-z][a-z0-9_]*$", name)
	}
	path := filepath.Join(s.root, name+".json")
	if !inside(s.root, path) {
		return "", fmt.Errorf("데이터 폴더 밖이다: %q", name)
	}
	return path, nil
}

func validTableName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
		case i > 0 && (c >= '0' && c <= '9' || c == '_'):
		default:
			return false
		}
	}
	return true
}

// inside 는 경로가 뿌리 안인지 본다.
//
// 대소문자는 **접지 않는다** — filepath.Rel 은 글자 그대로 본다.
// 표 이름 쪽에서 소문자만 통과시키므로(validTableName) 여기서 더 접을 것이 없다.
func inside(root, path string) bool {
	rel, err := filepath.Rel(root, filepath.Clean(path))
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !filepath.IsAbs(rel)
}

func isJSONType(contentType string) bool {
	base, _, _ := strings.Cut(contentType, ";")
	return strings.TrimSpace(strings.ToLower(base)) == "application/json"
}

func allowMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
		"ok": false, "error": method + " 만 받는다",
	})
	return false
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, errorBody(err))
}

func errorBody(err error) map[string]any {
	return map[string]any{"ok": false, "error": err.Error()}
}

func writeJSON(w http.ResponseWriter, code int, body map[string]any) {
	out, err := json.Marshal(body)
	if err != nil {
		http.Error(w, "결과를 JSON 으로 못 적었다", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	w.Write(out) //nolint:errcheck // 이미 보내기 시작한 뒤라 알릴 곳이 없다
}
