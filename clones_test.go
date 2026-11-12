package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/godiff/internal/git"
	"github.com/egoist/godiff/internal/github"
)

// freshState gives the test a state of its own, kept in a temporary
// directory, and restores the previous one afterwards.
func freshState(t *testing.T) {
	t.Helper()
	old := state
	state = &store{
		path: filepath.Join(t.TempDir(), "state.json"),
		data: storeData{Viewed: map[string]map[string]string{}, Clones: map[string]string{}},
	}
	t.Cleanup(func() { state = old })
}

// fakeGitHubGit makes git reach the local repository at server in place of
// repo on GitHub, and cloneURL name it so.
func fakeGitHubGit(t *testing.T, server string, repo github.Repo) {
	t.Helper()
	fakeGitHubGits(t, map[github.Repo]string{repo: server})
}

// fakeGitHubGits is fakeGitHubGit for several repositories at once: git
// reads one global config, so they all go in it.
func fakeGitHubGits(t *testing.T, servers map[github.Repo]string) {
	t.Helper()
	urlOf := func(r github.Repo) string { return "https://github.com/" + r.Owner + "/" + r.Name + ".git" }
	var conf strings.Builder
	for repo, server := range servers {
		conf.WriteString("[url \"" + server + "\"]\n\tinsteadOf = " + urlOf(repo) + "\n")
	}
	rc := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(rc, []byte(conf.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", rc)
	old := cloneURL
	cloneURL = urlOf
	t.Cleanup(func() { cloneURL = old })
}

// pullClone is pullServer with the clone's origin on GitHub, which
// fetches reach the local server through.
func pullClone(t *testing.T, repo github.Repo) (clone, base, head string) {
	t.Helper()
	server, clone, base, head := pullServer(t)
	gitIn(t, clone, "remote", "set-url", "origin", "https://github.com/"+repo.Owner+"/"+repo.Name+".git")
	fakeGitHubGit(t, server, repo)
	return clone, base, head
}

// repoWithRemote makes a repository in dir with a remote.
func repoWithRemote(t *testing.T, dir, url string) string {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "remote", "add", "origin", url)
	return dir
}

func TestFindClone(t *testing.T) {
	freshState(t)
	api := repoWithRemote(t, t.TempDir(), "https://github.com/acme/api.git")
	web := repoWithRemote(t, t.TempDir(), "git@github.com:acme/web.git")
	both := []string{api, web}
	if got := findClone(github.Repo{Owner: "acme", Name: "web"}, both); got != web {
		t.Errorf("found %q, want %q", got, web)
	}
	if got := findClone(github.Repo{Owner: "acme", Name: "none"}, both); got != "" {
		t.Errorf("found %q, want none", got)
	}
}

func TestScanProjects(t *testing.T) {
	freshState(t)
	root := t.TempDir()
	for _, p := range []string{"a/x/.git", "b/.git", "c/d/e/.git"} {
		os.MkdirAll(filepath.Join(root, p), 0o755)
	}
	got := scanProjects(root)
	has := func(p string) bool {
		for _, g := range got {
			if g == filepath.Join(root, p) {
				return true
			}
		}
		return false
	}
	if !has("a/x") || !has("b") || has("c/d/e") || len(got) != 2 {
		t.Errorf("scanned %v", got)
	}
}

func TestLocateCloneDropsStalePath(t *testing.T) {
	freshState(t)
	root := t.TempDir()
	api := github.Repo{Owner: "acme", Name: "api"}
	state.setClonePath(api, filepath.Join(root, "gone"))
	found := repoWithRemote(t, filepath.Join(root, "OpenSource", "api"), "https://github.com/acme/api.git")
	if got := locateClone(api, root); got != found {
		t.Fatalf("located %q, want %q", got, found)
	}
	if got := state.clonePath(api); got != found {
		t.Errorf("saved %q, want %q", got, found)
	}
}

func TestCloneRepo(t *testing.T) {
	freshState(t)
	app := github.Repo{Owner: "acme", Name: "app"}
	server, _, _, _ := pullServer(t)
	fakeGitHubGit(t, server, app)
	root := t.TempDir()
	dir, err := cloneRepo(context.Background(), app, root)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "acme", "app"); dir != want {
		t.Errorf("cloned to %q, want %q", dir, want)
	}
	if _, err := git.Open(dir); err != nil {
		t.Error(err)
	}
	if got := state.clonePath(app); got != dir {
		t.Errorf("saved %q, want %q", got, dir)
	}
}

func TestCloneRepoRefusesExistingDir(t *testing.T) {
	freshState(t)
	app := github.Repo{Owner: "acme", Name: "app"}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "acme", "app"), "notes.txt", "mine\n")
	// No fake git: running git here would fail or reach the network.
	_, err := cloneRepo(context.Background(), app, root)
	if err == nil || !strings.Contains(err.Error(), "exists and is not a clone") {
		t.Fatalf("error %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "acme", "app", "notes.txt")); err != nil {
		t.Error(err)
	}
}
