package serve

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
)

// fileRev 는 파일의 디스크 바이트와 rev(sha256 hex)를 준다.
// rev 는 BOM·줄끝까지 포함한 원본 바이트로 낸다 — 손으로 고친 것도 「바뀜」으로 잡는다.
func fileRev(path string) ([]byte, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return raw, revOf(raw), nil
}

// revOf 는 바이트의 rev 다. 방금 쓴 바이트의 새 rev 를 낼 때 쓴다.
func revOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ifMatchRev 는 If-Match 머리의 rev 다. W/ 접두와 따옴표를 걷는다.
// 비었거나 「*」(아무 판이나 덮어쓰기)면 "" 다 — 부르는 쪽이 428 로 거절한다. 강제 덮어쓰기 길은 없다.
func ifMatchRev(r *http.Request) string {
	want := strings.TrimSpace(r.Header.Get("If-Match"))
	want = strings.Trim(strings.TrimPrefix(want, "W/"), `"`)
	if want == "*" {
		return ""
	}
	return want
}
