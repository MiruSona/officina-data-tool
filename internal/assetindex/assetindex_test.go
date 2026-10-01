package assetindex_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mirusona/officina-data-tool/internal/assetindex"
	"github.com/mirusona/officina-data-tool/internal/assettest"
)

const examplePath = "../../Testdata/contract/address-index.example.json"

// 계약 예제(2-2)를 그대로 읽는다. AssetTool 이 같은 바이트를 낸다.
func TestParseContractExample(t *testing.T) {
	ix, err := assetindex.Load(examplePath)
	if err != nil {
		t.Fatalf("계약 예제를 못 읽었다: %v", err)
	}
	if len(ix.Entries) != 3 || ix.UnityRoot != "../.." {
		t.Fatalf("칸을 잘못 읽었다: %d항목 unityRoot=%q", len(ix.Entries), ix.UnityRoot)
	}
	if got := strings.Join(ix.Addresses(), ","); got != "Hero,Sfx/hit.wav,icons" {
		t.Fatalf("주소 차례가 바이트 차례가 아니다: %s", got)
	}

	m := ix.Find("icons[icon_sword]")
	if !m.HasSub || m.Address != "icons" || m.Sub != "icon_sword" || len(m.Entries) != 1 {
		t.Fatalf("하위 에셋을 못 갈랐다: %+v", m)
	}
	if !m.Entries[0].HasSub("icon_sword") || m.Entries[0].HasSub("icon_axe") {
		t.Fatal("sub 이름 찾기가 틀렸다")
	}
	if r := m.Entries[0].Sub[1].Rect; r.X != 64 || r.Y != 64 || r.W != 64 || r.H != 64 {
		t.Fatalf("rect 를 잘못 읽었다: %+v", r)
	}
	if hero := ix.Find("Hero"); len(hero.Entries) != 1 || hero.HasSub {
		t.Fatalf("주소 그대로를 못 찾았다: %+v", hero)
	}
	if hit := ix.Find("Sfx/hit.wav").Entries; len(hit) != 1 || hit[0].Sub != nil || hit[0].FromFolder != "Sfx" {
		t.Fatalf("sub 없는 항목은 nil(모름)이어야 한다: %+v", hit)
	}
	if none := ix.Find("Heroo"); len(none.Entries) != 0 {
		t.Fatal("없는 주소인데 찾았다")
	}
}

func TestSplitSub(t *testing.T) {
	cases := map[string][2]string{"a[b]": {"a", "b"}, "x/y.png[s_1]": {"x/y.png", "s_1"}, "a[b][c]": {"a[b]", "c"}}
	for in, want := range cases {
		a, s, ok := assetindex.SplitSub(in)
		if !ok || a != want[0] || s != want[1] {
			t.Errorf("%q → %q %q %v", in, a, s, ok)
		}
	}
	for _, in := range []string{"a", "[b]", "a[]", "a]"} {
		if _, _, ok := assetindex.SplitSub(in); ok {
			t.Errorf("%q 를 하위 꼴로 봤다", in)
		}
	}
}

// A5. 모르는 계약 버전 · 깨진 JSON · 필수 칸 빠짐은 모두 오류다 (CLI 는 종료 4).
func TestBrokenIndex(t *testing.T) {
	raw, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	cases := map[string]struct{ text, want string }{
		"version":          {strings.Replace(src, `"version": 1`, `"version": 2`, 1), "계약 버전 2"},
		"json":             {src[:len(src)/2], "깨졌다"},
		"top-key":          {strings.Replace(src, `"sourceMtime"`, `"sourceMtimeX"`, 1), `"sourceMtime"`},
		"entry-key":        {strings.Replace(src, `"group": "Audio",`, ``, 1), `entries[1]: 필수 칸 "group"`},
		"kind":             {strings.Replace(src, `"kind": "prefab"`, `"kind": "sprite"`, 1), `kind "sprite"`},
		"time":             {strings.Replace(src, `"2026-09-23T05:12:00Z"`, `"어제"`, 1), "generatedAt"},
		"no-version":       {strings.Replace(src, `"version": 1,`, ``, 1), "version"},
		"wrong-type":       {strings.Replace(src, `"includeInBuild": true,`, `"includeInBuild": "yes",`, 1), "꼴이 틀렸다"},
		"null-required":    {strings.Replace(src, `"labels": [],`, `"labels": null,`, 1), `"labels"`},
		"settings-empty":   {strings.Replace(src, `"Assets/AddressableAssetsData/AddressableAssetSettings.asset"`, `""`, 1), "settingsPath"},
		"settings-ext":     {strings.Replace(src, `AddressableAssetSettings.asset"`, `AddressableAssetSettings.json"`, 1), "settingsPath"},
		"settings-outside": {strings.Replace(src, `"Assets/AddressableAssetsData/`, `"ProjectSettings/`, 1), "settingsPath"},
		"settings-root":    {strings.Replace(src, `"Assets/AddressableAssetsData/AddressableAssetSettings.asset"`, `"Assets/../x.asset"`, 1), "settingsPath"},
	}
	for name, c := range cases {
		_, err := assetindex.Parse([]byte(c.text), "idx.json")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: 오류에 %q 가 없다: %v", name, c.want, err)
		}
	}
}

func TestMissingIndexIsNotExist(t *testing.T) {
	_, err := assetindex.Load(filepath.Join(t.TempDir(), "없음.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("없는 색인인데 다른 오류다: %v", err)
	}
}

// A6 ③. 감옥 뿌리는 데이터 폴더의 부모일 때만 준다. 색인을 고쳐도 넓어지지 않는다.
func TestJailRoot(t *testing.T) {
	l := assettest.New(t)
	ix, err := assetindex.Load(l.Index)
	if err != nil {
		t.Fatal(err)
	}
	root, err := ix.JailRoot(l.GameData)
	if err != nil {
		t.Fatalf("맞는 뿌리인데 막았다: %v", err)
	}
	if want, _ := filepath.EvalSymlinks(l.Root); !strings.EqualFold(root, want) {
		t.Fatalf("뿌리가 %s 인데 %s 를 줬다", want, root)
	}

	for _, bad := range []string{"..", "../../..", `C:\`, l.GameData} {
		ix.UnityRoot = bad
		if _, err := ix.JailRoot(l.GameData); !errors.Is(err, assetindex.ErrRootMismatch) {
			t.Errorf("unityRoot=%q 인데 안 막았다: %v", bad, err)
		}
	}
}

// A4. 설정 폴더의 .asset 이 sourceMtime 보다 새로우면 낡았다.
func TestStale(t *testing.T) {
	l := assettest.New(t)
	ix, err := assetindex.Load(l.Index)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := ix.Stale(l.Root)
	if err != nil || stale {
		t.Fatalf("2100년 색인인데 낡았다고 했다: %v %v", stale, err)
	}

	settings := filepath.Join(l.Root, "Assets", "AddressableAssetsData", "AddressableAssetSettings.asset")
	later := time.Date(2100, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(settings, later, later); err != nil {
		t.Fatal(err)
	}
	if stale, err := ix.Stale(l.Root); err != nil || !stale {
		t.Fatalf("설정이 더 새로운데 안 낡았다고 했다: %v %v", stale, err)
	}

	ix.SettingsPath = "Assets/없는폴더/AddressableAssetSettings.asset"
	if stale, err := ix.Stale(l.Root); err != nil || stale {
		t.Fatalf("설정 폴더가 없으면 낡음을 모른다(거짓): %v %v", stale, err)
	}
	ix.SettingsPath = "../밖/AddressableAssetSettings.asset"
	if _, err := ix.Stale(l.Root); err == nil {
		t.Fatal("뿌리 밖 settingsPath 를 훑었다")
	}
}
