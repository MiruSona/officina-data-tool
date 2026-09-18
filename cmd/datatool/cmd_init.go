package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultDataDir = "GameData"

// initFile 은 init 이 만드는 파일 한 장이다.
type initFile struct {
	name    string
	content string
}

// cmdInit 은 데이터 폴더에 예제 한 벌을 만든다.
//
// **한 장이라도 이미 있으면 아무것도 안 쓰고 멈춘다.** 덮어쓰기로 남의 데이터를
// 날리는 것보다 사람이 한 번 더 치는 쪽이 낫다.
func cmdInit(opts options, rest []string) int {
	if len(rest) > 0 {
		return fail(opts, exitUsage, fmt.Sprintf("init 은 인자를 안 받는다: %s", strings.Join(rest, " ")))
	}

	dir := opts.data
	if dir == "" {
		dir = defaultDataDir
	}
	files := initFiles()

	if existing := existingNames(dir, files); len(existing) > 0 {
		return fail(opts, exitUsage,
			fmt.Sprintf("%s 에 이미 있다: %s (init 은 덮어쓰지 않는다)", dir, strings.Join(existing, ", ")))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail(opts, exitWriteFail, fmt.Sprintf("폴더를 못 만들었다: %v", err))
	}

	created := []string{}
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, []byte(f.content), 0o644); err != nil {
			return fail(opts, exitWriteFail, fmt.Sprintf("%s 를 못 썼다: %v", path, err))
		}
		created = append(created, filepath.ToSlash(path))
	}
	return reportInit(opts, created)
}

func existingNames(dir string, files []initFile) []string {
	existing := []string{}
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(dir, f.name)); err == nil {
			existing = append(existing, f.name)
		}
	}
	return existing
}

func reportInit(opts options, created []string) int {
	if opts.json {
		printJSON(map[string]any{
			"ok":      true,
			"exit":    exitOK,
			"created": created,
		})
		return exitOK
	}
	fmt.Println("만들었다 :")
	for _, path := range created {
		fmt.Println("  " + path)
	}
	fmt.Println("\nschema.json 의 namespace 를 게임 것으로 바꾸고 표를 더해 나가면 된다.")
	return exitOK
}

func initFiles() []initFile {
	return []initFile{
		{name: "schema.json", content: exampleSchema},
		{name: ".datatool.json", content: exampleConfig},
		{name: "item.json", content: exampleItem},
		{name: "drop.json", content: exampleDrop},
	}
}

const exampleSchema = `{
  "version": 1,
  "namespace": "MyGame.Data",
  "enums": {
    "Grade": ["common", "rare", "epic"]
  },
  "tables": [
    { "name": "item", "columns": [
      { "name": "id",    "type": "string", "desc": "표 안에서 유일한 열쇠" },
      { "name": "name",  "type": "string", "loc": true },
      { "name": "atk",   "type": "int",    "min": 0, "max": 9999, "default": 0 },
      { "name": "price", "type": "float",  "min": 0, "default": 0 },
      { "name": "grade", "type": "enum",   "enum": "Grade", "default": "common" },
      { "name": "tags",  "type": "list<string>", "default": [] } ] },
    { "name": "drop", "columns": [
      { "name": "id",      "type": "string" },
      { "name": "item_id", "type": "ref",   "ref": "item" },
      { "name": "rate",    "type": "float", "min": 0, "max": 1 } ] }
  ]
}
`

const exampleConfig = `{
  "gen": "../Assets/_Project/Scripts/Data/Generated",
  "export": "../Assets/StreamingAssets/gamedata.bytes"
}
`

// 행 하나가 한 줄이다. 기본값과 같은 열은 뺀다 (설계 3장).
const exampleItem = `[
{"id":"sword_iron","name":"철검","atk":12,"price":300,"tags":["weapon","melee"]},
{"id":"potion_hp","name":"체력 물약","price":50,"tags":["consume"]}
]
`

const exampleDrop = `[
{"id":"drop_slime_sword","item_id":"sword_iron","rate":0.05},
{"id":"drop_slime_potion","item_id":"potion_hp","rate":0.5}
]
`
