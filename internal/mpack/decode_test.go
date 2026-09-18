package mpack

import (
	"math"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
)

func TestDecodeSimple(t *testing.T) {
	cases := []struct {
		in   []byte
		want any
	}{
		{[]byte{0xc0}, nil},
		{[]byte{0xc2}, false},
		{[]byte{0xc3}, true},
		{[]byte{0xca, 0x3f, 0xc0, 0x00, 0x00}, float32(1.5)},
		{[]byte{0xa0}, ""},
		{[]byte{0x90}, []any{}},
		{[]byte{0x80}, map[string]any{}},
	}
	for _, c := range cases {
		got, err := Decode(c.in)
		if err != nil {
			t.Fatalf("Decode(%x): %v", c.in, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Decode(%x) = %#v 인데 %#v 여야 한다", c.in, got, c.want)
		}
	}
}

// 규격에 있어도 우리가 안 쓰는 꼴은 오류여야 한다. 조용히 넘어가면 시험이 눈이 먼다.
func TestDecodeRejects(t *testing.T) {
	cases := map[string][]byte{
		"float64":     {0xcb, 0, 0, 0, 0, 0, 0, 0, 0},
		"bin8":        {0xc4, 0x01, 0x00},
		"ext8":        {0xc7, 0x00, 0x00},
		"미정의 0xc1":    {0xc1},
		"잘린 str":      {0xa5, 'a'},
		"잘린 int":      {0xcd, 0x01},
		"모자란 배열":      {0x92, 0x01},
		"키가 정수인 맵":    {0x81, 0x01, 0x01},
		"뒤에 남은 바이트":   {0x01, 0x01},
		"uint64 넘침":   {0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		"UTF-8 아닌 문자": {0xa1, 0xff},
	}
	for name, in := range cases {
		if _, err := Decode(in); err == nil {
			t.Errorf("%s: 오류가 나야 한다", name)
		}
	}
}

// 깨진 파일이 "원소 40억 개" 라고 말해도 그 자리에서 메모리를 삼키면 안 된다.
func TestDecodeHugeHeaderDoesNotAllocate(t *testing.T) {
	if _, err := Decode([]byte{0xdd, 0xff, 0xff, 0xff, 0xff}); err == nil {
		t.Error("모자란 바이트를 오류로 알려야 한다")
	}
	if _, err := Decode([]byte{0xdf, 0xff, 0xff, 0xff, 0xff}); err == nil {
		t.Error("모자란 바이트를 오류로 알려야 한다")
	}
}

// 무작위 값 100개 왕복. 경계값 시험이 못 보는 조합을 훑는다.
func TestRoundTripRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(20260918, 4)) // 씨앗을 박아 둔다 — 깨지면 같은 판을 다시 돌려 본다
	for i := 0; i < 100; i++ {
		want := randomValue(r, 3)
		raw, err := Encode(want)
		if err != nil {
			t.Fatalf("%d번째 Encode: %v", i, err)
		}
		got, err := Decode(raw)
		if err != nil {
			t.Fatalf("%d번째 Decode(%x): %v", i, raw, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%d번째 왕복이 어긋났다:\n쓴 것 %#v\n읽은 것 %#v\n바이트 %x", i, want, got, raw)
		}
	}
}

// randomValue 는 우리가 굽는 일곱 안에서 값을 하나 만든다. depth 가 0 이면 잎만 만든다.
func randomValue(r *rand.Rand, depth int) any {
	kind := r.IntN(7)
	if depth <= 0 && kind >= 5 {
		kind = r.IntN(5)
	}
	switch kind {
	case 0:
		return nil
	case 1:
		return r.IntN(2) == 0
	case 2:
		return randomInt(r)
	case 3:
		// NaN 은 자기 자신과 같지 않아 DeepEqual 이 못 쓴다. 비트열 시험이 따로 본다.
		f := float32(r.Float64()*2000 - 1000)
		return f
	case 4:
		return randomString(r)
	case 5:
		n := r.IntN(6)
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, randomValue(r, depth-1))
		}
		return out
	default:
		n := r.IntN(6)
		out := make(map[string]any, n)
		for i := 0; i < n; i++ {
			out[randomString(r)] = randomValue(r, depth-1)
		}
		return out
	}
}

// randomInt 는 폭 경계 근처에 일부러 몰아서 뽑는다 — 가운데 값은 잘 안 틀린다.
func randomInt(r *rand.Rand) int64 {
	edges := []int64{0, 127, 128, 255, 256, 65535, 65536, 1 << 32, math.MaxInt64,
		-1, -32, -33, -128, -129, -32768, -32769, -1 << 31, math.MinInt64}
	base := edges[r.IntN(len(edges))]
	delta := int64(r.IntN(5) - 2)
	// 넘치면 그냥 경계값을 쓴다.
	if (delta > 0 && base > math.MaxInt64-delta) || (delta < 0 && base < math.MinInt64-delta) {
		return base
	}
	return base + delta
}

func randomString(r *rand.Rand) string {
	n := r.IntN(40)
	if r.IntN(4) == 0 {
		return strings.Repeat("한글", n) // 다바이트 길이 계산을 자주 밟게 한다
	}
	return strings.Repeat("a", n)
}

// 인코더가 고른 폭이 그 값에 **가장 짧은** 폭인지 본다. 폭이 커져도 읽기는 되므로 시험이 없으면 조용히 커진다.
func TestIntWidthIsShortest(t *testing.T) {
	cases := []struct {
		in   int64
		size int
	}{
		{0, 1}, {127, 1}, {-1, 1}, {-32, 1},
		{128, 2}, {255, 2}, {-33, 2}, {-128, 2},
		{256, 3}, {65535, 3}, {-129, 3}, {-32768, 3},
		{65536, 5}, {1<<32 - 1, 5}, {-32769, 5}, {-1 << 31, 5},
		{1 << 32, 9}, {math.MaxInt64, 9}, {-1<<31 - 1, 9}, {math.MinInt64, 9},
	}
	for _, c := range cases {
		var e Encoder
		e.Int(c.in)
		if got := e.Len(); got != c.size {
			t.Errorf("Int(%d): %d바이트인데 %d바이트여야 한다", c.in, got, c.size)
		}
	}
}
