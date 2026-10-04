package serve

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mirusona/officina-data-tool/internal/gen"
)

// SetGenDir 는 gen 이 쓸 폴더를 한 번 정규화해 쥔다 (스키마·enum 편집 설계 6장).
//
// 폴더가 이미 있으면 링크까지 푼 실제 경로로 쥔다. 없으면 절대경로만 — gen.Write 가 만든다.
// 데이터 폴더 밖(보통 ../Assets/…)이 정상이라 뿌리 안인지는 안 본다. 자리는 설정에서만 온다.
func (s *Server) SetGenDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		return fmt.Errorf("gen 자리가 폴더가 아니다: %s", filepath.ToSlash(abs))
	}
	s.genDir = abs
	return nil
}

// handleGen 은 CLI `gen` 과 같은 일을 한다 — 먼저 validate(V10 은 오류), 그다음 C# 을 쓴다.
// 읽는 것은 디스크의 스키마·데이터다. 저장 안 한 편집은 안 본다.
func (s *Server) handleGen(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}
	if s.genDir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok": false, "error": ".datatool.json 에 gen 칸이 없다 — C# 을 쓸 폴더를 거기 적고 serve 를 다시 띄운다",
		})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	sch, tables, err := s.loadAll()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	problems, warnings, err := s.validateStrict(sch, tables)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(problems) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok": false, "error": "검증에 걸려 안 만들었다", "problems": problems, "warnings": warnings,
		})
		return
	}

	written, stale, err := gen.Build(sch, s.genDir)
	if err != nil {
		code := http.StatusInternalServerError
		var bad *gen.SchemaError
		if errors.As(err, &bad) {
			code = http.StatusBadRequest
		}
		writeError(w, code, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "dir": filepath.ToSlash(s.genDir), "written": written, "stale": stale, "warnings": warnings,
	})
}
