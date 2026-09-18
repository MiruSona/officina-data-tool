package main

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/table"
)

// cmdFmt 는 데이터 JSON 을 설계 3장의 규칙대로 다시 쓴다.
//
// --check 면 한 글자도 안 쓰고 「바뀔 파일이 있나」만 알린다 (있으면 종료 2).
// CI 나 커밋 훅이 이 꼴로 부른다.
func cmdFmt(opts options, rest []string) int {
	check := false
	for _, arg := range rest {
		if arg != "--check" {
			return fail(opts, exitUsage, fmt.Sprintf("fmt 가 모르는 인자다: %q (쓸 수 있는 것: --check)", arg))
		}
		check = true
	}

	root, code, err := resolveDataRoot(opts)
	if err != nil {
		return fail(opts, code, err.Error())
	}
	dir := root.dir

	sch, err := schema.Load(filepath.Join(dir, table.SchemaFileName))
	if err != nil {
		return failLoad(opts, err)
	}
	tables, err := table.LoadAll(dir, sch)
	if err != nil {
		return failLoad(opts, err)
	}

	changed, err := formatTables(sch, tables, check)
	if err != nil {
		return failLoad(opts, err)
	}
	return reportFmt(opts, changed, check)
}

// formatTables 는 표마다 규칙대로 다시 적어보고 달라진 파일 이름을 모은다.
// check 가 아니면 달라진 것만 실제로 쓴다 — 안 바뀐 파일의 수정 시각을 안 흔든다.
func formatTables(sch *schema.File, tables map[string]*table.Table, check bool) ([]string, error) {
	changed := []string{}
	for _, name := range sch.TableNames() {
		t := tables[name]
		out, err := t.Format(sch.Table(name))
		if err != nil {
			return nil, err
		}
		if bytes.Equal(out, t.Raw) {
			continue
		}
		changed = append(changed, filepath.ToSlash(t.Path))
		if check {
			continue
		}
		if err := table.WriteFile(t.Path, out); err != nil {
			return nil, &writeError{err: err}
		}
	}
	return changed, nil
}

// writeError 는 쓰다 실패한 것이다. 읽기 실패(4)와 가르려고 싸 둔다.
type writeError struct{ err error }

func (e *writeError) Error() string { return e.err.Error() }

// failLoad 는 오류 종류로 종료 코드를 가른다.
// 스키마가 틀린 것(3)·데이터 파일 집합이 틀린 것(2)·읽기 실패(4)는 고칠 파일이 다르다.
func failLoad(opts options, err error) int {
	switch typed := err.(type) {
	case *schema.Errors:
		return fail(opts, exitSchema, typed.Error())
	case *table.SetError:
		return fail(opts, exitData, typed.Error())
	case *writeError:
		return fail(opts, exitWriteFail, typed.Error())
	}
	return fail(opts, exitReadFail, err.Error())
}

func reportFmt(opts options, changed []string, check bool) int {
	code := exitOK
	if check && len(changed) > 0 {
		code = exitData
	}
	if opts.json {
		printJSON(map[string]any{
			"ok":      code == exitOK,
			"exit":    code,
			"changed": changed,
		})
		return code
	}
	if len(changed) == 0 {
		fmt.Println("다 규칙대로다.")
		return code
	}
	if check {
		fmt.Println("규칙과 다른 파일이 있다 (datatool fmt 로 고친다) :")
	} else {
		fmt.Println("다시 썼다 :")
	}
	for _, path := range changed {
		fmt.Println("  " + path)
	}
	return code
}
