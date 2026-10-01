package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/mirusona/officina-data-tool/internal/gen"
	"github.com/mirusona/officina-data-tool/internal/validate"
)

// defaultGenDir 은 데이터 폴더 기준 생성 폴더다 (.datatool.json 의 "gen" 기본값과 같다).
// 생성 파일만 모이는 폴더를 따로 가른다 — 그래야 통째로 덮어도 사람 코드가 안 날아간다 (설계 8장).
const defaultGenDir = "../Assets/_Project/Scripts/Data/Generated"

// cmdGen 은 스키마에서 C# 코드를 만든다 (설계 5장·8장).
//
// 데이터 파일은 안 읽는다 — 생성 코드는 스키마만 보고 태어난다.
func cmdGen(opts options, rest []string) int {
	rest, requireIndex := takeFlag(rest, requireIndexFlag)
	out, err := parseOutArg("gen", rest)
	if err != nil {
		return fail(opts, exitUsage, err.Error())
	}

	root, code, err := resolveDataRoot(opts)
	if err != nil {
		return fail(opts, code, err.Error())
	}

	// 생성 코드는 스키마만 보고 태어나지만, 굽기와 마찬가지로 **먼저 validate** 다 (설계 5장).
	// 데이터가 스키마와 어긋난 채로 C# 만 새로 나오면 어긋남이 Unity 에서야 드러난다.
	sch, tables, warnings, code, err := loadAndValidate(opts, root, requireIndex)
	if code != exitOK {
		return reportProblems(opts, tables, warnings, code, err)
	}

	// 네임스페이스는 gen.Generate 안에서 이미 박힌다 — 생성 .cs 는 스키마의 namespace 로 태어나고,
	// 손으로 쓴 Unity 셋은 runtimeFiles 가 ReplaceNamespace 로 갈아 끼우며 「Officina. 잔여 0」을 센다.
	// 여기서 또 치환하면 `Studio.Officina.Data` 같은 이름이 두 번 갈려 망가진다.
	files, err := gen.Generate(sch)
	if err != nil {
		return fail(opts, exitSchema, err.Error())
	}

	out = root.outPath(out, root.cfg.Gen, defaultGenDir)
	stale, err := gen.Write(out, files)
	if err != nil {
		return fail(opts, exitWriteFail, err.Error())
	}
	return reportGen(opts, out, files, stale, warnings)
}

func reportGen(opts options, dir string, files map[string]string, stale []string, warnings []*validate.Problem) int {
	written := make([]string, 0, len(files))
	for name := range files {
		written = append(written, name)
	}
	sort.Strings(written)
	if stale == nil {
		stale = []string{}
	}

	if opts.json {
		printJSON(map[string]any{
			"ok":       true,
			"exit":     exitOK,
			"dir":      filepath.ToSlash(dir),
			"written":  written,
			"stale":    stale,
			"warnings": warnings,
		})
		return exitOK
	}

	fmt.Println("만들었다 : " + filepath.ToSlash(dir))
	for _, name := range written {
		fmt.Println("  " + name)
	}
	if len(stale) > 0 {
		// 지우지 않는다 (설계 8장). 표를 없앴을 때 남는 파일은 사람이 보고 지운다.
		fmt.Fprintln(os.Stderr, "\n이번에 안 나온 옛 생성 파일이 남아 있다 (직접 지운다) :")
		for _, name := range stale {
			fmt.Fprintln(os.Stderr, "  "+name)
		}
	}
	return exitOK
}
