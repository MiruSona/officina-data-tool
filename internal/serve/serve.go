// Package serve 는 표 편집 UI 를 띄우는 로컬 서버다 (설계 9장).
//
// 붙는 곳은 127.0.0.1 뿐이고, 프로세스가 사는 동안만 산다 — 상주하지 않는다.
// 파일을 쓰는 곳은 PUT 하나뿐이며, 쓰기 규칙은 자기가 안 만들고 table 묶음에 맡긴다.
// UI 가 JSON 파일을 만들면 fmt 로 정리한 파일과 diff 가 갈린다 (설계 9장).
package serve

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mirusona/officina-data-tool/ui"
)

// maxBody 는 요청 본문 상한이다. 2,000행 표를 통째로 받아도 남는 크기이면서
// 로컬 프로그램이 메모리를 밀어 넣는 것을 막는다.
const maxBody = 32 << 20

// tokenHeader 는 UI 가 토큰을 실어 보내는 머리다. 주소창에 남는 ?t= 대신 이것을 쓴다.
const tokenHeader = "X-Datatool-Token"

// Server 는 데이터 폴더 하나를 보는 서버다.
type Server struct {
	// 심볼릭 링크까지 푼 데이터 폴더 절대경로. 경로 감옥의 뿌리는 이것 하나뿐이다.
	root string
	// 뜰 때 한 번 뽑는다. 같은 기계의 딴 프로그램이 API 를 부르는 것을 막는다 (설계 9장).
	token string
	// 쓰기는 한 번에 하나만. 두 탭이 같이 저장하면 나중 것이 앞 것을 조용히 덮는다.
	mu sync.Mutex
	// AssetTool 색인 자리(절대경로). 비면 색인이 없는 것으로 본다.
	indexPath string
}

// New 는 데이터 폴더를 잡고 서버를 만든다. 폴더가 없으면 여기서 실패한다.
// indexPath 는 AssetTool 색인 자리다. 파일은 부를 때마다 다시 읽는다.
func New(dir, indexPath string) (*Server, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	// 뿌리는 여기서 한 번만 정규화한다. 파일마다 링크를 푸는 것은 비싸다.
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s: 폴더가 아니다", dir)
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	if indexPath != "" {
		if indexPath, err = filepath.Abs(indexPath); err != nil {
			return nil, err
		}
	}
	return &Server{root: root, token: token, indexPath: indexPath}, nil
}

// Root 는 서버가 보는 데이터 폴더다.
func (s *Server) Root() string { return s.root }

// Token 은 API 를 부를 때 필요한 토큰이다.
func (s *Server) Token() string { return s.token }

// Listen 은 127.0.0.1 의 포트를 잡는다. port 가 0 이면 빈 포트를 받는다.
//
// 띄우기(Serve)와 나눠 둔 것은, 주소를 사람에게 알린 다음에 막혀야 하기 때문이다.
func (s *Server) Listen(port int) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
}

// Serve 는 잡아 둔 포트에서 요청을 받는다. 서버가 멈출 때까지 안 돌아온다.
func (s *Server) Serve(ln net.Listener) error {
	return (&http.Server{Handler: s.Handler()}).Serve(ln)
}

// Handler 는 라우팅과 문지기를 얹은 처리기다. 시험은 이것을 httptest 에 얹는다.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/schema", s.handleSchema)
	mux.HandleFunc("/api/tables", s.handleTables)
	mux.HandleFunc("/api/table/", s.handleTable)
	mux.HandleFunc("/api/validate", s.handleValidate)
	mux.HandleFunc("/api/assetindex", s.handleAssetIndex)
	mux.HandleFunc("/api/asset", s.handleAsset)
	mux.Handle("/", http.FileServer(http.FS(ui.FS)))
	return s.guard(mux)
}

// guard 는 API 로 가는 요청을 세 가지로 거른다.
//
//   - Host 가 127.0.0.1·localhost 인가 — 브라우저가 딴 이름으로 이 포트에 오는 것을 막는다
//   - 토큰이 맞나 — 같은 기계의 딴 프로그램을 막는다
//   - 본문이 상한 안인가
//
// CORS 머리는 하나도 안 붙인다. 같은 출처(우리 UI)만 부르면 된다.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !localHost(r.Host) {
			http.Error(w, "이 서버는 127.0.0.1 로만 부른다", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if !s.checkToken(r) {
				writeJSON(w, http.StatusUnauthorized, map[string]any{
					"ok": false, "error": "토큰이 없거나 틀렸다",
				})
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) checkToken(r *http.Request) bool {
	got := r.Header.Get(tokenHeader)
	if got == "" {
		got = r.URL.Query().Get("t")
	}
	// 길이로 새어 나가는 것까지 막는다 — 토큰은 짧고 요청은 얼마든지 반복된다.
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

// localHost 는 Host 머리가 우리 주소인지 본다. 포트는 안 본다 (빈 포트를 받으므로).
func localHost(host string) bool {
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	return name == "127.0.0.1" || name == "localhost" || name == "[::1]" || name == "::1"
}

func newToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// URL 은 브라우저에 줄 주소다. 토큰을 한 번 실어 보내고, UI 는 그것을 머리로 바꿔 쓴다.
func (s *Server) URL(ln net.Listener) string {
	return fmt.Sprintf("http://127.0.0.1:%d/?t=%s", ln.Addr().(*net.TCPAddr).Port, s.token)
}
