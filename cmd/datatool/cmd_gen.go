package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mirusona/officina-data-tool/internal/gen"
)

// defaultGenDir 은 데이터 폴더 기준 생성 폴더다 (.datatool.json 의 "gen" 기본값과 같다).
// 생성 파일만 모이는 폴더를 따로 가른다 — 그래야 통째로 덮어도 사람 코드가 안 날아간다 (설계 8장).
const defaultGenDir = "../Assets/_Project/Scripts/Data/Generated"

// cmdGen 은 스키마에서 C# 코드를 만든다 (설계 5장·8장).
//
// 데이터 파일은 안 읽는다 — 생성 코드는 스키마만 보고 태어난다.
func cmdGen(opts options, rest []string) int {
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
	sch, tables, code, err := loadAndValidate(opts, root.dir)
	if code != exitOK {
		return reportProblems(opts, tables, code, err)
	}

	files, err := gen.Generate(sch)
	if err != nil {
		return fail(opts, exitSchema, err.Error())
	}
	files, err = applyNamespace(files, sch.Namespace)
	if err != nil {
		return fail(opts, exitWriteFail, err.Error())
	}

	out = root.outPath(out, root.cfg.Gen, defaultGenDir)
	stale, err := gen.Write(out, files)
	if err != nil {
		return fail(opts, exitWriteFail, err.Error())
	}
	return reportGen(opts, out, files, stale)
}

// applyNamespace 는 생성 내용에 남은 템플릿 네임스페이스를 갈아 끼운다.
//
// 생성 파일은 스키마의 namespace 를 박고 태어나므로 보통은 바꿀 것이 없다. 그래도 한 번
// 통과시키는 이유는 **`Officina.` 잔여를 0 으로 세려는 것**이다 (설계 8장).
// 다만 스키마의 namespace 자체가 `Officina.` 로 시작하면 그 글자가 잔여로 보이니 건너뛴다.
func applyNamespace(files map[string]string, ns string) (map[string]string, error) {
	if strings.HasPrefix(strings.TrimSpace(ns), "Officina.") {
		return files, nil
	}
	out := make(map[string]string, len(files))
	for name, src := range files {
		replaced, err := gen.ReplaceNamespace(src, ns)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out[name] = replaced
	}
	return out, nil
}

func reportGen(opts options, dir string, files map[string]string, stale []string) int {
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
			"ok":      true,
			"exit":    exitOK,
			"dir":     filepath.ToSlash(dir),
			"written": written,
			"stale":   stale,
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
