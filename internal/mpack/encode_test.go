package mpack

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

// 기대 바이트는 **규격 문서를 보고 손으로 적은 것**이다.
// 인코더로 만든 값을 인코더와 대조하면 아무것도 못 잡는다 (설계 11장 T5).

func TestEncodeSimple(t *testing.T) {
	cases := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"nil", mustEncode(t, nil), []byte{0xc0}},
		{"true", mustEncode(t, true), []byte{0xc3}},
		{"false", mustEncode(t, false), []byte{0xc2}},
	}
	for _, c := range cases {
		if !bytes.Equal(c.got, c.want) {
			t.Errorf("%s: %x 인데 %x 여야 한다", c.name, c.got, c.want)
		}
	}
}

func TestEncodeIntBoundaries(t *testing.T) {
	cases := []struct {
		in   int64
		want []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{127, []byte{0x7f}},
		{128, []byte{0xcc, 0x80}},
		{255, []byte{0xcc, 0xff}},
		{256, []byte{0xcd, 0x01, 0x00}},
		{65535, []byte{0xcd, 0xff, 0xff}},
		{65536, []byte{0xce, 0x00, 0x01, 0x00, 0x00}},
		{4294967295, []byte{0xce, 0xff, 0xff, 0xff, 0xff}}, // 2^32-1
		{4294967296, []byte{0xcf, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}}, // 2^32
		{-1, []byte{0xff}},
		{-32, []byte{0xe0}},
		{-33, []byte{0xd0, 0xdf}},
		{-128, []byte{0xd0, 0x80}},
		{-129, []byte{0xd1, 0xff, 0x7f}},
		{-32768, []byte{0xd1, 0x80, 0x00}},
		{-32769, []byte{0xd2, 0xff, 0xff, 0x7f, 0xff}},
		{-2147483648, []byte{0xd2, 0x80, 0x00, 0x00, 0x00}}, // -2^31
		{-2147483649, []byte{0xd3, 0xff, 0xff, 0xff, 0xff, 0x7f, 0xff, 0xff, 0xff}},   // -2^31-1
		{math.MaxInt64, []byte{0xcf, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}}, // 양수라 uint64 자리가 더 짧거나 같다
		{math.MinInt64, []byte{0xd3, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
	}
	for _, c := range cases {
		var e Encoder
		e.Int(c.in)
		if got := e.Bytes(); !bytes.Equal(got, c.want) {
			t.Errorf("Int(%d): %x 인데 %x 여야 한다", c.in, got, c.want)
		}
		back, err := Decode(c.want)
		if err != nil {
			t.Fatalf("Decode(%x): %v", c.want, err)
		}
		if back != c.in {
			t.Errorf("Decode(%x) = %v 인데 %d 여야 한다", c.want, back, c.in)
		}
	}
}

func TestEncodeStringBoundaries(t *testing.T) {
	cases := []struct {
		n    int
		head []byte
	}{
		{0, []byte{0xa0}},
		{1, []byte{0xa1}},
		{31, []byte{0xbf}},
		{32, []byte{0xd9, 0x20}},
		{255, []byte{0xd9, 0xff}},
		{256, []byte{0xda, 0x01, 0x00}},
		{65535, []byte{0xda, 0xff, 0xff}},
		{65536, []byte{0xdb, 0x00, 0x01, 0x00, 0x00}},
	}
	for _, c := range cases {
		s := strings.Repeat("a", c.n)
		want := append(append([]byte{}, c.head...), s...)
		got := mustEncode(t, s)
		if !bytes.Equal(got, want) {
			t.Errorf("String(길이 %d): 머리가 %x 인데 %x 여야 한다", c.n, got[:len(c.head)], c.head)
		}
		back, err := Decode(got)
		if err != nil || back != s {
			t.Errorf("길이 %d 왕복 실패: %v", c.n, err)
		}
	}
}

// 길이는 글자 수가 아니라 UTF-8 바이트 수다 — 한글에서 가장 쉽게 틀리는 자리다.
func TestEncodeStringUTF8(t *testing.T) {
	cases := []struct {
		in   string
		want []byte
	}{
		{"한", []byte{0xa3, 0xed, 0x95, 0x9c}},
		{"한글", []byte{0xa6, 0xed, 0x95, 0x9c, 0xea, 0xb8, 0x80}},
		{"철검", []byte{0xa6, 0xec, 0xb2, 0xa0, 0xea, 0xb2, 0x80}},
	}
	for _, c := range cases {
		if got := mustEncode(t, c.in); !bytes.Equal(got, c.want) {
			t.Errorf("String(%q): %x 인데 %x 여야 한다", c.in, got, c.want)
		}
	}
	// 글자 11자인데 바이트는 33자 — fixstr 을 넘겨 str8 로 가야 한다.
	long := strings.Repeat("한", 11)
	got := mustEncode(t, long)
	if got[0] != 0xd9 || got[1] != 33 {
		t.Errorf("한글 11자: 머리가 %x 인데 d9 21 이어야 한다", got[:2])
	}
}

func TestEncodeArrayHeader(t *testing.T) {
	cases := []struct {
		n    int
		want []byte
	}{
		{0, []byte{0x90}},
		{15, []byte{0x9f}},
		{16, []byte{0xdc, 0x00, 0x10}},
		{65535, []byte{0xdc, 0xff, 0xff}},
		{65536, []byte{0xdd, 0x00, 0x01, 0x00, 0x00}},
	}
	for _, c := range cases {
		var e Encoder
		e.ArrayHeader(c.n)
		if got := e.Bytes(); !bytes.Equal(got, c.want) {
			t.Errorf("ArrayHeader(%d): %x 인데 %x 여야 한다", c.n, got, c.want)
		}
	}
}

func TestEncodeMapHeader(t *testing.T) {
	cases := []struct {
		n    int
		want []byte
	}{
		{0, []byte{0x80}},
		{15, []byte{0x8f}},
		{16, []byte{0xde, 0x00, 0x10}},
		{65535, []byte{0xde, 0xff, 0xff}},
		{65536, []byte{0xdf, 0x00, 0x01, 0x00, 0x00}},
	}
	for _, c := range cases {
		var e Encoder
		e.MapHeader(c.n)
		if got := e.Bytes(); !bytes.Equal(got, c.want) {
			t.Errorf("MapHeader(%d): %x 인데 %x 여야 한다", c.n, got, c.want)
		}
	}
}

func TestEncodeFloat32(t *testing.T) {
	cases := []struct {
		name string
		in   float32
		want []byte
	}{
		{"0", 0, []byte{0xca, 0x00, 0x00, 0x00, 0x00}},
		{"1.5", 1.5, []byte{0xca, 0x3f, 0xc0, 0x00, 0x00}},
		{"-0.1", -0.1, []byte{0xca, 0xbd, 0xcc, 0xcc, 0xcd}},
		{"NaN", float32(math.NaN()), []byte{0xca, 0x7f, 0xc0, 0x00, 0x00}},
		{"+Inf", float32(math.Inf(1)), []byte{0xca, 0x7f, 0x80, 0x00, 0x00}},
		{"-Inf", float32(math.Inf(-1)), []byte{0xca, 0xff, 0x80, 0x00, 0x00}},
	}
	for _, c := range cases {
		if got := mustEncode(t, c.in); !bytes.Equal(got, c.want) {
			t.Errorf("Float32(%s): %x 인데 %x 여야 한다", c.name, got, c.want)
		}
	}
}

// 굽는 본문 한 판. 설계 7장의 꼴을 줄여 손으로 적었다.
func TestEncodeNestedValue(t *testing.T) {
	v := map[string]any{
		"item": []any{
			[]any{"sword_iron", "철검", int64(12)},
		},
	}
	want := []byte{
		0x81,                     // fixmap 1
		0xa4, 'i', 't', 'e', 'm', // "item"
		0x91,                                                   // fixarray 1
		0x93,                                                   // fixarray 3
		0xaa, 's', 'w', 'o', 'r', 'd', '_', 'i', 'r', 'o', 'n', // "sword_iron"
		0xa6, 0xec, 0xb2, 0xa0, 0xea, 0xb2, 0x80, // "철검"
		0x0c, // 12
	}
	if got := mustEncode(t, v); !bytes.Equal(got, want) {
		t.Fatalf("중첩 값:\n%x\n인데\n%x\n여야 한다", got, want)
	}
}

// 맵 키 차례가 판마다 달라지면 같은 데이터가 다른 파일로 구워진다.
func TestEncodeMapKeyOrderIsStable(t *testing.T) {
	v := map[string]any{"c": int64(3), "a": int64(1), "b": int64(2)}
	want := []byte{0x83, 0xa1, 'a', 0x01, 0xa1, 'b', 0x02, 0xa1, 'c', 0x03}
	for i := 0; i < 20; i++ {
		if got := mustEncode(t, v); !bytes.Equal(got, want) {
			t.Fatalf("%d번째: %x 인데 %x 여야 한다", i, got, want)
		}
	}
}

func TestEncodeRejectsUnsupported(t *testing.T) {
	bad := []any{float64(1.5), uint8(1), []string{"a"}, map[string]int{"a": 1}, struct{}{}}
	for _, v := range bad {
		if _, err := Encode(v); err == nil {
			t.Errorf("%T 를 받아 주면 안 된다", v)
		}
	}
}

func mustEncode(t *testing.T, v any) []byte {
	t.Helper()
	out, err := Encode(v)
	if err != nil {
		t.Fatalf("Encode(%v): %v", v, err)
	}
	return out
}
