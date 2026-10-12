package serve

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
)

// schemaRevHeader 는 표 PUT 이 「이 스키마 판을 보고 고쳤다」고 알리는 고른 머리다.
// 없으면 안 본다 — If-Match 만 싣는 옛 스크립트가 안 깨지게.
const schemaRevHeader = "X-Schema-Rev"

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

// ifMatchRev 는 If-Match 머리의 rev 다.
// 비었거나 「*」(아무 판이나 덮어쓰기)면 "" 다 — 부르는 쪽이 428 로 거절한다. 강제 덮어쓰기 길은 없다.
func ifMatchRev(r *http.Request) string {
	return revHeader(r, "If-Match")
}

// revHeader 는 rev 를 실은 머리(If-Match · X-Schema-Rev)를 한 가지로 걷는다 — W/ 접두와 따옴표를 떼고, 「*」 는 "" 로 본다.
// 두 머리가 다르게 걷히면 같은 rev 를 실어도 한쪽만 409 가 된다.
func revHeader(r *http.Request, name string) string {
	want := strings.TrimSpace(r.Header.Get(name))
	want = strings.Trim(strings.TrimPrefix(want, "W/"), `"`)
	if want == "*" {
		return ""
	}
	return want
}
