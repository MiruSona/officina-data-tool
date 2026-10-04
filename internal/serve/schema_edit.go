package serve

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mirusona/officina-data-tool/internal/gen"
	"github.com/mirusona/officina-data-tool/internal/migrate"
	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/table"
	"github.com/mirusona/officina-data-tool/internal/validate"
)

// 스키마 편집 API 의 규칙 이름이다. validate.Problem 의 rule 칸에 나간다 (설계 3-2).
const (
	ruleSchema = "schema"
	ruleGen    = "gen"
)

// schemaEdit 는 plan · PUT 의 본문이다 (스키마·enum 편집 설계 3-1).
type schemaEdit struct {
	Schema json.RawMessage `json:"schema"`
	Ops    []migrate.Op    `json:"ops"`
}

// fileChange 는 plan 미리보기의 한 줄이다 — 어느 파일이 얼마나 바뀌나.
type fileChange struct {
	File        string `json:"file"`
	RowsChanged int    `json:"rowsChanged"`
	ValuesLost  int    `json:"valuesLost"`
}

// editPlan 은 본문을 메모리에서 끝까지 돌려 본 결과다. 디스크는 안 건드린다.
type editPlan struct {
	files    []fileChange
	pending  []table.Pending
	problems []*validate.Problem
	warnings []*validate.Problem
	notes    []string
}

func (e *editPlan) body(rev string) map[string]any {
	return map[string]any{
		"ok": len(e.problems) == 0, "rev": rev,
		"files": e.files, "problems": e.problems, "warnings": e.warnings, "notes": e.notes,
	}
}

// handleSchema 는 스키마를 읽거나(GET) 바꾼다(PUT).
func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getSchema(w)
	case http.MethodPut:
		s.putSchema(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"ok": false, "error": "GET 이나 PUT 만 받는다",
		})
	}
}

// getSchema 는 UI 가 쓰는 꼴 + rev + 파일 꼴 그대로(source)를 준다.
// 편집 화면은 source 를 고쳐 그대로 돌려보낸다 — UI 꼴에서 파일 꼴로 되옮기다 칸을 잃지 않게.
func (s *Server) getSchema(w http.ResponseWriter) {
	s.mu.Lock()
	raw, rev, err := s.readSchemaFile()
	s.mu.Unlock()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sch, err := schema.Parse(raw, table.SchemaFileName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	body := schemaJSON(sch)
	body["rev"] = rev
	body["source"] = json.RawMessage(sch.Format())
	writeJSON(w, http.StatusOK, body)
}

// handleSchemaPlan 은 PUT 과 똑같이 돌려 보되 한 글자도 안 쓴다.
func (s *Server) handleSchemaPlan(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}
	edit, err := readSchemaEdit(r)
	if err != nil {
		writeJSONError(w, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, rev, err := s.readSchemaFile()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	plan, err := s.planEdit(edit, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, plan.body(rev))
}

// putSchema 는 검증을 다 통과했을 때만 표들과 schema.json 을 쓴다 (설계 4장).
// 걸리면 한 글자도 안 쓴다. 우회하는 문(--force)은 없다.
func (s *Server) putSchema(w http.ResponseWriter, r *http.Request) {
	want := strings.TrimSpace(r.Header.Get("If-Match"))
	if want == "" {
		writeJSON(w, http.StatusPreconditionRequired, map[string]any{
			"ok": false, "error": "If-Match 머리에 GET /api/schema 의 rev 를 실어 보낸다",
		})
		return
	}
	edit, err := readSchemaEdit(r)
	if err != nil {
		writeJSONError(w, err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	_, rev, err := s.readSchemaFile()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.Trim(want, `"`) != rev {
		writeJSON(w, http.StatusConflict, map[string]any{
			"ok": false, "rev": rev, "error": "그사이 schema.json 이 바뀌었다 — 다시 읽고 고친다",
		})
		return
	}
	plan, err := s.planEdit(edit, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(plan.problems) > 0 {
		body := plan.body(rev)
		body["error"] = "검증에 걸려 안 썼다"
		writeJSON(w, http.StatusBadRequest, body)
		return
	}

	written, err := table.WriteAll(plan.pending)
	if err != nil {
		body := map[string]any{"ok": false, "error": "쓰다가 멈췄다: " + err.Error(), "written": s.names(written)}
		var partial *table.WriteAllError
		if errors.As(err, &partial) {
			body["notWritten"] = s.names(partial.NotWritten)
		}
		writeJSON(w, http.StatusInternalServerError, body)
		return
	}
	_, newRev, err := s.readSchemaFile()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "rev": newRev, "written": s.names(written),
		"warnings": plan.warnings, "notes": plan.notes,
	})
}

// planEdit 는 설계 4장의 2~7 을 메모리에서 돈다. 부르는 쪽이 s.mu 를 잡고 온다.
// 돌려주는 error 는 디스크를 못 읽은 것뿐이다. 본문·데이터의 잘못은 plan.problems 에 담는다.
func (s *Server) planEdit(edit *schemaEdit, withGit bool) (*editPlan, error) {
	plan := &editPlan{files: []fileChange{}, problems: []*validate.Problem{}, warnings: []*validate.Problem{}, notes: []string{}}
	old, tables, err := s.loadAll()
	if err != nil {
		return nil, err
	}

	mig, problems := migrate.New(old, edit.Ops)
	if len(problems) > 0 {
		plan.problems = problems
		return plan, nil
	}
	raw, problems := mig.PatchDefaults(edit.Schema)
	if len(problems) > 0 {
		plan.problems = problems
		return plan, nil
	}
	next, err := schema.Parse(raw, table.SchemaFileName)
	if err != nil {
		plan.problems = schemaProblems(err)
		return plan, nil
	}
	moved, problems := mig.Apply(next, tables)
	if len(problems) > 0 {
		plan.problems = problems
		return plan, nil
	}

	plan.problems, plan.warnings = s.validateForSave(next, moved.Tables)
	if _, err := gen.Generate(next); err != nil {
		plan.problems = append(plan.problems, &validate.Problem{File: table.SchemaFileName, Rule: ruleGen, Message: err.Error()})
	}
	validate.Relativize(plan.problems, s.root)
	validate.Relativize(plan.warnings, s.root)
	plan.notes = moved.Notes
	if len(plan.problems) > 0 {
		return plan, nil
	}

	if err := s.collectPending(plan, next, moved); err != nil {
		return nil, err
	}
	if withGit && len(plan.pending) > 0 {
		plan.notes = append(plan.notes, gitNotes(s.root, plan.pending)...)
	}
	return plan, nil
}

// collectPending 은 달라지는 파일만 모은다. 차례는 표(스키마 차례) 먼저, schema.json 마지막이다.
func (s *Server) collectPending(plan *editPlan, next *schema.File, moved *migrate.Result) error {
	for _, name := range next.TableNames() {
		t := moved.Tables[name]
		out, err := t.Format(next.Table(name))
		if err != nil {
			return err
		}
		if bytes.Equal(out, t.Raw) {
			continue
		}
		// 표 이름은 새 스키마에서 왔다. 쓰기 직전에 감옥(이름 꼴 + 뿌리 안)을 다시 지난다.
		path, err := s.writablePath(name + ".json")
		if err != nil {
			return err
		}
		change := moved.Changes[name]
		plan.files = append(plan.files, fileChange{File: name + ".json", RowsChanged: change.RowsChanged, ValuesLost: change.ValuesLost})
		plan.pending = append(plan.pending, table.Pending{Path: path, Data: out})
	}

	raw, _, err := s.readSchemaFile()
	if err != nil {
		return err
	}
	if out := next.Format(); !bytes.Equal(out, raw) {
		path, err := s.writablePath(table.SchemaFileName)
		if err != nil {
			return err
		}
		plan.files = append(plan.files, fileChange{File: table.SchemaFileName})
		plan.pending = append(plan.pending, table.Pending{Path: path, Data: out})
	}
	return nil
}

// writablePath 는 데이터 폴더 안의 파일 하나를 쓸 자리로 바꾼다.
// 표 파일은 tablePath(이름 꼴 + 뿌리 안)를 지나고, 이미 있으면 **보통 파일**이어야 한다 —
// 링크를 rename 으로 덮으면 링크 너머가 아니라 링크 자리가 바뀌지만, 그런 자리를 애초에 안 만진다.
func (s *Server) writablePath(file string) (string, error) {
	var path string
	if file == table.SchemaFileName {
		path = filepath.Join(s.root, table.SchemaFileName)
	} else {
		p, err := s.tablePath(strings.TrimSuffix(file, ".json"))
		if err != nil {
			return "", err
		}
		path = p
	}
	info, err := os.Lstat(path)
	if err == nil && !info.Mode().IsRegular() {
		return "", fmt.Errorf("보통 파일이 아니라 안 쓴다: %s", file)
	}
	return path, nil
}

// readSchemaFile 은 schema.json 의 디스크 바이트와 rev(sha256)를 준다.
// rev 는 BOM·줄끝까지 포함한 바이트로 낸다 — 손으로 고친 것도 「바뀜」으로 잡는다.
func (s *Server) readSchemaFile() ([]byte, string, error) {
	raw, err := os.ReadFile(filepath.Join(s.root, table.SchemaFileName))
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:]), nil
}

// names 는 절대경로를 데이터 폴더 기준 파일 이름으로 바꾼다.
func (s *Server) names(paths []string) []string {
	out := []string{}
	for _, p := range paths {
		out = append(out, filepath.ToSlash(filepath.Base(p)))
	}
	return out
}

// badBody 는 본문 꼴이 틀린 것이다 (400). 크기 초과(413)와 가른다.
type badBody struct{ msg string }

func (e *badBody) Error() string { return e.msg }

// readSchemaEdit 는 plan · PUT 본문을 읽는다. 모르는 칸은 바로 막는다 — 오타가 조용히 무시되면 안 된다.
func readSchemaEdit(r *http.Request) (*schemaEdit, error) {
	if !isJSONType(r.Header.Get("Content-Type")) {
		return nil, &badBody{"Content-Type 이 application/json 이어야 한다"}
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err // 상한(guard 의 MaxBytesReader)을 넘었거나 끊겼다
	}
	if !utf8.Valid(data) {
		return nil, &badBody{"본문이 UTF-8 이 아니다"}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	edit := &schemaEdit{}
	if err := dec.Decode(edit); err != nil {
		return nil, &badBody{"본문이 {schema, ops} 꼴이 아니다: " + err.Error()}
	}
	if len(bytes.TrimSpace(edit.Schema)) == 0 || bytes.Equal(bytes.TrimSpace(edit.Schema), []byte("null")) {
		return nil, &badBody{"schema 칸이 없다"}
	}
	return edit, nil
}

func writeJSONError(w http.ResponseWriter, err error) {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Errorf("본문이 상한(%d 바이트)을 넘었다", tooBig.Limit))
		return
	}
	var bad *badBody
	if errors.As(err, &bad) && strings.HasPrefix(bad.msg, "Content-Type") {
		writeError(w, http.StatusUnsupportedMediaType, err)
		return
	}
	writeError(w, http.StatusBadRequest, err)
}

// schemaProblems 는 schema.Parse 의 오류를 문제 목록으로 옮긴다. 자리(where)는 글 앞에 붙인다.
func schemaProblems(err error) []*validate.Problem {
	var se *schema.Errors
	if !errors.As(err, &se) {
		return []*validate.Problem{{File: table.SchemaFileName, Rule: ruleSchema, Message: err.Error()}}
	}
	out := []*validate.Problem{}
	for _, one := range se.List {
		out = append(out, &validate.Problem{File: table.SchemaFileName, Rule: ruleSchema, Message: one.Error()})
	}
	return out
}

// gitNotes 는 바뀔 파일을 git 이 지키고 있는지 본다 (결정 7 — 막지 않고 알리기만 한다).
//
// 셸을 안 거치고 인자를 서버가 만든 파일 이름으로만 채운다. 5초 안에 못 끝나면 확인을 접는다.
func gitNotes(root string, pending []table.Pending) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []string{"-C", root, "status", "--porcelain", "--"}
	for _, p := range pending {
		args = append(args, filepath.Base(p.Path))
	}
	out, err := exec.CommandContext(ctx, "git", args...).Output()
	if errors.Is(err, exec.ErrNotFound) {
		return []string{"git 을 못 찾아 되돌릴 길을 확인 못 했다 — 저장 전에 데이터 폴더를 따로 챙긴다"}
	}
	if err != nil {
		return []string{"데이터 폴더가 git 아래가 아니다 — 저장하면 되돌릴 길이 없다"}
	}
	dirty := []string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if len(line) > 3 {
			dirty = append(dirty, filepath.Base(strings.TrimSpace(line[3:])))
		}
	}
	if len(dirty) == 0 {
		return []string{}
	}
	return []string{"커밋 안 한 변경이 있는 파일: " + strings.Join(dirty, ", ") + " — 저장 전에 커밋해 두면 되돌리기 쉽다"}
}
