package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/godiff/internal/git"
	"github.com/egoist/godiff/internal/github"
	"github.com/egoist/godiff/internal/proc"
	"github.com/egoist/godiff/internal/shellenv"
)

func cloneKey(repo github.Repo) string { return strings.ToLower(repo.String()) }

// clonePath returns the local clone chosen for a repository, "" if none.
func (s *store) clonePath(repo github.Repo) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Clones[cloneKey(repo)]
}

// setClonePath keeps the local clone of a repository; "" forgets it.
func (s *store) setClonePath(repo github.Repo, path string) {
	s.mu.Lock()
	if path == "" {
		delete(s.data.Clones, cloneKey(repo))
	} else {
		if s.data.Clones == nil {
			s.data.Clones = map[string]string{}
		}
		s.data.Clones[cloneKey(repo)] = path
	}
	s.mu.Unlock()
	s.save()
}

// findClone returns the first of dirs that is a git repository with a
// remote of want, "" if none.
func findClone(want github.Repo, dirs []string) string {
	for _, dir := range dirs {
		repo, err := git.Open(dir)
		if err != nil {
			continue
		}
		if _, ok := findGitHubRemote(repo, want); ok {
			return dir
		}
	}
	return ""
}

// scanProjects lists the directories of root, and of its directories,
// that hold a .git.
func scanProjects(root string) []string {
	var found []string
	for _, pattern := range []string{"*", "*/*"} {
		dirs, _ := filepath.Glob(filepath.Join(root, pattern))
		for _, dir := range dirs {
			if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
				found = append(found, dir)
			}
		}
	}
	return found
}

// locateClone finds the local clone of a repository, trying the one kept,
// the repositories opened, then those under root, and keeps what it finds.
// A kept path that no longer is a clone of it is forgotten.
func locateClone(want github.Repo, root string) string {
	if saved := state.clonePath(want); saved != "" {
		if findClone(want, []string{saved}) != "" {
			return saved
		}
		state.setClonePath(want, "")
	}
	for _, dirs := range [][]string{state.recent(), scanProjects(root)} {
		if dir := findClone(want, dirs); dir != "" {
			state.setClonePath(want, dir)
			return dir
		}
	}
	return ""
}

// cloneURL is the URL to clone a repository from: SSH when the GitHub CLI
// is set to it, else HTTPS.
var cloneURL = func(r github.Repo) string {
	if gh, err := shellenv.LookPath("gh"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, gh, "config", "get", "git_protocol")
		proc.HideConsole(cmd)
		cmd.Env = shellenv.Environ()
		if out, err := cmd.Output(); err == nil && strings.TrimSpace(string(out)) == "ssh" {
			return fmt.Sprintf("git@github.com:%s/%s.git", r.Owner, r.Name)
		}
	}
	return fmt.Sprintf("https://github.com/%s/%s.git", r.Owner, r.Name)
}

// cloneRepo clones a repository into root/owner/name and keeps the path.
// It leaves alone a directory that holds anything.
func cloneRepo(ctx context.Context, want github.Repo, root string) (string, error) {
	dir := filepath.Join(root, want.Owner, want.Name)
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return "", fmt.Errorf("%s exists and is not a clone of %s/%s", dir, want.Owner, want.Name)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	if err := git.Clone(ctx, cloneURL(want), dir); err != nil {
		return "", err
	}
	state.setClonePath(want, dir)
	return dir, nil
}

// resolving are the repositories whose clones are being found or made.
var resolving = &inflight{calls: map[string]*resolveCall{}}

// inflight runs one resolve of a repository at a time: the others wait
// for its result, and hear its status.
type inflight struct {
	mu    sync.Mutex
	calls map[string]*resolveCall
}

type resolveCall struct {
	done     chan struct{}
	dir      string
	err      error
	status   string         // the last status, under inflight.mu
	watchers []func(string) // those told of the status, under inflight.mu
}

// do runs resolve for a repository, or waits for the run already going,
// and returns its result.
func (f *inflight) do(ctx context.Context, want github.Repo, status func(string), resolve func(status func(string)) (string, error)) (string, error) {
	key := cloneKey(want)
	f.mu.Lock()
	if c := f.calls[key]; c != nil {
		c.watchers = append(c.watchers, status)
		last := c.status
		f.mu.Unlock()
		if last != "" {
			status(last)
		}
		select {
		case <-c.done:
			return c.dir, c.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	c := &resolveCall{done: make(chan struct{}), watchers: []func(string){status}}
	f.calls[key] = c
	f.mu.Unlock()

	c.dir, c.err = resolve(func(s string) {
		f.mu.Lock()
		c.status = s
		watchers := slices.Clone(c.watchers)
		f.mu.Unlock()
		for _, w := range watchers {
			w(s)
		}
	})
	// A later call, as Try Again, resolves anew.
	f.mu.Lock()
	delete(f.calls, key)
	f.mu.Unlock()
	close(c.done)
	return c.dir, c.err
}

// waiting is how many wait for the resolve of a repository, for tests.
func (f *inflight) waiting(want github.Repo) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c := f.calls[cloneKey(want)]; c != nil {
		return len(c.watchers) - 1
	}
	return 0
}
