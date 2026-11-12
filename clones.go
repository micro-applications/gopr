package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
