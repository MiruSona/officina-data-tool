package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/mirusona/officina-data-tool/internal/serve"
)

// cmdServe 는 표 편집 UI 를 띄운다 (설계 9장).
//
//	serve [--data DIR] [--port N] [--open] [--json]
//
// 127.0.0.1 에만 붙고 프로세스가 사는 동안만 산다. --port 를 안 주면 빈 포트를 받는다.
// 파일을 쓰는 것은 UI 가 저장을 눌렀을 때뿐이다.
func cmdServe(opts options, rest []string) int {
	port, open, err := parseServeArgs(rest)
	if err != nil {
		return fail(opts, exitUsage, err.Error())
	}

	root, code, err := resolveDataRoot(opts)
	if err != nil {
		return fail(opts, code, err.Error())
	}
	server, err := serve.New(root.dir)
	if err != nil {
		return failLoad(opts, err)
	}
	ln, err := server.Listen(port)
	if err != nil {
		return fail(opts, exitUsage, fmt.Sprintf("포트를 못 잡았다: %v", err))
	}

	url := server.URL(ln)
	announce(opts, url, server.Root())
	if open {
		if err := serve.Open(url); err != nil {
			// 주소는 이미 찍혔다. 브라우저를 못 열었다고 서버를 접지 않는다.
			fmt.Fprintln(os.Stderr, "브라우저를 못 열었다 (주소를 손으로 연다):", err)
		}
	}
	if err := server.Serve(ln); err != nil {
		return fail(opts, exitUsage, err.Error())
	}
	return exitOK
}

// announce 는 주소를 한 줄로 알린다. --json 이면 한 덩어리로 낸다 (AI·스크립트용).
func announce(opts options, url, root string) {
	if opts.json {
		printJSON(map[string]any{"ok": true, "exit": exitOK, "url": url, "data": root})
		return
	}
	fmt.Println(url)
	fmt.Printf("데이터 폴더 : %s\n", root)
	fmt.Println("멈추려면 Ctrl+C.")
}

func parseServeArgs(rest []string) (int, bool, error) {
	port := 0
	open := false
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		switch arg {
		case "--open":
			open = true
		case "--port":
			if i+1 >= len(rest) {
				return 0, false, fmt.Errorf("--port 뒤에 번호가 없다")
			}
			i++
			n, err := strconv.Atoi(rest[i])
			if err != nil || n < 0 || n > 65535 {
				return 0, false, fmt.Errorf("--port 가 0~65535 가 아니다: %q", rest[i])
			}
			port = n
		default:
			return 0, false, fmt.Errorf("serve 가 모르는 인자다: %q (쓸 수 있는 것: --port N, --open)", arg)
		}
	}
	return port, open, nil
}
