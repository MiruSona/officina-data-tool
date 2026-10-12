package serve

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// itemRows 는 시험 자료 item 표 그대로에서 sword_iron 의 atk 만 바꾼 행 배열이다.
func itemRows(atk int) string {
	return fmt.Sprintf(`[{"id":"sword_iron","name":"철검","atk":%d,"price":300,"tags":["weapon","melee"]},
	{"id":"sword_steel","name":"강철검","atk":24,"price":900,"grade":"rare","tags":["weapon","melee"]},
	{"id":"bow_short","name":"단궁","atk":9,"price":250,"tags":["weapon","ranged"]},
	{"id":"potion_hp","name":"체력 물약","price":50,"usable":true,"tags":["consume"]},
	{"id":"potion_mp","name":"마나 물약","price":60,"usable":true,"tags":["consume"]},
	{"id":"gem_fire","name":"불의 보석","price":1200,"grade":"epic"}]`, atk)
}

func diskRev(t *testing.T, path string) string {
	t.Helper()
	_, rev, err := fileRev(path)
	if err != nil {
		t.Fatal(err)
	}
	return rev
}

// 머리 없음 428 · 틀린 rev 409 — 둘 다 한 글자도 안 쓴다.
// 같은 rev 로 내용이 다른 PUT 을 두 번 보내면 첫째만 된다 (표저장rev설계 5-1).
func TestPutTableNeedsMatchingRev(t *testing.T) {
	ts, s, dir := newTestServer(t)
	path := filepath.Join(dir, "item.json")
	before, _ := os.ReadFile(path)
	rev := tableRev(t, ts, s, "item")

	code, body := putTable(t, ts, s, "item", "", itemRows(13))
	if code != http.StatusPreconditionRequired {
		t.Fatalf("If-Match 없이 %d 다 (428 이어야 한다): %v", code, body)
	}
	code, body = putTable(t, ts, s, "item", strings.Repeat("0", 64), itemRows(13))
	if code != http.StatusConflict || body["rev"] != rev {
		t.Fatalf("틀린 rev 인데 %d %v 다 (409 + 디스크 rev 여야 한다)", code, body)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Fatal("428·409 인데 파일이 바뀌었다")
	}

	// 따옴표로 싼 rev 도 받는다 (스키마와 같다).
	if code, body = putTable(t, ts, s, "item", `"`+rev+`"`, itemRows(13)); code != http.StatusOK {
		t.Fatalf("맞는 rev 인데 %d 다: %v", code, body)
	}
	written, _ := os.ReadFile(path)
	if code, body = putTable(t, ts, s, "item", rev, itemRows(14)); code != http.StatusConflict {
		t.Fatalf("옛 rev 로 둘째 저장이 %d 다 (409 여야 한다): %v", code, body)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(written, after) {
		t.Fatal("둘째 409 인데 파일이 바뀌었다")
	}
}

// 200 의 rev 는 쓴 바이트의 rev 다 — 그 rev 로 연달아 또 저장할 수 있다.
func TestPutTableReturnsNewRev(t *testing.T) {
	ts, s, dir := newTestServer(t)
	path := filepath.Join(dir, "item.json")

	code, body := putTable(t, ts, s, "item", tableRev(t, ts, s, "item"), itemRows(13))
	if code != http.StatusOK {
		t.Fatalf("저장이 %d 다: %v", code, body)
	}
	got, _ := body["rev"].(string)
	if got != tableRev(t, ts, s, "item") || got != diskRev(t, path) {
		t.Fatalf("200 의 rev %q 가 GET·디스크 rev 와 다르다", got)
	}
	if code, body = putTable(t, ts, s, "item", got, itemRows(14)); code != http.StatusOK {
		t.Fatalf("새 rev 로 둘째 저장이 %d 다: %v", code, body)
	}
	if !strings.Contains(readString(t, path), `"atk":14`) {
		t.Fatal("둘째 저장이 안 쓰였다")
	}
}

// BOM 과 CRLF 만 바뀌어도 rev 가 바뀐다. 다시 받은 rev 로 쓰면 BOM·CR 이 걷힌다.
func TestPutTableBOMChangesRev(t *testing.T) {
	ts, s, dir := newTestServer(t)
	path := filepath.Join(dir, "item.json")
	old := tableRev(t, ts, s, "item")

	raw, _ := os.ReadFile(path)
	crlf := bytes.ReplaceAll(raw, []byte("\n"), []byte("\r\n"))
	if err := os.WriteFile(path, append([]byte("\xef\xbb\xbf"), crlf...), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, body := putTable(t, ts, s, "item", old, itemRows(12)); code != http.StatusConflict {
		t.Fatalf("BOM·CRLF 만 바뀌었는데 옛 rev 가 %d 다 (409 여야 한다): %v", code, body)
	}

	fresh := tableRev(t, ts, s, "item")
	if fresh == old {
		t.Fatal("BOM·CRLF 를 더했는데 rev 가 같다")
	}
	if code, body := putTable(t, ts, s, "item", fresh, itemRows(12)); code != http.StatusOK {
		t.Fatalf("새 rev 로 저장이 %d 다: %v", code, body)
	}
	after := readString(t, path)
	if strings.HasPrefix(after, "\xef\xbb\xbf") || strings.Contains(after, "\r") {
		t.Fatalf("쓴 파일에 BOM 이나 CR 이 남았다: %q", after[:20])
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// W/ 접두는 걷고 견준다. 「*」 는 아무 판이나 덮어쓰라는 뜻이라 428 로 거절한다 — 표·스키마 둘 다.
func TestIfMatchWeakAndStar(t *testing.T) {
	ts, s, dir := newTestServer(t)
	path := filepath.Join(dir, "item.json")
	before, _ := os.ReadFile(path)

	for _, star := range []string{"*", `"*"`} {
		if code, body := putTable(t, ts, s, "item", star, itemRows(13)); code != http.StatusPreconditionRequired {
			t.Fatalf("표 If-Match %s 가 %d 다 (428 이어야 한다): %v", star, code, body)
		}
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Fatal("「*」 를 거절했는데 파일이 바뀌었다")
	}
	rev := tableRev(t, ts, s, "item")
	if code, body := putTable(t, ts, s, "item", `W/"`+rev+`"`, itemRows(13)); code != http.StatusOK {
		t.Fatalf("W/ 붙은 맞는 rev 가 %d 다: %v", code, body)
	}

	schemaRev, source := schemaSource(t, ts, s)
	if code, body := putSchema(t, ts, s, "*", editBody(source, "[]")); code != http.StatusPreconditionRequired {
		t.Fatalf("스키마 If-Match * 가 %d 다 (428 이어야 한다): %v", code, body)
	}
	if code, body := putSchema(t, ts, s, `W/"`+schemaRev+`"`, editBody(source, "[]")); code != http.StatusOK {
		t.Fatalf("스키마 W/ 붙은 맞는 rev 가 %d 다: %v", code, body)
	}
}

// 표 GET 은 스키마와 표 바이트를 같은 잠금 안에서 읽는다 — 스키마 PUT 과 겨뤄도 옛 열 + 새 표가 짝이 되지 않는다.
// 잠금을 쥔 사이 schema.json 을 바꾸면, 기다리던 GET 은 바뀐 스키마의 열을 내야 한다.
func TestGetTableReadsSchemaUnderLock(t *testing.T) {
	ts, s, dir := newTestServer(t)
	schemaPath := filepath.Join(dir, "schema.json")
	raw, _ := os.ReadFile(schemaPath)
	changed := strings.Replace(string(raw), `"max": 9999`, `"max": 8888`, 1)
	if changed == string(raw) {
		t.Fatal("시험 자료에 atk max 9999 가 없다")
	}

	s.mu.Lock()
	done := make(chan map[string]any)
	go func() {
		_, body := call(t, ts, s, http.MethodGet, "/api/table/item", "")
		done <- body
	}()
	time.Sleep(200 * time.Millisecond) // GET 이 잠금 앞까지 오게 둔다
	if err := os.WriteFile(schemaPath, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	s.mu.Unlock()

	body := <-done
	for _, c := range body["columns"].([]any) {
		col := c.(map[string]any)
		if col["name"] == "atk" && col["max"] != float64(8888) {
			t.Fatalf("잠금 밖에서 읽은 옛 스키마다: atk max %v", col["max"])
		}
	}
}
