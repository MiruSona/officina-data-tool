package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceNamespace(t *testing.T) {
	src := "namespace Officina.Data\n{\n    // Officina.Data 안이다\n}\n"
	got, err := ReplaceNamespace(src, "MyGame.Data")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "Officina.") {
		t.Errorf("치환 잔여가 있다 : %s", got)
	}
	if !strings.Contains(got, "namespace MyGame.Data") {
		t.Errorf("치환이 안 됐다 : %s", got)
	}
}

func TestReplaceNamespaceRejectsLeftover(t *testing.T) {
	src := "using Officina.Hubs;\nnamespace Officina.Data\n{\n}\n"
	if _, err := ReplaceNamespace(src, "MyGame.Data"); err == nil {
		t.Fatal("Officina.Hubs 가 남았는데 통과했다")
	}
	if _, err := ReplaceNamespace("namespace Officina.Data { }", ""); err == nil {
		t.Fatal("빈 namespace 를 받았다")
	}
}

// Unity/ 의 손으로 쓴 파일이 치환 규칙을 지키는가 — Officina.Data 말고 다른 Officina. 이 없어야 한다.
func TestUnityFilesAreSubstitutable(t *testing.T) {
	dir := "../../Unity"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Unity 폴더를 못 읽었다 : %v", err)
	}
	seen := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".cs") {
			continue
		}
		seen++
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		got, err := ReplaceNamespace(string(raw), "MyGame.Data")
		if err != nil {
			t.Errorf("%s : %v", e.Name(), err)
			continue
		}
		if strings.Contains(got, "Officina.") {
			t.Errorf("%s 에 치환 잔여가 있다", e.Name())
		}
		if !strings.Contains(string(raw), "namespace "+TemplateNamespace) {
			t.Errorf("%s 의 네임스페이스가 %s 가 아니다", e.Name(), TemplateNamespace)
		}
	}
	if seen == 0 {
		t.Fatal("Unity 폴더에 .cs 가 없다")
	}
}
