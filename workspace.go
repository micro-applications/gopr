package main

import (
	"context"
	"fmt"

	"github.com/egoist/godiff/internal/git"
	"github.com/egoist/godiff/internal/github"
)

// opening is a pull request whose repository the window is finding.
type opening struct {
	repo   github.Repo
	number int
}

// resolveClone returns the local clone of a repository: the one found, else
// a new one under root, cloned as status says. Tests replace it.
var resolveClone = func(ctx context.Context, want github.Repo, root string, status func(string)) (string, error) {
	if dir := locateClone(want, root); dir != "" {
		return dir, nil
	}
	status(fmt.Sprintf("Cloning %s/%s…", want.Owner, want.Name))
	return cloneRepo(ctx, want, root)
}

// openPull shows a pull request of any repository: it finds the
// repository's clone, or clones it, and binds the window to it. The last
// pull request chosen wins over those still opening.
func (w *window) openPull(repo github.Repo, n int) {
	op := &opening{repo: repo, number: n}
	w.opening = op
	w.openErr = nil
	w.loadStatus = fmt.Sprintf("Finding %s/%s…", repo.Owner, repo.Name)
	root := w.settings.cloneRoot()
	w.background(func() {
		status := func(s string) {
			w.update(func() {
				if w.opening == op {
					w.loadStatus = s
				}
			})
		}
		var found *git.Repo
		dir, err := resolveClone(context.Background(), repo, root, status)
		if err == nil {
			found, err = git.Open(dir)
		}
		w.update(func() {
			if w.opening != op {
				return
			}
			if err != nil {
				// The failed pull request stays, for Try Again.
				w.openErr = err
				return
			}
			w.opening = nil
			src := pullSource(repo, n)
			rebound := w.repo == nil || w.repo.Root != found.Root
			if rebound {
				w.bindRepo(found, repo)
			}
			if rebound && w.source == src {
				// bindRepo made the load of the source stale.
				w.load()
				return
			}
			w.setSource(src)
		})
	})
}

// bindRepo makes the window review a repository in place of the one it
// had: loads in flight for the old one go stale.
func (w *window) bindRepo(repo *git.Repo, want github.Repo) {
	w.gen++
	w.genA.Store(int64(w.gen))
	w.repo = repo
	w.gh, _ = findGitHubRemote(repo, want)
	w.treeSel = ""
	w.closedDirs = map[string]bool{}
	w.changed = false
	w.loadUser()
	state.addRecent(repo.Root)
}
