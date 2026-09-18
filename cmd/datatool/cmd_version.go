package main

import "fmt"

// 빌드할 때 -ldflags 로 박는다. 안 박으면 개발판이다.
var (
	version   = "0.1.0-dev"
	buildTime = "미상"
)

func cmdVersion(opts options) int {
	if opts.json {
		printJSON(map[string]any{
			"ok":        true,
			"exit":      exitOK,
			"version":   version,
			"buildTime": buildTime,
		})
		return exitOK
	}
	fmt.Printf("datatool %s (빌드 %s)\n", version, buildTime)
	return exitOK
}
