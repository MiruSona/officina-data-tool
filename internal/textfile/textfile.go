// Package textfile 은 읽기 입구 셋이 같이 쓰는 파일 읽기다.
//
// 데이터 표(table)·스키마(schema)·설정(.datatool.json) 셋 다 사람이 편집기로 고치는
// 텍스트 파일이라 **같은 전처리를 거쳐야 한다.** 한 군데만 BOM 을 걷으면
// 「설정은 읽히는데 표는 안 읽히는」 어긋남이 난다.
//
// 지금 하는 일은 BOM 걷기 하나뿐이다. 줄바꿈 정책(CRLF → LF)이 붙는다면 여기다.
package textfile

import "os"

// bom 은 UTF-8 바이트 차례 표시다. Windows 편집기·PowerShell 이 붙여서 저장한다.
var bom = []byte{0xEF, 0xBB, 0xBF}

// ReadFile 은 텍스트 파일을 읽고 BOM 을 걷어 준다.
func ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return TrimBOM(data), nil
}

// TrimBOM 은 앞머리 BOM 을 걷는다. 이미 읽어 온 바이트를 다룰 때 쓴다.
func TrimBOM(data []byte) []byte {
	if len(data) >= len(bom) &&
		data[0] == bom[0] && data[1] == bom[1] && data[2] == bom[2] {
		return data[len(bom):]
	}
	return data
}
