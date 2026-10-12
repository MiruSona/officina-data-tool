// Package textfile 은 읽기 입구 셋이 같이 쓰는 파일 읽기다.
//
// 데이터 표(table)·스키마(schema)·설정(.datatool.json) 셋 다 사람이 편집기로 고치는
// 텍스트 파일이라 **같은 전처리를 거쳐야 한다.** 한 군데만 BOM 을 걷으면
// 「설정은 읽히는데 표는 안 읽히는」 어긋남이 난다.
//
// 지금 하는 일은 BOM 걷기 하나뿐이다. 줄바꿈 정책(CRLF → LF)이 붙는다면 여기다.
package textfile

import (
	"bytes"
	"os"
)

// BOM 은 UTF-8 바이트 차례 표시다. Windows 편집기·PowerShell 이 붙여서 저장한다.
// 시험이 BOM 붙은 파일을 만들 때도 이 값을 쓴다 — 바이트를 두 군데 적지 않게.
var BOM = []byte{0xEF, 0xBB, 0xBF}

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
	return bytes.TrimPrefix(data, BOM)
}

// SameContent 는 새로 만든 바이트(out)가 디스크 바이트(raw)와 「같은 내용」 인가 본다 — 같으면 안 쓴다.
//
// BOM 은 걷고 견준다. Format 은 BOM 을 안 내고 읽기도 BOM 을 걷으니, BOM 만 다른 파일은 내용이 같다.
// 줄끝(CRLF)은 걷지 않는다. 읽기가 CR 을 안 걷으므로 CRLF 파일은 다르다고 보고 LF 로 다시 쓴다.
// 이 규칙은 저장(serve)용이다. BOM·CRLF 를 고치는 게 일인 fmt 는 따로 견준다.
func SameContent(out, raw []byte) bool {
	return bytes.Equal(out, TrimBOM(raw))
}
