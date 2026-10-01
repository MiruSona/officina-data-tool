// Package assettest 는 asset 시험이 같이 쓰는 판 깔기다. 시험에서만 부른다.
package assettest

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Layout 은 임시 Unity 뿌리 하나다. GameData 가 데이터 폴더, Index 가 색인 자리다.
type Layout struct {
	Root     string
	GameData string
	Index    string
}

// New 는 Testdata/asset 을 임시 폴더에 깐다. 색인은 Testdata/asset/address-index.json 이다.
func New(t testing.TB) Layout {
	t.Helper()
	src := testdata(t)
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join(src, "unity"))); err != nil {
		t.Fatal(err)
	}
	l := Layout{
		Root:     root,
		GameData: filepath.Join(root, "GameData"),
		Index:    filepath.Join(root, "Library", "AssetTool", "address-index.json"),
	}
	mustMkdir(t, l.GameData)
	mustMkdir(t, filepath.Dir(l.Index))
	Copy(t, filepath.Join(src, "schema.json"), filepath.Join(l.GameData, "schema.json"))
	Copy(t, filepath.Join(src, "card.json"), filepath.Join(l.GameData, "card.json"))
	Copy(t, filepath.Join(src, "address-index.json"), l.Index)
	return l
}

// Path 는 Testdata/asset 아래 파일 자리다.
func Path(t testing.TB, name string) string {
	t.Helper()
	return filepath.Join(testdata(t), filepath.FromSlash(name))
}

// Write 는 파일 하나를 쓴다. 폴더가 없으면 만든다.
func Write(t testing.TB, path, content string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Copy 는 파일 하나를 베낀다.
func Copy(t testing.TB, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Dir(to))
	if err := os.WriteFile(to, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t testing.TB, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

// testdata 는 이 파일 자리에서 Testdata/asset 을 찾는다. 부르는 묶음의 깊이와 상관없다.
func testdata(t testing.TB) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("자기 자리를 못 찾았다")
	}
	return filepath.Join(filepath.Dir(here), "..", "..", "Testdata", "asset")
}
