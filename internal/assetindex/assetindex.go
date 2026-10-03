// Package assetindex 는 AssetTool `index` 가 쓴 색인 JSON(계약 버전 1)을 읽는다.
//
// 계약을 아는 곳은 이 묶음 하나다. 계약 글은 README 의 「경계 계약」 절이다.
package assetindex

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/mirusona/officina-data-tool/internal/textfile"
)

// Version 은 이 묶음이 아는 계약 버전이다.
const Version = 1

// 항목 종류 다섯 (계약 2-1 의 kind 표).
const (
	KindImage  = "image"
	KindAudio  = "audio"
	KindPrefab = "prefab"
	KindScene  = "scene"
	KindOther  = "other"
)

// Kinds 는 kind 로 쓸 수 있는 값 전부다. 스키마 열의 kind 칸도 이것을 본다.
var Kinds = []string{KindImage, KindAudio, KindPrefab, KindScene, KindOther}

// ErrRootMismatch 는 색인의 unityRoot 가 데이터 폴더의 부모와 다를 때다.
var ErrRootMismatch = errors.New("색인의 unityRoot 가 데이터 폴더의 부모와 다르다")

var topKeys = []string{"version", "generator", "generatedAt", "unityRoot", "settingsPath", "sourceMtime", "labels", "entries"}

var entryKeys = []string{"address", "guid", "path", "kind", "group", "includeInBuild", "labels"}

// Rect 는 하위 에셋의 픽셀 자리다. y 는 아래에서 잰다 (Unity 꼴).
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Sub 는 스프라이트 시트·아틀라스 안의 하위 에셋 하나다. W·H 가 0 이면 그림 전체다.
// Path 가 있으면 Rect 는 그 파일 기준(아틀라스), 없으면 항목 path 기준(시트)이다. 계약 1판 덧붙임.
type Sub struct {
	Name string `json:"name"`
	Rect Rect   `json:"rect"`
	Path string `json:"path,omitempty"`
	GUID string `json:"guid,omitempty"`
}

// Entry 는 색인 항목 하나다. Sub 가 nil 이면 「하위를 모른다」이다.
type Entry struct {
	Address        string   `json:"address"`
	GUID           string   `json:"guid"`
	Path           string   `json:"path"`
	Kind           string   `json:"kind"`
	Group          string   `json:"group"`
	IncludeInBuild bool     `json:"includeInBuild"`
	Labels         []string `json:"labels"`
	FromFolder     string   `json:"fromFolder,omitempty"`
	Sub            []Sub    `json:"sub,omitempty"`
}

// HasSub 는 하위 이름이 이 항목의 sub 에 있는지 본다.
func (e *Entry) HasSub(name string) bool {
	return e.FindSub(name) != nil
}

// FindSub 는 이름이 맞는 sub 를 준다. 없으면 nil 이다.
func (e *Entry) FindSub(name string) *Sub {
	for i := range e.Sub {
		if e.Sub[i].Name == name {
			return &e.Sub[i]
		}
	}
	return nil
}

// IsAtlas 는 path 확장자로 스프라이트 아틀라스인지 본다. kind 는 other 그대로다 (새 kind 를 안 만든다).
func (e *Entry) IsAtlas() bool {
	ext := strings.ToLower(path.Ext(e.Path))
	return ext == ".spriteatlas" || ext == ".spriteatlasv2"
}

// PathOK 는 정리된 `/` 상대경로이고 Assets/ · Packages/ 로 시작하는지 본다. 항목 path·sub.path 모두 이것으로 막는다.
// 「Assets/../ProjectSettings」 처럼 접두만 맞춘 것도 정리하면 달라지니 막힌다.
func PathOK(p string) bool {
	if strings.ContainsAny(p, "\\:") || path.IsAbs(p) || path.Clean(p) != p || !fs.ValidPath(p) {
		return false
	}
	return strings.HasPrefix(p, "Assets/") || strings.HasPrefix(p, "Packages/")
}

// Index 는 색인 파일 한 장이다.
type Index struct {
	Version      int      `json:"version"`
	Generator    string   `json:"generator"`
	GeneratedAt  string   `json:"generatedAt"`
	UnityRoot    string   `json:"unityRoot"`
	SettingsPath string   `json:"settingsPath"`
	SourceMtime  string   `json:"sourceMtime"`
	Labels       []string `json:"labels"`
	Entries      []*Entry `json:"entries"`

	file      string
	source    time.Time
	byAddress map[string][]*Entry
}

// Match 는 칸 값 하나를 색인에서 찾은 결과다. Entries 가 비면 없는 주소다.
type Match struct {
	Address string
	// HasSub 면 칸 값이 `address[Sub]` 꼴이었다.
	Sub     string
	HasSub  bool
	Entries []*Entry
}

// Load 는 색인 파일을 읽는다. 파일이 없으면 errors.Is(err, os.ErrNotExist) 다.
func Load(file string) (*Index, error) {
	data, err := textfile.ReadFile(file)
	if err != nil {
		return nil, err
	}
	ix, err := Parse(data, filepath.ToSlash(file))
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	ix.file = abs
	return ix, nil
}

// Parse 는 색인 내용을 읽는다. 깨진 JSON · 모르는 계약 버전 · 필수 칸 없음은 오류다.
func Parse(data []byte, name string) (*Index, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("%s: 색인 JSON 이 깨졌다: %v", name, err)
	}
	// 버전을 먼저 본다. 다른 계약이면 칸 이름부터 다를 수 있다.
	var version int
	if err := json.Unmarshal(top["version"], &version); err != nil {
		return nil, fmt.Errorf("%s: 색인에 정수 version 칸이 없다", name)
	}
	if version != Version {
		return nil, fmt.Errorf("%s: 색인 계약 버전 %d 을 모른다. DataTool 을 새로 빌드하거나 AssetTool 버전을 맞춰라", name, version)
	}
	if err := requireKeys(top, topKeys, name); err != nil {
		return nil, err
	}
	var rawEntries []map[string]json.RawMessage
	if err := json.Unmarshal(top["entries"], &rawEntries); err != nil {
		return nil, fmt.Errorf("%s: entries 가 객체 배열이 아니다: %v", name, err)
	}
	for i, one := range rawEntries {
		if err := requireKeys(one, entryKeys, fmt.Sprintf("%s: entries[%d]", name, i)); err != nil {
			return nil, err
		}
	}

	ix := &Index{}
	if err := json.Unmarshal(data, ix); err != nil {
		return nil, fmt.Errorf("%s: 색인 칸 꼴이 틀렸다: %v", name, err)
	}
	if err := ix.finish(name); err != nil {
		return nil, err
	}
	return ix, nil
}

func requireKeys(fields map[string]json.RawMessage, keys []string, where string) error {
	for _, key := range keys {
		raw, ok := fields[key]
		if !ok || strings.TrimSpace(string(raw)) == "null" {
			return fmt.Errorf("%s: 필수 칸 %q 가 없다", where, key)
		}
	}
	return nil
}

// finish 는 시각·kind 를 검사하고 주소 표를 만든다.
func (ix *Index) finish(name string) error {
	if _, err := time.Parse(time.RFC3339Nano, ix.GeneratedAt); err != nil {
		return fmt.Errorf("%s: generatedAt 이 RFC3339 시각이 아니다: %q", name, ix.GeneratedAt)
	}
	source, err := time.Parse(time.RFC3339Nano, ix.SourceMtime)
	if err != nil {
		return fmt.Errorf("%s: sourceMtime 이 RFC3339 시각이 아니다: %q", name, ix.SourceMtime)
	}
	ix.source = source
	if !settingsPathOK(ix.SettingsPath) {
		return fmt.Errorf("%s: settingsPath 는 Assets/·Packages/ 아래 정리된 .asset 상대경로여야 한다: %q", name, ix.SettingsPath)
	}
	ix.byAddress = map[string][]*Entry{}
	for i, e := range ix.Entries {
		if !knownKind(e.Kind) {
			return fmt.Errorf("%s: entries[%d] 의 kind %q 를 모른다 (있는 것: %s)", name, i, e.Kind, strings.Join(Kinds, ", "))
		}
		ix.byAddress[e.Address] = append(ix.byAddress[e.Address], e)
	}
	return nil
}

// settingsPathOK 는 낡음 판정이 뿌리 전체를 훑지 않게 설정 파일 자리를 좁힌다.
func settingsPathOK(p string) bool {
	if strings.ContainsAny(p, "\\:") || path.Clean(p) != p || !fs.ValidPath(p) || !strings.HasSuffix(p, ".asset") {
		return false
	}
	return strings.HasPrefix(p, "Assets/") || strings.HasPrefix(p, "Packages/")
}

func knownKind(kind string) bool {
	for _, k := range Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// File 은 읽은 색인 파일의 절대경로다. Parse 로만 만들었으면 빈 값이다.
func (ix *Index) File() string { return ix.file }

// Find 는 칸 값 하나를 찾는다. 주소 그대로를 먼저 보고, 없으면 `address[이름]` 으로 가른다.
func (ix *Index) Find(value string) Match {
	if entries := ix.byAddress[value]; len(entries) > 0 {
		return Match{Address: value, Entries: entries}
	}
	address, sub, ok := SplitSub(value)
	if !ok {
		return Match{Address: value}
	}
	return Match{Address: address, Sub: sub, HasSub: true, Entries: ix.byAddress[address]}
}

// SplitSub 는 `address[이름]` 을 둘로 가른다. 그 꼴이 아니면 ok 가 거짓이다.
func SplitSub(value string) (string, string, bool) {
	if !strings.HasSuffix(value, "]") {
		return "", "", false
	}
	open := strings.LastIndex(value, "[")
	if open <= 0 || open == len(value)-2 {
		return "", "", false
	}
	return value[:open], value[open+1 : len(value)-1], true
}

// Addresses 는 겹치지 않는 주소를 바이트 차례로 준다.
func (ix *Index) Addresses() []string {
	list := make([]string, 0, len(ix.byAddress))
	for address := range ix.byAddress {
		list = append(list, address)
	}
	sort.Strings(list)
	return list
}

// JailRoot 는 파일을 열 뿌리를 정한다. 뿌리는 늘 데이터 폴더의 부모이고,
// 색인의 unityRoot 가 그곳을 가리킬 때만 준다 — 색인을 고쳐 뿌리를 넓힐 수 없다.
func (ix *Index) JailRoot(dataDir string) (string, error) {
	want := canonical(filepath.Dir(canonical(dataDir)))
	claimed := filepath.FromSlash(ix.UnityRoot)
	if !filepath.IsAbs(claimed) && filepath.VolumeName(claimed) == "" {
		claimed = filepath.Join(filepath.Dir(ix.file), claimed)
	}
	got := canonical(claimed)
	if !samePath(got, want) {
		return "", fmt.Errorf("%w (색인: %s · 데이터 폴더의 부모: %s)", ErrRootMismatch, filepath.ToSlash(got), filepath.ToSlash(want))
	}
	return want, nil
}

func canonical(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	return abs
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Stale 은 settingsPath 폴더 아래 *.asset 중 가장 새 mtime 이 sourceMtime 보다 뒤인지 본다.
// 폴더가 없으면 낡았다고 못 하니 거짓이다. root 는 JailRoot 가 준 뿌리다.
func (ix *Index) Stale(root string) (bool, error) {
	jail, err := os.OpenRoot(root)
	if err != nil {
		return false, err
	}
	defer jail.Close()

	dir := path.Dir(ix.SettingsPath)
	if !fs.ValidPath(dir) {
		return false, fmt.Errorf("settingsPath 가 뿌리 안 상대경로가 아니다: %q", ix.SettingsPath)
	}
	var newest time.Time
	walkErr := fs.WalkDir(jail.FS(), dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(path.Ext(p), ".asset") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	if errors.Is(walkErr, fs.ErrNotExist) {
		return false, nil
	}
	if walkErr != nil {
		return false, walkErr
	}
	return newest.After(ix.source), nil
}

// Status 는 색인을 연 결과 중 경고로 알릴 것이다. File 은 사람이 볼 자리(/ 구분)다.
type Status struct {
	File    string
	Missing bool
	Stale   bool
	Notes   []string
}

// Open 은 V10 이 쓸 색인을 연다. 없으면 Missing 이고 오류가 아니다.
// 깨졌거나 모르는 계약 버전이면 오류다 (CLI 는 종료 4).
func Open(file, dataDir string) (*Index, Status, error) {
	st := Status{File: filepath.ToSlash(file)}
	ix, err := Load(file)
	if errors.Is(err, fs.ErrNotExist) {
		st.Missing = true
		return nil, st, nil
	}
	if err != nil {
		return nil, st, err
	}
	root, err := ix.JailRoot(dataDir)
	if err != nil {
		st.Notes = append(st.Notes, "낡음을 못 본다 — "+err.Error())
		return ix, st, nil
	}
	stale, err := ix.Stale(root)
	if err != nil {
		st.Notes = append(st.Notes, "낡음을 못 봤다 — "+err.Error())
		return ix, st, nil
	}
	st.Stale = stale
	return ix, st, nil
}
