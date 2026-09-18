// datatool 은 게임 데이터 원본(JSON)을 검증하고 Unity 로 내보내는 툴이다.
//
// 이 파일은 인자를 가르기만 한다. 실제 일은 internal 묶음이 한다 (설계 2장).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// 종료 코드 (설계 5장). 2 와 3 을 가르는 이유는 고칠 파일이 다르기 때문이다.
const (
	exitOK        = 0 // 성공
	exitUsage     = 1 // 사용법 잘못
	exitData      = 2 // 데이터 검증 실패
	exitSchema    = 3 // 스키마 자체가 틀림
	exitReadFail  = 4 // 읽기 실패 (없음·JSON 깨짐·경로 감옥)
	exitWriteFail = 5 // 쓰기 실패
)

// options 는 모든 명령이 함께 받는 전역 옵션이다.
type options struct {
	// 데이터 폴더(GameData). 안 주면 명령이 알아서 정한다.
	data string
	// AI 가 부르기 좋게 결과를 JSON 한 덩어리로 낸다.
	json bool
}

// notYet 은 아직 안 만든 명령과 그 명령이 만들어질 소단계다.
// 지금은 비었다 — 설계 5장의 일곱 명령이 다 있다.
var notYet = map[string]string{}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	command, rest, opts, err := parseArgs(args)
	if err != nil {
		return fail(opts, exitUsage, err.Error())
	}
	if command == "" || command == "help" || command == "--help" || command == "-h" {
		fmt.Print(usage())
		return exitOK
	}

	switch command {
	case "init":
		return cmdInit(opts, rest)
	case "fmt":
		return cmdFmt(opts, rest)
	case "validate":
		return cmdValidate(opts, rest)
	case "export":
		return cmdExport(opts, rest)
	case "gen":
		return cmdGen(opts, rest)
	case "serve":
		return cmdServe(opts, rest)
	case "version":
		return cmdVersion(opts)
	}

	if step, ok := notYet[command]; ok {
		return fail(opts, exitUsage, fmt.Sprintf("%s 는 아직 없다 (%s 에서 만든다)", command, step))
	}
	return fail(opts, exitUsage, fmt.Sprintf("모르는 명령이다: %q\n\n%s", command, usage()))
}

// parseArgs 는 전역 옵션을 걷어내고 명령과 나머지 인자를 남긴다.
func parseArgs(args []string) (string, []string, options, error) {
	opts := options{}
	command := ""
	rest := []string{}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			opts.json = true
		case arg == "--data":
			if i+1 >= len(args) {
				return command, rest, opts, fmt.Errorf("--data 뒤에 폴더가 없다")
			}
			i++
			opts.data = args[i]
		case strings.HasPrefix(arg, "--data="):
			opts.data = strings.TrimPrefix(arg, "--data=")
		case command == "":
			command = arg
		default:
			rest = append(rest, arg)
		}
	}
	return command, rest, opts, nil
}

// fail 은 오류 한 줄을 사람 눈 또는 --json 으로 내고 종료 코드를 준다.
func fail(opts options, code int, message string) int {
	if opts.json {
		printJSON(map[string]any{
			"ok":     false,
			"exit":   code,
			"errors": []map[string]any{{"message": message}},
		})
		return code
	}
	fmt.Fprintln(os.Stderr, message)
	return code
}

func printJSON(v map[string]any) {
	out, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintln(os.Stderr, "결과를 JSON 으로 못 적었다:", err)
		return
	}
	fmt.Println(string(out))
}
