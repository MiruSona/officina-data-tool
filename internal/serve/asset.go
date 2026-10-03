package serve

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/mirusona/officina-data-tool/internal/assetindex"
)

// maxAssetBytes 를 넘는 파일은 미리보기를 안 한다.
const maxAssetBytes = 50 << 20

// previewTypes 는 브라우저가 여는 형식의 Content-Type 이다. 추측하지 않고 이 표로만 박는다.
var previewTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".bmp":  "image/bmp",
	".wav":  "audio/wav",
	".mp3":  "audio/mpeg",
	".ogg":  "audio/ogg",
}

// openIndex 는 부를 때마다 색인을 다시 읽는다. index 를 다시 돌려도 serve 를 껐다 켤 일이 없다.
func (s *Server) openIndex() (*assetindex.Index, assetindex.Status, error) {
	if s.indexPath == "" {
		return nil, assetindex.Status{Missing: true}, nil
	}
	return assetindex.Open(s.indexPath, s.root)
}

// handleAssetIndex 는 UI 가 칸을 그리고 드롭다운을 채울 색인 요약을 준다.
// 색인이 깨졌어도 200 이다 — UI 가 띠에 그 글을 보인다.
func (s *Server) handleAssetIndex(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	ix, status, err := s.openIndex()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	entries := []map[string]any{}
	if ix != nil {
		thumbs := map[string]bool{}
		if jail, jailErr := ix.JailRoot(s.root); jailErr == nil {
			thumbs = thumbSet(jail)
		}
		for _, e := range ix.Entries {
			entries = append(entries, map[string]any{
				"address":  e.Address,
				"kind":     e.Kind,
				"group":    e.Group,
				"sub":      subNames(e),
				"subKnown": e.Sub != nil,
				"rects":    subRects(e),
				"atlas":    e.IsAtlas(),
				"preview":  previewKind(e),
				"thumb":    thumbs[strings.ToLower(e.GUID)+".png"],
			})
		}
	}
	notes := status.Notes
	if notes == nil {
		notes = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "missing": status.Missing, "stale": status.Stale,
		"file": status.File, "notes": notes, "entries": entries,
	})
}

func subNames(e *assetindex.Entry) []string {
	names := []string{}
	for _, one := range e.Sub {
		names = append(names, one.Name)
	}
	return names
}

// subRects 는 sub 마다 이름·rect·미리보기 종류·src 를 준다. sub.path 는 안 싣는다 — 파일은 주소로만 받는다.
// src 는 그 그림 파일의 guid 다. 같은 원본을 쓰는 타일 여럿을 UI 가 한 번만 받는 데 쓴다 (모르면 빈 값).
func subRects(e *assetindex.Entry) []map[string]any {
	list := []map[string]any{}
	for i := range e.Sub {
		one := &e.Sub[i]
		src := e.GUID
		if one.Path != "" {
			src = one.GUID
		}
		list = append(list, map[string]any{"name": one.Name, "rect": one.Rect, "preview": targetPreview(e, one), "src": strings.ToLower(src)})
	}
	return list
}

// previewKind 는 브라우저가 그 파일을 열 수 있는지로 고른다. 경로를 못 푼 항목은 none 이다.
func previewKind(e *assetindex.Entry) string {
	if e.Path == "" {
		return "none"
	}
	ct, ok := previewTypes[strings.ToLower(path.Ext(e.Path))]
	if !ok {
		return "none"
	}
	if e.Kind == assetindex.KindImage && strings.HasPrefix(ct, "image/") {
		return "image"
	}
	if e.Kind == assetindex.KindAudio && strings.HasPrefix(ct, "audio/") {
		return "audio"
	}
	return "none"
}

// thumbSet 은 Library/AssetTool/thumbs/ 의 파일 이름(소문자)을 한 번에 모은다 (계약 2-3, 2차부터 쓴다).
func thumbSet(jail string) map[string]bool {
	set := map[string]bool{}
	root, err := os.OpenRoot(jail)
	if err != nil {
		return set
	}
	defer root.Close()
	list, err := fs.ReadDir(root.FS(), "Library/AssetTool/thumbs")
	if err != nil {
		return set
	}
	for _, d := range list {
		if d.Type().IsRegular() {
			set[strings.ToLower(d.Name())] = true
		}
	}
	return set
}

// handleAsset 은 주소 하나의 원본 파일을 준다. 막기 일곱은 연동 설계 3-4 차례 그대로다 (1 은 guard).
func (s *Server) handleAsset(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	ix, _, err := s.openIndex()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if ix == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "색인이 없다"})
		return
	}

	// 2. 주소만 받는다. 경로 인자는 없다.
	m := ix.Find(r.URL.Query().Get("address"))
	if len(m.Entries) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "색인에 없는 주소다"})
		return
	}
	// 항목 path 든 아틀라스 sub.path 든 여기서 rel 하나로 모은다. 아래 막기는 rel 에만 건다.
	rel, found := pickTarget(m)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "색인에 없는 하위 에셋이다"})
		return
	}
	if rel == "" {
		writePreviewless(w, rel, "경로를 못 푼 항목이다")
		return
	}

	// 3. 감옥 뿌리는 DataTool 이 정한다.
	jail, err := ix.JailRoot(s.root)
	if err != nil {
		writeForbidden(w, err.Error())
		return
	}
	// 4. Assets/ · Packages/ 아래 정리된 상대경로만.
	if !assetindex.PathOK(rel) {
		writeForbidden(w, fmt.Sprintf("Assets/·Packages/ 아래 경로가 아니다: %q", rel))
		return
	}
	s.sendAsset(w, jail, rel)
}

// target 은 항목(과 하위 이름)이 가리키는 파일이다. 아틀라스 sub 에 path 가 있으면 그 파일이다.
func target(e *assetindex.Entry, sub *assetindex.Sub) string {
	if sub != nil && sub.Path != "" {
		return sub.Path
	}
	return e.Path
}

// targetPreview 는 target 파일의 미리보기 종류다. sub.path 파일은 그림일 때만 image 다.
func targetPreview(e *assetindex.Entry, sub *assetindex.Sub) string {
	if sub != nil && sub.Path != "" {
		if ct, ok := previewTypes[strings.ToLower(path.Ext(sub.Path))]; ok && strings.HasPrefix(ct, "image/") {
			return "image"
		}
		return "none"
	}
	return previewKind(e)
}

// pickTarget 은 같은 주소 여럿 중 열 파일을 고른다 : 미리보기가 되고 경로가 바른 첫 것.
// 없으면 경로가 있는 첫 것을 줘서 뒤의 막기가 403·preview:false 를 정하게 한다. 하위 이름이 어디에도 없으면 found 가 거짓이다.
func pickTarget(m assetindex.Match) (rel string, found bool) {
	type cand struct {
		e   *assetindex.Entry
		sub *assetindex.Sub
	}
	list := []cand{}
	for _, e := range m.Entries {
		var sub *assetindex.Sub
		if m.HasSub && e.Sub != nil {
			if sub = e.FindSub(m.Sub); sub == nil {
				continue
			}
		}
		list = append(list, cand{e, sub})
	}
	if len(list) == 0 {
		return "", false
	}
	for _, c := range list {
		if targetPreview(c.e, c.sub) != "none" && assetindex.PathOK(target(c.e, c.sub)) {
			return target(c.e, c.sub), true
		}
	}
	for _, c := range list {
		if p := target(c.e, c.sub); p != "" {
			return p, true
		}
	}
	return target(list[0].e, list[0].sub), true
}

// sendAsset 은 막기 5~7 이다 : 확장자 표를 먼저 보고, os.Root 로 열고, 파일·크기를 본다.
func (s *Server) sendAsset(w http.ResponseWriter, jail, rel string) {
	ext := strings.ToLower(path.Ext(rel))
	ct, ok := previewTypes[ext]
	if !ok {
		writePreviewless(w, rel, fmt.Sprintf("브라우저가 못 여는 형식(%s)", strings.TrimPrefix(ext, ".")))
		return
	}
	root, err := os.OpenRoot(jail)
	if err != nil {
		writeForbidden(w, "뿌리를 못 열었다")
		return
	}
	defer root.Close()
	// 뿌리 밖 junction·링크는 os.Root 가 막는다 (A6 시험 TestOSRootBlocksJunction 으로 확인).
	file, err := root.Open(rel)
	if errors.Is(err, fs.ErrNotExist) {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "파일이 없다 (색인이 낡았나)"})
		return
	}
	if err != nil {
		writeForbidden(w, "뿌리 밖이거나 못 여는 자리다")
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writePreviewless(w, rel, "파일이 아니다")
		return
	}
	if info.Size() > maxAssetBytes {
		writePreviewless(w, rel, fmt.Sprintf("50MB 를 넘는다 (%d바이트)", info.Size()))
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", fmt.Sprint(info.Size()))
	w.WriteHeader(http.StatusOK)
	io.CopyN(w, file, info.Size()) //nolint:errcheck // 이미 보내기 시작한 뒤라 알릴 곳이 없다
}

func writeForbidden(w http.ResponseWriter, why string) {
	writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": why})
}

func writePreviewless(w http.ResponseWriter, p, reason string) {
	writeJSON(w, http.StatusOK, map[string]any{"preview": false, "path": p, "reason": reason})
}
