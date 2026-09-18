package serve

import (
	"os/exec"
	"runtime"
)

// Open 은 기본 브라우저로 주소를 연다.
//
// 실패해도 서버는 계속 산다 — 주소는 이미 화면에 찍혔으니 사람이 손으로 열면 된다.
// Windows 에서 `cmd /c start` 대신 rundll32 를 쓰는 것은, start 가 & 가 든 주소를
// 따옴표 규칙 때문에 잘라 먹기 때문이다. 우리 주소에는 ?t= 뒤에 토큰이 붙는다.
func Open(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
