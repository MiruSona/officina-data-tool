package main

import "fmt"

// 빌드할 때 -ldflags 로 박는다. 안 박으면 개발판이다.
var (
	version     = "0.1.0-dev"
	buildTime   = "미상"
	buildCommit = ""
)

// bin 은 git 에 안 올라가니, 실행 파일이 어느 소스로 빌드됐는지 스스로 알려야 한다.
func commitLabel() string {
	if buildCommit == "" {
		return "dev"
	}
	return buildCommit
}

func cmdVersion(opts options) int {
	if opts.json {
		printJSON(map[string]any{
			"ok":          true,
			"exit":        exitOK,
			"version":     version,
			"buildCommit": commitLabel(),
			"buildTime":   buildTime,
		})
		return exitOK
	}
	fmt.Printf("datatool %s (커밋 %s · 빌드 %s)\n", version, commitLabel(), buildTime)
	return exitOK
}
