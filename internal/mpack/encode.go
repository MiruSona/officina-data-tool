// Package mpack 은 MessagePack 을 쓰고 되읽는다.
//
// 내보내는 꼴은 일곱뿐이다 — nil · bool · int · float32 · str · array · map.
// 우리가 굽는 값이 이 일곱을 넘지 않아서, 바깥 라이브러리 대신 규격대로 직접 쓴다
// (설계 7-1). 규격 : https://github.com/msgpack/msgpack/blob/master/spec.md
package mpack

import (
	"bytes"
	"fmt"
	"math"
	"sort"
)

// 머리 바이트. 규격의 이름을 그대로 쓴다 — 규격 문서와 대조하기 쉬우라고.
const (
	tagNil     = 0xc0
	tagFalse   = 0xc2
	tagTrue    = 0xc3
	tagFloat32 = 0xca
	tagUint8   = 0xcc
	tagUint16  = 0xcd
	tagUint32  = 0xce
	tagUint64  = 0xcf
	tagInt8    = 0xd0
	tagInt16   = 0xd1
	tagInt32   = 0xd2
	tagInt64   = 0xd3
	tagStr8    = 0xd9
	tagStr16   = 0xda
	tagStr32   = 0xdb
	tagArray16 = 0xdc
	tagArray32 = 0xdd
	tagMap16   = 0xde
	tagMap32   = 0xdf

	prefixFixStr   = 0xa0 // 0xa0..0xbf, 아래 5비트가 길이
	prefixFixArray = 0x90 // 0x90..0x9f, 아래 4비트가 개수
	prefixFixMap   = 0x80 // 0x80..0x8f, 아래 4비트가 개수

	maxFixStr    = 31
	maxFixArray  = 15
	maxFixMap    = 15
	maxPosFixInt = 127
	minNegFixInt = -32
)

// Encoder 는 값을 차례로 이어 붙인다.
//
// 굽는 쪽(bake)은 행을 한 줄씩 흘려 보내므로, 값 하나를 통째로 만드는
// Encode 보다 이 낮은 층이 편하다. 제로값이 바로 쓸 수 있는 빈 통이다.
type Encoder struct {
	buf bytes.Buffer
}

// Bytes 는 지금까지 쓴 바이트다. 안쪽 버퍼를 그대로 주므로 다음 쓰기가 덮을 수 있다.
func (e *Encoder) Bytes() []byte { return e.buf.Bytes() }

// Len 은 지금까지 쓴 바이트 수다. 굽기 머리의 본문 길이를 채울 때 쓴다.
func (e *Encoder) Len() int { return e.buf.Len() }

// Reset 은 통을 비운다.
func (e *Encoder) Reset() { e.buf.Reset() }

// Nil 은 nil 하나를 쓴다.
func (e *Encoder) Nil() { e.buf.WriteByte(tagNil) }

// Bool 은 참·거짓 하나를 쓴다.
func (e *Encoder) Bool(b bool) {
	if b {
		e.buf.WriteByte(tagTrue)
		return
	}
	e.buf.WriteByte(tagFalse)
}

// Int 는 정수 하나를 **가장 짧은 폭**으로 쓴다.
//
// 폭을 줄이는 것이 파일 크기의 대부분이다 — 행마다 작은 수가 수십 개 들어간다.
// 읽는 쪽(MessagePack-CSharp)은 어느 폭이든 같은 정수로 읽으므로 안전하다.
func (e *Encoder) Int(i int64) {
	switch {
	case i >= 0 && i <= maxPosFixInt:
		e.buf.WriteByte(byte(i))
	case i < 0 && i >= minNegFixInt:
		// negative fixint 은 0xe0..0xff 다. 2의 보수 하위 8비트가 그대로 그 자리다.
		e.buf.WriteByte(byte(i))
	case i >= 0 && i <= math.MaxUint8:
		e.buf.WriteByte(tagUint8)
		e.buf.WriteByte(byte(i))
	case i >= math.MinInt8 && i < 0:
		e.buf.WriteByte(tagInt8)
		e.buf.WriteByte(byte(i))
	case i >= 0 && i <= math.MaxUint16:
		e.buf.WriteByte(tagUint16)
		e.writeUint16(uint16(i))
	case i >= math.MinInt16 && i < 0:
		e.buf.WriteByte(tagInt16)
		e.writeUint16(uint16(i))
	case i >= 0 && i <= math.MaxUint32:
		e.buf.WriteByte(tagUint32)
		e.writeUint32(uint32(i))
	case i >= math.MinInt32 && i < 0:
		e.buf.WriteByte(tagInt32)
		e.writeUint32(uint32(i))
	case i >= 0:
		e.buf.WriteByte(tagUint64)
		e.writeUint64(uint64(i))
	default:
		e.buf.WriteByte(tagInt64)
		e.writeUint64(uint64(i))
	}
}

// Float32 는 float32 하나를 쓴다.
//
// float64 는 쓰지 않는다 — 게임 수치에 배정밀도가 필요한 자리가 없고,
// 폭이 두 배가 되면 파일만 커진다 (설계 7장).
func (e *Encoder) Float32(f float32) {
	e.buf.WriteByte(tagFloat32)
	e.writeUint32(math.Float32bits(f))
}

// String 은 문자열 하나를 쓴다. 길이는 글자 수가 아니라 **UTF-8 바이트 수**다.
func (e *Encoder) String(s string) {
	n := len(s)
	switch {
	case n <= maxFixStr:
		e.buf.WriteByte(prefixFixStr | byte(n))
	case n <= math.MaxUint8:
		e.buf.WriteByte(tagStr8)
		e.buf.WriteByte(byte(n))
	case n <= math.MaxUint16:
		e.buf.WriteByte(tagStr16)
		e.writeUint16(uint16(n))
	default:
		e.buf.WriteByte(tagStr32)
		e.writeUint32(uint32(n))
	}
	e.buf.WriteString(s)
}

// ArrayHeader 는 원소 n 개짜리 배열의 머리를 쓴다. 뒤이어 값 n 개를 쓰는 것은 부르는 쪽 몫이다.
func (e *Encoder) ArrayHeader(n int) {
	switch {
	case n <= maxFixArray:
		e.buf.WriteByte(prefixFixArray | byte(n))
	case n <= math.MaxUint16:
		e.buf.WriteByte(tagArray16)
		e.writeUint16(uint16(n))
	default:
		e.buf.WriteByte(tagArray32)
		e.writeUint32(uint32(n))
	}
}

// MapHeader 는 짝 n 개짜리 맵의 머리를 쓴다. 뒤이어 키·값을 2n 개 쓴다.
func (e *Encoder) MapHeader(n int) {
	switch {
	case n <= maxFixMap:
		e.buf.WriteByte(prefixFixMap | byte(n))
	case n <= math.MaxUint16:
		e.buf.WriteByte(tagMap16)
		e.writeUint16(uint16(n))
	default:
		e.buf.WriteByte(tagMap32)
		e.writeUint32(uint32(n))
	}
}

// 길이·수치는 전부 빅엔디안이다 (규격). 굽기 머리의 리틀엔디안과 헷갈리지 않게 한 자리에 모았다.
func (e *Encoder) writeUint16(v uint16) {
	e.buf.Write([]byte{byte(v >> 8), byte(v)})
}

func (e *Encoder) writeUint32(v uint32) {
	e.buf.Write([]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

func (e *Encoder) writeUint64(v uint64) {
	e.buf.Write([]byte{
		byte(v >> 56), byte(v >> 48), byte(v >> 40), byte(v >> 32),
		byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v),
	})
}

// Value 는 Go 값 하나를 알맞은 꼴로 쓴다. 우리가 굽는 일곱 밖이면 오류다.
//
// 맵의 키 차례는 **이름순으로 고정**한다. Go 맵을 도는 차례는 판마다 달라서,
// 그냥 돌면 같은 데이터를 두 번 구웠을 때 파일 바이트가 달라진다.
func (e *Encoder) Value(v any) error {
	switch t := v.(type) {
	case nil:
		e.Nil()
	case bool:
		e.Bool(t)
	case int:
		e.Int(int64(t))
	case int64:
		e.Int(t)
	case float32:
		e.Float32(t)
	case string:
		e.String(t)
	case []any:
		e.ArrayHeader(len(t))
		for _, one := range t {
			if err := e.Value(one); err != nil {
				return err
			}
		}
	case map[string]any:
		e.MapHeader(len(t))
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			e.String(k)
			if err := e.Value(t[k]); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("mpack: 굽지 못하는 타입이다: %T", v)
	}
	return nil
}

// Encode 는 값 하나를 통째로 바이트로 만든다.
//
// float64 를 막는 것이 이 함수의 숨은 일이다 — JSON 에서 읽은 수는 float64 라서
// 그대로 넘기기 쉬운데, 소리 없이 폭이 어긋나면 C# 쪽에서야 드러난다.
func Encode(v any) ([]byte, error) {
	var e Encoder
	if err := e.Value(v); err != nil {
		return nil, err
	}
	out := make([]byte, e.buf.Len())
	copy(out, e.buf.Bytes())
	return out, nil
}
