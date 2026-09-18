package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mirusona/officina-data-tool/internal/bake"
	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/table"
)

// defaultExportPath 는 데이터 폴더 기준 굽는 자리다 (.datatool.json 의 "export" 기본값과 같다).
// GameData/ 가 Assets/ 밖에 있으므로 한 칸 올라간다 (설계 2장).
const defaultExportPath = "../Assets/StreamingAssets/gamedata.bytes"

// cmdExport 는 gamedata.bytes 를 굽는다 (설계 5장·7장).
//
// --force 같은 우회 옵션을 두지 않는다 — 굽기를 억지로 통과시키는 문을 열면 그 문이 기본값이 된다.
func cmdExport(opts options, rest []string) int {
	out, err := parseOutArg("export", rest)
	if err != nil {
		return fail(opts, exitUsage, err.Error())
	}

	root, code, err := resolveDataRoot(opts)
	if err != nil {
		return fail(opts, code, err.Error())
	}

	// 먼저 validate 다 (설계 5장). 검증에서 걸리면 한 바이트도 안 굽는다 —
	// 반쯤 맞는 gamedata.bytes 가 게임에 들어가는 것이 제일 나쁘다.
	sch, tables, code, err := loadAndValidate(opts, root.dir)
	if code != exitOK {
		return reportProblems(opts, tables, code, err)
	}

	data, err := bake.Bake(sch, tables, time.Now())
	if err != nil {
		return failBake(opts, err)
	}

	out = root.outPath(out, root.cfg.Export, defaultExportPath)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return fail(opts, exitWriteFail, fmt.Sprintf("굽는 폴더를 못 만들었다 : %v", err))
	}
	if err := table.WriteFile(out, data); err != nil {
		return fail(opts, exitWriteFail, fmt.Sprintf("%s 를 못 썼다 : %v", filepath.ToSlash(out), err))
	}
	return reportExport(opts, out, data, sch, tables)
}

// parseOutArg 는 --out(-o) 하나만 걷어낸다. 안 주면 빈 문자열이고 부르는 쪽이 기본값을 정한다.
func parseOutArg(command string, rest []string) (string, error) {
	out := ""
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		switch {
		case arg == "--out" || arg == "-o":
			if i+1 >= len(rest) {
				return "", fmt.Errorf("%s 뒤에 자리가 없다", arg)
			}
			i++
			out = rest[i]
		case strings.HasPrefix(arg, "--out="):
			out = strings.TrimPrefix(arg, "--out=")
		default:
			return "", fmt.Errorf("%s 가 모르는 인자다: %q (쓸 수 있는 것: --out)", command, arg)
		}
	}
	return out, nil
}

// failBake 는 굽다 만난 오류의 종료 코드를 가른다.
// 값 하나가 틀린 것(*bake.Error)은 데이터 잘못이라 종료 2 다 — 스키마를 봐도 소용없다.
func failBake(opts options, err error) int {
	if typed, ok := err.(*bake.Error); ok {
		return fail(opts, exitData, typed.Error())
	}
	return fail(opts, exitData, err.Error())
}

func reportExport(opts options, out string, data []byte, sch *schema.File, tables map[string]*table.Table) int {
	rows := 0
	for _, t := range tables {
		rows += len(t.Rows)
	}
	path := filepath.ToSlash(out)

	if opts.json {
		printJSON(map[string]any{
			"ok":         true,
			"exit":       exitOK,
			"out":        path,
			"bytes":      len(data),
			"tables":     len(sch.Tables),
			"rows":       rows,
			"schemaHash": sch.Hash(),
		})
		return exitOK
	}
	fmt.Printf("구웠다 : %s (%d바이트 · 표 %d개 · %d행)\n", path, len(data), len(sch.Tables), rows)
	fmt.Println("schemaHash : " + sch.Hash())
	return exitOK
}
