package table

import (
	"fmt"
	"os"
	"path/filepath"
)

// Pending 은 쓸 파일 한 장이다.
type Pending struct {
	Path string
	Data []byte
}

// WriteAllError 는 여러 장을 쓰다 멈춘 자리다. 무엇을 썼고 무엇을 못 썼는지 들고 있다.
type WriteAllError struct {
	Written    []string
	NotWritten []string
	Err        error
}

func (e *WriteAllError) Error() string {
	return fmt.Sprintf("%d 장 중 %d 장을 쓰고 멈췄다: %v", len(e.Written)+len(e.NotWritten), len(e.Written), e.Err)
}

// WriteAll 은 여러 파일을 **먼저 tmp 로 다 쓴 뒤 rename 을 잇는다** (스키마·enum 편집 설계 4장).
//
// tmp 단계에서 실패하면 tmp 를 다 지우고 아무것도 안 바꾼다. rename 은 주어진 차례대로 한다 —
// 부르는 쪽이 표 먼저, schema.json 을 마지막에 둔다. rename 중에 실패하면 어디까지 썼나를 돌려준다.
// rename 전에 옛 파일을 지우지 않는다 — Go 의 os.Rename 은 윈도에서도 덮어쓰고,
// 먼저 지웠다가 rename 이 실패하면 옛 파일까지 잃는다.
func WriteAll(files []Pending) ([]string, error) {
	tmps := make([]string, 0, len(files))
	cleanup := func(from int) {
		for _, name := range tmps[from:] {
			os.Remove(name)
		}
	}

	for _, f := range files {
		name, err := writeTemp(f.Path, f.Data)
		if err != nil {
			cleanup(0)
			return nil, &WriteAllError{NotWritten: paths(files), Err: err}
		}
		tmps = append(tmps, name)
	}

	written := []string{}
	for i, f := range files {
		if err := os.Rename(tmps[i], f.Path); err != nil {
			cleanup(i)
			return written, &WriteAllError{Written: written, NotWritten: paths(files[i:]), Err: err}
		}
		written = append(written, f.Path)
	}
	return written, nil
}

// writeTemp 는 path 옆에 tmp 를 쓰고 그 이름을 준다. 실패하면 tmp 를 남기지 않는다.
func writeTemp(path string, data []byte) (string, error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

func paths(files []Pending) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}
