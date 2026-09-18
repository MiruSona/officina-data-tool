package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mirusona/officina-data-tool/internal/textfile"
)

// configFileName 은 데이터 폴더에 놓이는 작은 설정이다 (설계 2장·5장).
// init 이 만들고, 사람이 손으로 고친다.
const configFileName = ".datatool.json"

// config 는 .datatool.json 의 내용이다. 칸 둘뿐이고 둘 다 내보낼 자리다.
// 모르는 칸은 조용히 넘기지 않고 즉시 실패한다 — 오타가 기본값으로 둔갑하면 안 된다.
type config struct {
	// Gen 은 gen 이 C# 을 쓸 폴더다. 데이터 폴더 기준 상대경로로 적는다.
	Gen string `json:"gen"`
	// Export 는 export 가 gamedata.bytes 를 구울 자리다.
	Export string `json:"export"`
}

// dataRoot 는 명령 하나가 쓸 데이터 폴더와 거기서 읽은 설정이다.
// cfgPath 가 "" 면 설정 파일이 없었다는 뜻이다 (그래도 폴더는 쓴다).
type dataRoot struct {
	dir     string
	cfg     config
	cfgPath string
}

// resolveDataRoot 는 명령이 쓸 데이터 폴더를 정한다 (설계 5장).
//
//	--data 를 주면 그 폴더다. 설정은 있으면 읽고 없으면 넘어간다.
//	안 주면 지금 폴더에서 위로 올라가며 .datatool.json 을 찾는다.
//
// 위로 올라가며 볼 때 각 칸에서 그 폴더 자신과 그 아래 GameData/ 둘을 본다 —
// 설정이 GameData/ 안에 살기 때문에(설계 2장) 게임 뿌리에서 부른 명령도 찾게 하려는 것이다.
// 못 찾으면 종료 1 과 안내다. 엉뚱한 폴더를 데이터 폴더로 지어내지 않는다.
func resolveDataRoot(opts options) (dataRoot, int, error) {
	if opts.data != "" {
		cfg, path, err := readConfigIn(opts.data)
		if err != nil {
			return dataRoot{}, exitReadFail, err
		}
		return dataRoot{dir: opts.data, cfg: cfg, cfgPath: path}, exitOK, nil
	}

	start, err := os.Getwd()
	if err != nil {
		return dataRoot{}, exitReadFail, fmt.Errorf("지금 폴더를 못 알아냈다 : %v", err)
	}
	for dir := start; ; {
		for _, candidate := range []string{dir, filepath.Join(dir, defaultDataDir)} {
			cfg, path, err := readConfigIn(candidate)
			if err != nil {
				return dataRoot{}, exitReadFail, err
			}
			if path != "" {
				return dataRoot{dir: candidate, cfg: cfg, cfgPath: path}, exitOK, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return dataRoot{}, exitUsage, fmt.Errorf(
		"%s 를 못 찾았다 (%s 에서 위로 다 봤다).\n"+
			"데이터 폴더를 --data 로 알려주거나, 그 폴더에서 datatool init 을 한 번 돌린다.",
		configFileName, filepath.ToSlash(start))
}

// readConfigIn 은 폴더 하나에서 설정을 읽는다.
// 파일이 없으면 오류가 아니다 — 찾는 중이기 때문이다. 있는데 깨졌으면 바로 실패한다.
func readConfigIn(dir string) (config, string, error) {
	// BOM 걷기는 읽기 입구 셋이 같은 함수를 쓴다 (internal/textfile).
	path := filepath.Join(dir, configFileName)
	raw, err := textfile.ReadFile(path)
	if err != nil {
		return config{}, "", nil
	}

	var cfg config
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return config{}, "", fmt.Errorf("%s 를 못 읽었다 : %v", filepath.ToSlash(path), err)
	}
	return cfg, path, nil
}

// outPath 는 내보낼 자리를 정한다. 차례는 하나뿐이다 —
// **명령 옵션 > .datatool.json > 붙박이 기본값.** 옵션을 주면 설정이 그것을 못 뒤집는다.
//
// 설정의 자리는 데이터 폴더 기준 상대경로로 본다. 절대경로·UNC·다른 드라이브면
// 그대로 쓰되 **무엇을 쓸지 stderr 에 한 줄 찍는다** — 엉뚱한 곳에 조용히 쓰는 것이 제일 나쁘다.
// 부모 밖으로 나가는 ../ 는 막지 않는다. 게임의 Assets/ 가 GameData/ 의 형제라 그게 정상 경로다.
func (r dataRoot) outPath(fromFlag, fromConfig, builtin string) string {
	if fromFlag != "" {
		return fromFlag
	}
	if fromConfig == "" {
		return filepath.Join(r.dir, builtin)
	}
	if filepath.IsAbs(fromConfig) || filepath.VolumeName(fromConfig) != "" {
		fmt.Fprintf(os.Stderr, "%s 가 가리키는 절대 경로에 쓴다 : %s\n",
			filepath.ToSlash(r.cfgPath), filepath.ToSlash(fromConfig))
		return fromConfig
	}
	return filepath.Join(r.dir, fromConfig)
}
