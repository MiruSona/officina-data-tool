package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/table"
	"github.com/mirusona/officina-data-tool/internal/validate"
)

// cmdValidate 는 스키마와 데이터를 읽고 설계 6장의 규칙을 다 돌린다.
// 한 글자도 쓰지 않는다.
func cmdValidate(opts options, rest []string) int {
	if len(rest) > 0 {
		return fail(opts, exitUsage, fmt.Sprintf("validate 는 인자를 안 받는다: %v", rest))
	}

	root, code, err := resolveDataRoot(opts)
	if err != nil {
		return fail(opts, code, err.Error())
	}

	_, tables, code, err := loadAndValidate(opts, root.dir)
	if code != exitOK {
		return reportProblems(opts, tables, code, err)
	}
	return reportOK(opts, tables)
}

// problemsError 는 검증에서 걸린 것들이다.
// 읽기 실패와 가르려고 싸 두고, --json 을 낼 때 다시 풀어 쓴다.
type problemsError struct{ problems []*validate.Problem }

func (e *problemsError) Error() string { return validate.Text(e.problems) }

// loadAndValidate 는 스키마·데이터를 읽고 검증까지 한 번에 한다.
//
// `export`·`gen` 이 「먼저 validate」(설계 5장)를 이 함수 하나로 한다 —
// 굽기 전에 검사하는 자리가 둘로 갈리면 한쪽만 고치는 일이 생긴다.
// code 가 exitOK 면 err 은 nil 이고 들고 온 것을 그대로 쓰면 된다.
// 검증에서 걸리면 code 는 exitData, err 은 *problemsError 다.
func loadAndValidate(opts options, dir string) (*schema.File, map[string]*table.Table, int, error) {
	sch, err := schema.Load(filepath.Join(dir, table.SchemaFileName))
	if err != nil {
		return nil, nil, loadExitCode(err), err
	}
	tables, err := table.LoadAll(dir, sch)
	if err != nil {
		return sch, nil, loadExitCode(err), err
	}
	if problems := validate.Run(sch, tables); len(problems) > 0 {
		// 자리는 **데이터 폴더 기준 상대경로**로 낸다 (설계 5장의 보기 꼴).
		validate.Relativize(problems, dir)
		return sch, tables, exitData, &problemsError{problems: problems}
	}
	return sch, tables, exitOK, nil
}

// loadExitCode 는 읽기 중 난 오류의 종류로 종료 코드를 가른다 — 고칠 파일이 다르다.
func loadExitCode(err error) int {
	switch err.(type) {
	case *schema.Errors:
		return exitSchema
	case *table.SetError:
		return exitData
	}
	return exitReadFail
}

// reportProblems 는 걸린 것을 한 줄씩 낸다. 첫 건에서 안 멈춘다 (설계 6장).
func reportProblems(opts options, tables map[string]*table.Table, code int, err error) int {
	typed, ok := err.(*problemsError)
	if !ok {
		return fail(opts, code, err.Error())
	}
	if opts.json {
		printJSON(map[string]any{
			"ok":     false,
			"exit":   code,
			"errors": typed.problems,
			"counts": counts(tables, len(typed.problems)),
		})
		return code
	}
	for _, line := range validate.Lines(typed.problems) {
		fmt.Fprintln(os.Stderr, line)
	}
	return code
}

func reportOK(opts options, tables map[string]*table.Table) int {
	if opts.json {
		printJSON(map[string]any{
			"ok":     true,
			"exit":   exitOK,
			"errors": []*validate.Problem{},
			"counts": counts(tables, 0),
		})
		return exitOK
	}
	fmt.Printf("OK %d tables, %d rows\n", len(tables), rowCount(tables))
	return exitOK
}

func counts(tables map[string]*table.Table, errors int) map[string]any {
	return map[string]any{
		"tables": len(tables),
		"rows":   rowCount(tables),
		"errors": errors,
	}
}

func rowCount(tables map[string]*table.Table) int {
	rows := 0
	for _, t := range tables {
		rows += len(t.Rows)
	}
	return rows
}
