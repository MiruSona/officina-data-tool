package gen

import (
	"sort"

	"github.com/mirusona/officina-data-tool/internal/schema"
)

// SchemaError 는 만들기(Generate)에서 난 실패다 — 이름 충돌처럼 스키마를 고쳐야 하는 것.
// 쓰기(Write) 실패와 가르려고 싼다. CLI 는 종료 3 과 5, 웹은 400 과 500 으로 가른다.
type SchemaError struct{ Err error }

func (e *SchemaError) Error() string { return e.Err.Error() }

// Build 는 만들기와 쓰기를 한 번에 한다. CLI `gen` 과 웹 `POST /api/gen` 이 같이 쓴다.
//
// 검증(validate)은 하지 않는다 — 부르는 쪽이 먼저 한다 (설계 5장 「먼저 validate」).
// written 은 쓴 파일 이름(가나다 차례), stale 은 이번에 안 나온 옛 생성 파일이다.
func Build(f *schema.File, dir string) (written []string, stale []string, err error) {
	files, err := Generate(f)
	if err != nil {
		return nil, nil, &SchemaError{Err: err}
	}
	stale, err = Write(dir, files)
	if err != nil {
		return nil, nil, err
	}
	written = make([]string, 0, len(files))
	for name := range files {
		written = append(written, name)
	}
	sort.Strings(written)
	if stale == nil {
		stale = []string{}
	}
	return written, stale, nil
}
