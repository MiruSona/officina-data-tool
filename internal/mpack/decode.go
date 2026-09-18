package mpack

import (
	"fmt"
	"math"
	"unicode/utf8"
)

// Decode 는 시험용 되읽기다.
//
// 게임은 C# 이 읽으므로 Go 는 되읽을 일이 없다. 그런데 손으로 쓴 직렬화기는
// **조용히 틀린다** — 그래서 왕복 시험을 세우려고 짝을 만들어 둔다 (설계 7-1·11장 T5).
// 그래서 관대하지 않다. 우리가 쓰지 않는 꼴은 전부 오류이고, 남는 바이트도 오류다.
//
// 돌려주는 Go 타입은 nil · bool · int64 · float32 · string · []any · map[string]any 다.
func Decode(data []byte) (any, error) {
	d := &decoder{data: data}
	v, err := d.value()
	if err != nil {
		return nil, err
	}
	if d.pos != len(d.data) {
		return nil, fmt.Errorf("mpack: 값 뒤에 바이트가 %d개 남았다", len(d.data)-d.pos)
	}
	return v, nil
}

type decoder struct {
	data []byte
	pos  int
}

// take 는 n 바이트를 떼어 준다. 모자라면 오류다 — 잘린 파일을 반쯤 읽지 않는다.
func (d *decoder) take(n int) ([]byte, error) {
	if n < 0 || d.pos+n > len(d.data) {
		return nil, fmt.Errorf("mpack: %d번째에서 %d바이트가 모자란다", d.pos, n)
	}
	out := d.data[d.pos : d.pos+n]
	d.pos += n
	return out, nil
}

func (d *decoder) uint16() (int, error) {
	b, err := d.take(2)
	if err != nil {
		return 0, err
	}
	return int(b[0])<<8 | int(b[1]), nil
}

func (d *decoder) uint32() (uint32, error) {
	b, err := d.take(4)
	if err != nil {
		return 0, err
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]), nil
}

func (d *decoder) uint64() (uint64, error) {
	b, err := d.take(8)
	if err != nil {
		return 0, err
	}
	var v uint64
	for _, one := range b {
		v = v<<8 | uint64(one)
	}
	return v, nil
}

func (d *decoder) value() (any, error) {
	head, err := d.take(1)
	if err != nil {
		return nil, err
	}
	tag := head[0]

	switch {
	case tag <= maxPosFixInt: // 0x00..0x7f
		return int64(tag), nil
	case tag >= 0xe0: // negative fixint
		return int64(int8(tag)), nil
	case tag&0xf0 == prefixFixMap: // 0x80..0x8f
		return d.mapBody(int(tag & 0x0f))
	case tag&0xf0 == prefixFixArray: // 0x90..0x9f
		return d.arrayBody(int(tag & 0x0f))
	case tag&0xe0 == prefixFixStr: // 0xa0..0xbf
		return d.stringBody(int(tag & 0x1f))
	}

	switch tag {
	case tagNil:
		return nil, nil
	case tagFalse:
		return false, nil
	case tagTrue:
		return true, nil
	case tagFloat32:
		bits, err := d.uint32()
		if err != nil {
			return nil, err
		}
		return math.Float32frombits(bits), nil
	case tagUint8:
		b, err := d.take(1)
		if err != nil {
			return nil, err
		}
		return int64(b[0]), nil
	case tagUint16:
		n, err := d.uint16()
		if err != nil {
			return nil, err
		}
		return int64(n), nil
	case tagUint32:
		v, err := d.uint32()
		if err != nil {
			return nil, err
		}
		return int64(v), nil
	case tagUint64:
		v, err := d.uint64()
		if err != nil {
			return nil, err
		}
		if v > math.MaxInt64 {
			return nil, fmt.Errorf("mpack: uint64 %d 는 int64 에 안 들어간다", v)
		}
		return int64(v), nil
	case tagInt8:
		b, err := d.take(1)
		if err != nil {
			return nil, err
		}
		return int64(int8(b[0])), nil
	case tagInt16:
		n, err := d.uint16()
		if err != nil {
			return nil, err
		}
		return int64(int16(n)), nil
	case tagInt32:
		v, err := d.uint32()
		if err != nil {
			return nil, err
		}
		return int64(int32(v)), nil
	case tagInt64:
		v, err := d.uint64()
		if err != nil {
			return nil, err
		}
		return int64(v), nil
	case tagStr8:
		b, err := d.take(1)
		if err != nil {
			return nil, err
		}
		return d.stringBody(int(b[0]))
	case tagStr16:
		n, err := d.uint16()
		if err != nil {
			return nil, err
		}
		return d.stringBody(n)
	case tagStr32:
		v, err := d.uint32()
		if err != nil {
			return nil, err
		}
		return d.stringBody(int(v))
	case tagArray16:
		n, err := d.uint16()
		if err != nil {
			return nil, err
		}
		return d.arrayBody(n)
	case tagArray32:
		v, err := d.uint32()
		if err != nil {
			return nil, err
		}
		return d.arrayBody(int(v))
	case tagMap16:
		n, err := d.uint16()
		if err != nil {
			return nil, err
		}
		return d.mapBody(n)
	case tagMap32:
		v, err := d.uint32()
		if err != nil {
			return nil, err
		}
		return d.mapBody(int(v))
	}
	return nil, fmt.Errorf("mpack: %d번째의 0x%02x 는 우리가 쓰지 않는 꼴이다", d.pos-1, tag)
}

func (d *decoder) stringBody(n int) (string, error) {
	b, err := d.take(n)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		return "", fmt.Errorf("mpack: %d번째 문자열이 UTF-8 이 아니다", d.pos-n)
	}
	return string(b), nil
}

func (d *decoder) arrayBody(n int) ([]any, error) {
	out := make([]any, 0, growCap(n))
	for i := 0; i < n; i++ {
		v, err := d.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// mapBody 는 키가 문자열인 맵만 받는다. 우리는 그것만 굽는다 (설계 7장).
func (d *decoder) mapBody(n int) (map[string]any, error) {
	out := make(map[string]any, growCap(n))
	for i := 0; i < n; i++ {
		k, err := d.value()
		if err != nil {
			return nil, err
		}
		key, ok := k.(string)
		if !ok {
			return nil, fmt.Errorf("mpack: 맵 키가 문자열이 아니다: %T", k)
		}
		v, err := d.value()
		if err != nil {
			return nil, err
		}
		out[key] = v
	}
	return out, nil
}

// growCap 은 머리에 적힌 개수를 그대로 믿고 크게 잡지 않게 막는다.
// 깨진 파일이 "원소 40억 개" 라고 말해도 메모리를 한 번에 삼키면 안 된다.
func growCap(n int) int {
	const limit = 4096
	if n > limit {
		return limit
	}
	if n < 0 {
		return 0
	}
	return n
}
