package test_utils

import (
	"context"
	"fmt"
	config2 "github.com/go-git/go-git/v5/config"
	"github.com/kluctl/kluctl/v2/e2e/test-utils/http-server"
	port_tool "github.com/kluctl/kluctl/v2/e2e/test-utils/port-tool"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sigs.k8s.io/yaml"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/jinzhu/copier"
)

type TestGitServer struct {
	t *testing.T

	baseDir string

	gitServer     *http_server.Server
	gitHttpServer *http.Server
	gitServerPort int

	authUsername string
	authPassword string
	failWhenAuth bool

	cleanupMutex  sync.RWMutex
	cleanupDoneCh chan struct{}
}

type TestGitServerOpt func(*TestGitServer)

func WithTestGitServerAuth(username string, password string) TestGitServerOpt {
	return func(server *TestGitServer) {
		server.authUsername = username
		server.authPassword = password
	}
}

func WithTestGitServerFailWhenAuth(fail bool) TestGitServerOpt {
	return func(server *TestGitServer) {
		server.failWhenAuth = fail
	}
}

func NewTestGitServer(t *testing.T, opts ...TestGitServerOpt) *TestGitServer {
	p := &TestGitServer{
		t:             t,
		baseDir:       t.TempDir(),
		cleanupDoneCh: make(chan struct{}),
	}

	for _, o := range opts {
		o(p)
	}

	p.initGitServer()

	t.Cleanup(func() {
		p.Cleanup()
	})

	return p
}

func (p *TestGitServer) initGitServer() {
	p.gitServer = http_server.New(p.baseDir)

	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		p.cleanupMutex.RLock()
		defer p.cleanupMutex.RUnlock()

		if p.gitServer == nil {
			http.Error(writer, "server closed", http.StatusInternalServerError)
			return
		}

		username, password, ok := request.BasicAuth()
		if p.failWhenAuth {
			if ok {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
		} else if p.authUsername != "" {
			if p.authUsername != username || p.authPassword != password {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		p.gitServer.ServeHTTP(writer, request)
	})

	p.gitHttpServer = &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: handler,
	}

	ln := port_tool.NewListenerWithUniquePort("127.0.0.1")
	a := ln.Addr().(*net.TCPAddr)
	p.gitServerPort = a.Port

	go func() {
		err := p.gitHttpServer.Serve(ln)
		if err != nil {
			p.t.Logf("gitHttpServer.Serve() with port %d returned error: %s", p.gitServerPort, err.Error())
		} else {
			p.t.Logf("gitHttpServer.Serve() with port %d returned with no error", p.gitServerPort)
		}
		close(p.cleanupDoneCh)
	}()
}

func (p *TestGitServer) Cleanup() {
	p.cleanupMutex.Lock()
	defer p.cleanupMutex.Unlock()

	p.t.Logf("gitHttpServer.Cleanup() called for port %d", p.gitServerPort)

	if p.gitHttpServer != nil {
		_ = p.gitHttpServer.Shutdown(context.Background())
		p.gitHttpServer = nil
		p.gitServer = nil
		<-p.cleanupDoneCh
	}

	p.baseDir = ""
}

func (p *TestGitServer) GitInit(repo string) {
	dir := p.LocalRepoDir(repo)

	err := os.MkdirAll(dir, 0o700)
	if err != nil {
		p.t.Fatal(err)
	}

	r, err := git.PlainInit(dir, false)
	if err != nil {
		p.t.Fatal(err)
	}

	_, err = r.CreateRemote(&config2.RemoteConfig{
		Name: "origin",
		URLs: []string{p.GitRepoUrl(repo)},
	})
	if err != nil {
		p.t.Fatal(err)
	}

	config, err := r.Config()
	if err != nil {
		p.t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		p.t.Fatal(err)
	}

	config.User.Name = "Test User"
	config.User.Email = "no@mail.com"
	config.Author = config.User
	config.Committer = config.User
	err = r.SetConfig(config)
	if err != nil {
		p.t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, ".dummy"))
	if err != nil {
		p.t.Fatal(err)
	}
	_ = f.Close()
	_, err = wt.Add(".dummy")
	if err != nil {
		p.t.Fatal(err)
	}
	_, err = wt.Commit("initial", &git.CommitOptions{})
	if err != nil {
		p.t.Fatal(err)
	}
}

func (p *TestGitServer) CommitFiles(repo string, add []string, all bool, message string) {
	r, err := git.PlainOpen(p.LocalRepoDir(repo))
	if err != nil {
		p.t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		p.t.Fatal(err)
	}
	for _, a := range add {
		_, err = wt.Add(a)
		if err != nil {
			p.t.Fatal(err)
		}
	}
	_, err = wt.Commit(message, &git.CommitOptions{
		All: all,
	})
	if err != nil {
		p.t.Fatal(err)
	}
}

func (p *TestGitServer) CommitYaml(repo string, pth string, message string, o map[string]any) {
	fullPath := filepath.Join(p.LocalRepoDir(repo), pth)

	dir, _ := filepath.Split(fullPath)
	if dir != "" {
		err := os.MkdirAll(dir, 0o700)
		if err != nil {
			panic(err)
		}
	}

	b, err := yaml.Marshal(o)
	if err != nil {
		p.t.Fatal(err)
	}
	err = os.WriteFile(fullPath, b, 0o600)
	if err != nil {
		p.t.Fatal(err)
	}
	if message == "" {
		message = fmt.Sprintf("update %s", filepath.Join(repo, pth))
	}
	p.CommitFiles(repo, []string{pth}, false, message)
}

func (p *TestGitServer) UpdateFile(repo string, pth string, update func(f string) (string, error), message string) {
	fullPath := filepath.Join(p.LocalRepoDir(repo), pth)
	f := ""
	if _, err := os.Stat(fullPath); err == nil {
		b, err := os.ReadFile(fullPath)
		if err != nil {
			p.t.Fatal(err)
		}
		f = string(b)
	}

	newF, err := update(f)
	if err != nil {
		p.t.Fatal(err)
	}

	if f == newF {
		return
	}
	err = os.MkdirAll(filepath.Dir(fullPath), 0o700)
	if err != nil {
		p.t.Fatal(err)
	}
	err = os.WriteFile(fullPath, []byte(newF), 0o600)
	if err != nil {
		p.t.Fatal(err)
	}
	p.CommitFiles(repo, []string{pth}, false, message)
}

func (p *TestGitServer) UpdateYaml(repo string, pth string, update func(o map[string]any) error, message string) {
	fullPath := filepath.Join(p.LocalRepoDir(repo), pth)

	var o map[string]any
	if _, err := os.Stat(fullPath); err == nil {
		b, err := os.ReadFile(fullPath)
		if err != nil {
			p.t.Fatal(err)
		}
		err = yaml.Unmarshal(b, &o)
		if err != nil {
			p.t.Fatal(err)
		}
	} else {
		o = map[string]any{}
	}

	var orig map[string]any
	err := copier.CopyWithOption(&orig, &o, copier.Option{DeepCopy: true})
	if err != nil {
		p.t.Fatal(err)
	}

	err = update(o)
	if err != nil {
		p.t.Fatal(err)
	}
	if reflect.DeepEqual(o, orig) {
		return
	}
	p.CommitYaml(repo, pth, message, o)
}

func (p *TestGitServer) DeleteFile(repo string, pth string, message string) {
	fullPath := filepath.Join(p.LocalRepoDir(repo), pth)
	_ = os.Remove(fullPath)

	if message == "" {
		message = fmt.Sprintf("delete %s", filepath.Join(repo, pth))
	}
	p.CommitFiles(repo, []string{pth}, false, message)
}

func (p *TestGitServer) GitHost() string {
	return fmt.Sprintf("localhost:%d", p.gitServerPort)
}

func (p *TestGitServer) GitUrl() string {
	return fmt.Sprintf("http://localhost:%d", p.gitServerPort)
}

func (p *TestGitServer) GitRepoUrl(repo string) string {
	return fmt.Sprintf("%s/%s/.git", p.GitUrl(), repo)
}

func (p *TestGitServer) LocalRepoDir(repo string) string {
	return filepath.Join(p.baseDir, repo)
}

func (p *TestGitServer) GetGitRepo(repo string) *git.Repository {
	r, err := git.PlainOpen(p.LocalRepoDir(repo))
	if err != nil {
		p.t.Fatal(err)
	}
	return r
}

func (p *TestGitServer) GetWorktree(repo string) *git.Worktree {
	r := p.GetGitRepo(repo)
	wt, err := r.Worktree()
	if err != nil {
		p.t.Fatal(err)
	}
	return wt
}
