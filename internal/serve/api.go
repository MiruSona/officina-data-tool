package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/table"
	"github.com/mirusona/officina-data-tool/internal/validate"
)

// handleSchema 는 스키마 전체를 준다. UI 는 이것으로 열과 편집기를 만든다.
func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	sch, err := s.loadSchema()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, schemaJSON(sch))
}

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
	sch, err := s.loadSchema()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	st := sch.Table(name)
	if st == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"ok": false, "error": fmt.Sprintf("스키마에 없는 표다: %q", name),
		})
		return
	}
	path, err := s.tablePath(name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	s.mu.Lock()
	t, err := table.Load(path)
	s.mu.Unlock()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"name":    name,
		"columns": columnsJSON(st),
		"rows":    rowsJSON(t, st),
	})
}

// putTable 은 행 배열을 통째로 받아 **검증을 통과했을 때만** 파일을 다시 쓴다.
//
// 걸리면 한 글자도 안 쓰고 문제 목록을 400 으로 돌려준다 — 우회하는 문(--force)을 안 만든다.
func (s *Server) putTable(w http.ResponseWriter, r *http.Request, name string) {
	if ct := r.Header.Get("Content-Type"); !isJSONType(ct) {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{
			"ok": false, "error": "Content-Type 이 application/json 이어야 한다",
		})
		return
	}
	rows, err := readRows(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sch, tables, err := s.loadAll()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	st := sch.Table(name)
	if st == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"ok": false, "error": fmt.Sprintf("스키마에 없는 표다: %q", name),
		})
		return
	}
	path, err := s.tablePath(name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	t, err := parseRows(rows, path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	tables[name] = t // 참조(ref)는 다른 표까지 봐야 하므로 이 표만 갈아 끼운다
	if problems := validate.Run(sch, tables); len(problems) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok": false, "error": "검증에 걸려 안 썼다", "problems": problems,
		})
		return
	}

	out, err := t.Format(st)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := table.WriteFile(path, out); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "name": name, "rows": len(t.Rows),
		"path": filepath.ToSlash(filepath.Base(path)),
	})
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
	problems := validate.Run(sch, tables)
	rows := 0
	for _, t := range tables {
		rows += len(t.Rows)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       len(problems) == 0,
		"problems": problems,
		"counts": map[string]any{
			"tables": len(tables), "rows": rows, "errors": len(problems),
		},
	})
}

func (s *Server) loadSchema() (*schema.File, error) {
	return schema.Load(filepath.Join(s.root, table.SchemaFileName))
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
	writeJSON(w, code, map[string]any{"ok": false, "error": err.Error()})
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
