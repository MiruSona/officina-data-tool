package schema

import "strings"

// Error 는 스키마가 틀린 자리 하나다.
// Where 는 "tables[1].columns[2]" 처럼 자리를 찍는다 — 스키마 파일은
// 사람이 보게 들여쓴 JSON 이라 줄 번호 대신 자리로 찍는다.
type Error struct {
	Where   string
	Message string
}

func (e *Error) Error() string {
	return e.Where + ": " + e.Message
}

// Errors 는 한 파일에서 모은 스키마 오류 전부다.
// 첫 오류에서 멈추지 않는다 — 한 번에 다 고치게 한다.
type Errors struct {
	File string
	List []*Error
}

func (e *Errors) Error() string {
	lines := make([]string, 0, len(e.List))
	for _, one := range e.List {
		lines = append(lines, e.File+": "+one.Error())
	}
	return strings.Join(lines, "\n")
}

// collector 는 검사 중에 오류를 모은다.
type collector struct {
	file string
	list []*Error
}

func (c *collector) add(where, message string) {
	c.list = append(c.list, &Error{Where: where, Message: message})
}

// err 은 모인 것이 없으면 nil 을 준다. 있으면 *Errors 다.
func (c *collector) err() error {
	if len(c.list) == 0 {
		return nil
	}
	return &Errors{File: c.file, List: c.list}
}
