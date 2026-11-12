package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/godiff/internal/diff"
	"github.com/egoist/godiff/internal/git"
	"github.com/egoist/godiff/internal/github"
	"github.com/egoist/godiff/internal/highlight"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// sourceKind is what a window reviews.
type sourceKind uint8

const (
	sourceWorkingTree sourceKind = iota // uncommitted changes
	sourceCommit                        // a commit, against its first parent
	sourceBranch                        // the work tree since it branched from a branch
	sourcePull                          // a pull request on GitHub, since it branched
)

type source struct {
	kind sourceKind
	ref  string // the commit, the branch, or the pull request as owner/repo#123
}

// session is the review of one source, kept while the window shows
// another.
type session struct {
	comments []*comment
	viewed   map[string]string
}

// window is a window reviewing a repository.
type window struct {
	win      *mygo.Window
	repo     *git.Repo
	launch   source // what the window was opened on
	source   source
	settings Settings

	branch  string
	files   []*fileState
	commit  *git.Commit // the commit reviewed, for sourceCommit
	base    string      // the merge base, for sourceBranch and sourcePull
	loading bool
	loadErr error
	gen     int // counts loads, to drop the results of older ones
	genA    atomic.Int64
	// loadStart is when the load started.
	loadStart time.Time
	viewed    map[string]string
	// loadStatus says what a slow load does, as fetching a pull request.
	loadStatus string

	// The sidebar.
	sidebarShown bool
	sidebarWidth float32
	dragWidth    float32

	// The files pane, and its tree of the files.
	filesShown     bool
	filesWidth     float32
	filesDragWidth float32
	filesDragging  bool
	filter         string
	filterFocus    bool
	treeList       ui.ListState
	treeSel        string // the key of the row chosen
	treeRows       []treeRow
	closedDirs     map[string]bool
	treeItems      map[string]*treeNode
	treeRoots      []string
	// reloaded are the files that changed in the last refresh.
	reloaded map[string]bool

	// The diff surface.
	rows       []row
	rowsDirty  bool
	list       ui.ListState
	current    int // the file shown at the top of the surface
	hscroll    map[string]float32
	revealedAt time.Time
	now        time.Time
	charW      float32
	charKey    string
	// The hunk chosen with j and k.
	selFile, selHunk int
	focusList        bool
	focusedOnce      bool
	switchedAt       time.Time // when the source last changed, for GODIFF_DEBUG
	debugPressed     bool
	// hold keeps the background work of tests in held, to run frames
	// while it waits.
	hold bool
	held []func()
	// typing is set while a field has the focus, whose keys are its own.
	typing bool

	// Find in diffs.
	finding     bool
	query       string
	matches     []match
	match       int
	matchesFor  string
	fileMatches map[int]bool

	// The tabs of whole files, and the one shown: 0 for the changes, i for
	// fileTabs[i-1].
	fileTabs  []fileTab
	activeTab int
	// The revisions of the old and the new side of the last load; "" is
	// the work tree.
	oldRev, newRev string

	// The reviews of the other sources.
	sessions map[source]*session

	// GitHub: the repository's remote there, and the pull requests
	// reviewed, the one shown in pr.
	gh  ghRemote
	pr  *pullState
	prs map[source]*pullState

	// The groups of the files: by kind, or as an agent's review has them.
	groupPref groupMode
	groups    []fileGroup
	// notes are the notes of the agent's review, by path, and noteSel the
	// one gone to last.
	notes   map[string][]*aiNote
	noteSel *aiNote
	// regroupPending groups the files again as the next frame starts.
	regroupPending bool
	// placed are the threads and notes the rows show under their lines,
	// as the rows are built: the others show at the end of their files.
	placed map[any]bool
	// The agents' reviews, by source.
	analyses map[source]*analysisState
	// Review comments, the one asked about discarding, and the git user.
	comments   []*comment
	discarding *comment
	user       string
	copied     string
	copiedAt   time.Time

	// The command bar.
	paletteOpen  bool
	paletteQuery string
	paletteRow   int
	paletteList  ui.ListState
	// palettePointer is where the pointer was over the list.
	palettePointer [2]float32

	// The dialog opening a pull request.
	dialog      dialogKind
	dialogOpen  bool
	dialogValue string
	dialogErr   string

	// changed is set when the pull request shown has new commits.
	changed bool
	help    bool

	// The pull request whose repository is being found, and why it could
	// not be.
	opening *opening
	openErr error

	// The sidebar's pull requests.
	inbox inboxState
}

var (
	windowsMu sync.Mutex
	windows   []*window
)

// openMu keeps two launches from making two windows.
var openMu sync.Mutex

// openApp makes the window of the app, or brings the one open to the front.
func openApp() *window {
	openMu.Lock()
	defer openMu.Unlock()
	windowsMu.Lock()
	if len(windows) > 0 {
		w := windows[0]
		windowsMu.Unlock()
		w.win.Show()
		w.win.Focus()
		return w
	}
	windowsMu.Unlock()

	w := newWindow(nil, source{})
	w.win = mygo.NewWindow(mygo.WindowOptions{
		Title:          "GoPR",
		Width:          1280,
		Height:         860,
		MinWidth:       720,
		MinHeight:      420,
		StateKey:       "main",
		TitleBarStyle:  mygo.TitleBarHiddenInset,
		TitleBarHeight: 52,
		Vibrancy:       mygo.VibrancySidebar,
		Content:        ui.View(w.view),
	})
	windowsMu.Lock()
	windows = append(windows, w)
	windowsMu.Unlock()
	w.win.OnClose(func(*mygo.CloseEvent) {
		if w.settings.CopyCommentsOnClose && len(w.comments) > 0 {
			mygo.Clipboard.WriteText(w.commentsMarkdown())
		}
	})
	offSettings := cfg.OnChange(func(s Settings) { w.win.Update(func() { w.applySettings(s) }) })
	w.win.OnClosed(func() {
		offSettings()
		windowsMu.Lock()
		for i, o := range windows {
			if o == w {
				windows = append(windows[:i], windows[i+1:]...)
				break
			}
		}
		windowsMu.Unlock()
	})
	w.win.OnFocus(func() {
		go w.checkChanges()
		w.win.Update(w.refreshInboxIfStale)
	})
	w.loadInbox()
	w.captureIfAsked()
	return w
}

// newWindow makes the state of a window reviewing a source of a
// repository, or of a window waiting for a pull request, for a nil repo.
func newWindow(repo *git.Repo, src source) *window {
	width, shown := state.layout()
	filesWidth, filesShown := state.filesLayout()
	w := &window{
		repo:         repo,
		launch:       src,
		source:       src,
		settings:     cfg.Get(),
		sidebarShown: shown,
		sidebarWidth: width,
		filesShown:   filesShown,
		filesWidth:   filesWidth,
		viewed:       map[string]string{},
		hscroll:      map[string]float32{},
		closedDirs:   map[string]bool{},
		reloaded:     map[string]bool{},
		fileMatches:  map[int]bool{},
		selFile:      -1,
		selHunk:      -1,
		prs:          map[source]*pullState{},
		analyses:     map[source]*analysisState{},
	}
	if repo != nil {
		w.viewed = state.viewed(viewedKey(repo.Root, src))
		w.gh, _ = findGitHubRemote(repo, github.Repo{})
		if src.kind == sourcePull {
			w.pr = w.pullFor(src)
		}
		w.loadAnalysis(src)
	}
	w.dragWidth = w.sidebarWidth
	w.filesDragWidth = w.filesWidth
	w.list.Key = func(i int) any { return w.key(&w.rows[i]) }
	w.list.Header = func(i int) bool { return w.rows[i].kind == rowHeader }
	return w
}

// windowTitle is "<repository>[/<source>] · GoPR".
func windowTitle(root string, src source) string {
	name := filepath.Base(root)
	switch src.kind {
	case sourceCommit:
		name += "/" + shortHash(src.ref)
	case sourceBranch:
		name += "/" + src.ref
	case sourcePull:
		_, n := src.pull()
		name += fmt.Sprintf("/#%d", n)
	}
	return name + " · GoPR"
}

func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

// applySettings takes settings changed in the file or the menus.
func (w *window) applySettings(s Settings) {
	old := w.settings
	w.settings = s
	if old.ShowWhitespace != s.ShowWhitespace {
		w.load()
	}
	if old.DiffStyle != s.DiffStyle || old.WordWrap != s.WordWrap || old.CodeFontSize != s.CodeFontSize {
		w.rowsDirty = true
	}
}

// background runs fn on a goroutine of its own, or at once in tests,
// which have no window.
func (w *window) background(fn func()) {
	if w.win == nil {
		if w.hold {
			// Tests run frames while the work waits.
			w.held = append(w.held, fn)
			return
		}
		fn()
		return
	}
	go fn()
}

// update runs fn on the main thread, then draws a frame.
func (w *window) update(fn func()) {
	if w.win == nil {
		fn()
		return
	}
	if debugFrames {
		inner := fn
		fn = func() {
			start := time.Now()
			inner()
			if d := time.Since(start); d > 2*time.Millisecond {
				log.Printf("slow update: %v", d)
			}
		}
	}
	w.win.Update(fn)
}

func (w *window) invalidate() {
	if w.win != nil {
		w.win.Invalidate()
	}
}

func (w *window) split() bool { return w.settings.DiffStyle != "unified" }

// splitFile reports whether a file shows side by side: files that are
// all new or all gone show in one column.
func (w *window) splitFile(f *fileState) bool {
	if !w.split() {
		return false
	}
	return !f.oneSided()
}

// setSource shows another source in the window at once; its changes come
// as they load.
func (w *window) setSource(src source) {
	if w.source == src {
		return
	}
	if debugFrames {
		w.switchedAt = time.Now()
		log.Printf("switch to %+v", src)
		go probeMainThread(time.Second)
	}
	// The review takes the focus as the window opens only: the user, who
	// chose this source, keeps the focus where they put it.
	w.focusedOnce = true
	w.switchTo(src)
	w.load()
}

// switchTo makes a source the one shown, its review in place of the
// other's.
func (w *window) switchTo(src source) {
	// Each source keeps its review: the comments, and what was viewed of a
	// commit.
	if w.sessions == nil {
		w.sessions = map[source]*session{}
	}
	w.sessions[w.source] = &session{comments: w.comments, viewed: w.viewed}
	w.comments, w.viewed = nil, nil
	if s := w.sessions[src]; s != nil {
		w.comments, w.viewed = s.comments, s.viewed
	}
	if w.viewed == nil {
		w.viewed = map[string]string{}
		if w.repo != nil {
			if key := viewedKey(w.repo.Root, src); key != "" {
				w.viewed = state.viewed(key)
			}
		}
	}
	w.source = src
	w.files, w.rows, w.commit, w.groups = nil, nil, nil, nil
	w.pr = nil
	if src.kind == sourcePull {
		w.pr = w.pullFor(src)
	}
	w.loadAnalysis(src)
	// The tree refers to the files by their index: it goes with them.
	w.buildTree()
	w.loadErr = nil
	w.list = ui.ListState{Key: w.list.Key, Header: w.list.Header}
	w.selFile, w.selHunk = -1, -1
	w.finding, w.query = false, ""
	w.fileTabs, w.activeTab = nil, 0
	w.hscroll = map[string]float32{}
	w.current = 0
	if w.win != nil && w.repo != nil {
		w.win.SetTitle(windowTitle(w.repo.Root, src))
	}
}

// preloadBudget is how long after it starts a load may wait for the
// contents of the first files, to show them colored at once rather than
// as they come.
const preloadBudget = 80 * time.Millisecond

// load reads the changes of the source, the contents of the first files
// within preloadBudget, then the contents of the others.
func (w *window) load() {
	if w.repo == nil {
		return
	}
	repo := w.repo
	w.gen++
	gen := w.gen
	src := w.source
	opts := git.Options{ShowWhitespace: w.settings.ShowWhitespace}
	w.loading = true
	w.loadStart = time.Now()
	started := w.loadStart
	w.loadErr = nil
	w.changed = false
	w.genA.Store(int64(gen))
	// What is known already needs no git: the commit, and the branch, which
	// showing a commit does not change.
	known := w.commit
	if known != nil && known.Hash != src.ref {
		known = nil
	}
	branch := w.branch
	needBranch := src.kind != sourceCommit || branch == ""
	remote := w.pullRemote(src)
	w.loadStatus = ""
	// Files unchanged since the last load keep their contents.
	loadedFiles := map[string]bool{}
	for _, f := range w.files {
		if f.loaded {
			loadedFiles[f.Path+"\x00"+f.Fingerprint] = true
		}
	}
	w.background(func() {
		var (
			files  []*diff.File
			commit = known
			base   string
			pulled pulledRequest
			err    error
			wg     sync.WaitGroup
		)
		if needBranch {
			wg.Add(1)
			go func() {
				defer wg.Done()
				branch = repo.Branch()
			}()
		}
		switch src.kind {
		case sourceWorkingTree:
			files, err = repo.WorkingTree(opts)
		case sourceCommit:
			if commit == nil {
				var c git.Commit
				c, err = repo.CommitInfo(src.ref)
				commit = &c
			}
			if err == nil {
				files, err = repo.CommitDiff(*commit, opts)
			}
		case sourceBranch:
			files, base, err = repo.Compare(src.ref, opts)
		case sourcePull:
			pulled, err = w.fetchPull(gen, repo, src, remote, opts)
			files, base = pulled.files, pulled.mergeBase
		}
		wg.Wait()
		// The revisions of the old and the new side; "" is the work tree.
		var oldRev, newRev string
		switch src.kind {
		case sourceWorkingTree:
			if repo.HasHead() {
				oldRev = "HEAD"
			}
		case sourceCommit:
			if commit != nil && len(commit.Parents) > 0 {
				oldRev = commit.Parents[0]
			}
			newRev = src.ref
		case sourceBranch:
			oldRev = base
		case sourcePull:
			oldRev, newRev = base, pulled.head
		}
		var pre map[string]loaded
		if err == nil && !w.stale(gen) {
			var fresh []*diff.File
			for _, f := range files {
				if !loadedFiles[f.Path+"\x00"+f.Fingerprint] {
					fresh = append(fresh, f)
				}
			}
			if budget := preloadBudget - time.Since(started); budget > 0 {
				pre = w.preload(repo, fresh, oldRev, newRev, budget)
			}
		}
		w.update(func() {
			if gen != w.gen {
				return
			}
			w.loading = false
			w.loadErr = err
			w.loadStatus = ""
			w.branch = branch
			if err == nil {
				w.commit = commit
			}
			if src.kind == sourcePull && err == nil {
				w.pr.apply(pulled)
				w.loadReviews()
			}
			w.base = base
			w.oldRev, w.newRev = oldRev, newRev
			w.setFiles(files)
			for _, f := range w.files {
				if l, ok := pre[f.Path]; ok && !f.loaded {
					l.file = f
					l.apply()
				}
			}
			if err == nil {
				var rest []*fileState
				for _, f := range w.files {
					if !f.loaded {
						rest = append(rest, f)
					}
				}
				if len(rest) > 0 {
					w.background(func() { w.loadContents(gen, repo, oldRev, newRev, rest) })
				}
			}
		})
	})
}

// preload reads and colors the contents of files, in order, until
// budget runs out: the files it did not finish load later, as they come.
func (w *window) preload(repo *git.Repo, files []*diff.File, oldRev, newRev string, budget time.Duration) map[string]loaded {
	if len(files) == 0 {
		return nil
	}
	contents, err := repo.NewContents()
	if err != nil {
		return nil
	}
	defer boostGC()()
	results := make(chan loaded)
	var stop atomic.Bool
	go func() {
		defer close(results)
		defer contents.Close()
		for _, f := range files {
			if stop.Load() {
				return
			}
			l := read(repo, contents, f, oldRev, newRev)
			l.finish()
			select {
			case results <- l:
			case <-time.After(budget):
				return // nobody waits anymore
			}
		}
	}()
	out := map[string]loaded{}
	deadline := time.NewTimer(budget)
	defer deadline.Stop()
	for {
		select {
		case l, ok := <-results:
			if !ok {
				return out
			}
			out[l.path] = l
		case <-deadline.C:
			stop.Store(true)
			return out
		}
	}
}

// setFiles shows newly read files, keeping what the user did to those
// that did not change.
func (w *window) setFiles(files []*diff.File) {
	prev := map[string]*fileState{}
	for _, f := range w.files {
		prev[f.Path] = f
	}
	// A refresh marks the files that changed since, and opens them.
	refresh := len(prev) > 0
	w.reloaded = map[string]bool{}
	next := make([]*fileState, 0, len(files))
	for _, f := range files {
		fs := &fileState{File: f}
		switch p := prev[f.Path]; {
		case p != nil && p.Fingerprint == f.Fingerprint:
			fs.collapsed = p.collapsed
			fs.expanded = p.expanded
			// The contents stay until they load again.
			fs.oldLines, fs.newLines, fs.oldHL, fs.newHL = p.oldLines, p.newLines, p.oldHL, p.newHL
			fs.oldImage, fs.newImage, fs.oldSize, fs.newSize = p.oldImage, p.newImage, p.oldSize, p.newSize
			fs.loaded = p.loaded
		case refresh:
			w.reloaded[f.Path] = true
			fs.collapsed = f.Generated || f.Directory
		default:
			fs.collapsed = w.isViewed(fs) || f.Generated || f.Directory
		}
		next = append(next, fs)
	}
	w.files = next
	w.regroup()
	w.pruneComments()
}

// isViewed reports whether the user marked the file viewed, as it is now.
func (w *window) isViewed(f *fileState) bool {
	return f.Fingerprint != "" && w.viewed[f.Path] == f.Fingerprint
}

func (w *window) setViewed(f *fileState, viewed bool) {
	fp := ""
	if viewed {
		fp = f.Fingerprint
		w.viewed[f.Path] = fp
	} else {
		delete(w.viewed, f.Path)
	}
	// Only the work tree's and pull requests' files are remembered:
	// commits do not change.
	if key := viewedKey(w.repo.Root, w.source); key != "" {
		go state.setViewed(key, f.Path, fp)
	}
	f.collapsed = viewed
	w.rowsDirty = true
}

// maxContent is the size of the files whose contents are loaded.
const maxContent = 2 << 20

// loaded is a file's contents, read and tokenized, or decoded for
// pictures.
type loaded struct {
	file               *fileState // where they go, once known
	oldLines, newLines []string
	oldHL, newHL       [][]highlight.Seg
	oldData, newData   []byte // of pictures
	oldImage, newImage *ui.Bitmap
	path, oldPath      string
}

// read reads both sides of a file of a repository: lines of text, or the
// data of a picture.
func read(repo *git.Repo, contents *git.Contents, f *diff.File, oldRev, newRev string) loaded {
	l := loaded{path: f.Path, oldPath: f.OldPath}
	raw := func(rev, path string, limit int) []byte {
		var data []byte
		if rev == "" {
			data = repo.ReadWorkTree(path)
		} else {
			data, _ = contents.Read(rev, path)
		}
		if len(data) > limit {
			return nil
		}
		return data
	}
	hasOld := oldRev != "" && f.Status != diff.Added && f.Status != diff.Untracked
	hasNew := f.Status != diff.Deleted
	if f.Binary && isImage(f.Path) {
		if hasOld {
			l.oldData = raw(oldRev, f.OldPath, maxImage)
		}
		if hasNew {
			l.newData = raw(newRev, f.Path, maxImage)
		}
		return l
	}
	if f.Binary || f.Directory || f.TooLarge {
		return l
	}
	text := func(data []byte) []string {
		if data == nil || diff.IsBinary(data) {
			return nil
		}
		lines := diff.SplitLines(string(data))
		if lines == nil {
			lines = []string{}
		}
		return lines
	}
	if hasOld {
		l.oldLines = text(raw(oldRev, f.OldPath, maxContent))
	}
	if hasNew {
		l.newLines = text(raw(newRev, f.Path, maxContent))
	}
	return l
}

// finish colors the lines, and decodes the pictures.
func (l *loaded) finish() {
	l.oldHL = highlight.Lines(l.oldPath, l.oldLines)
	l.newHL = highlight.Lines(l.path, l.newLines)
	if l.oldData != nil {
		l.oldImage, _ = ui.DecodeBitmap(l.oldData)
	}
	if l.newData != nil {
		l.newImage, _ = ui.DecodeBitmap(l.newData)
	}
}

// apply gives the contents to their file, on the main thread.
func (l *loaded) apply() {
	f := l.file
	f.oldLines, f.newLines = l.oldLines, l.newLines
	f.oldHL, f.newHL = l.oldHL, l.newHL
	f.oldImage, f.newImage = l.oldImage, l.newImage
	f.oldSize, f.newSize = len(l.oldData), len(l.newData)
	f.loaded = true
	f.metricsDone = false
	f.spans = nil
}

// maxImage is the size of the pictures shown.
const maxImage = 32 << 20

// isImage reports whether a path is of a picture the app shows.
func isImage(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp", ".ico":
		return true
	}
	return false
}

// loadContents reads both sides of the files, for their colors and their
// unchanged lines, and shows them as they come.
func (w *window) loadContents(gen int, repo *git.Repo, oldRev, newRev string, files []*fileState) {
	defer boostGC()()
	contents, err := repo.NewContents()
	if err != nil {
		return
	}
	defer contents.Close()

	jobs := make(chan loaded)
	results := make(chan loaded)
	var wg sync.WaitGroup
	// A few workers: more would take the CPU, and the collector's pauses,
	// from the main thread, which draws the window.
	for range highlightWorkers() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				job.finish()
				results <- job
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, f := range files {
			if w.stale(gen) {
				return
			}
			job := read(repo, contents, f.File, oldRev, newRev)
			job.file = f
			jobs <- job
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	var batch []loaded
	flush := func() {
		if len(batch) == 0 {
			return
		}
		done := batch
		batch = nil
		w.update(func() {
			if gen != w.gen {
				return
			}
			for i := range done {
				done[i].apply()
			}
			w.rowsDirty = true
		})
	}
	tick := time.NewTicker(60 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case l, ok := <-results:
			if !ok {
				flush()
				return
			}
			batch = append(batch, l)
		case <-tick.C:
			flush()
		}
	}
}

// boostGC lets the heap grow further between collections while files load:
// Chroma's lexers make much garbage, and every collection has the main
// thread, which allocates as it draws, help with it. The function it
// returns ends the boost.
func boostGC() func() {
	gcBoost.Lock()
	if gcBoost.n == 0 {
		gcBoost.old = debug.SetGCPercent(400)
	}
	gcBoost.n++
	gcBoost.Unlock()
	return func() {
		gcBoost.Lock()
		gcBoost.n--
		if gcBoost.n == 0 {
			debug.SetGCPercent(gcBoost.old)
		}
		gcBoost.Unlock()
	}
}

var gcBoost struct {
	sync.Mutex
	n, old int
}

// highlightWorkers is how many files are highlighted at once.
func highlightWorkers() int { return min(max(runtime.NumCPU()/4, 1), 2) }

func (w *window) stale(gen int) bool { return w.genA.Load() != int64(gen) }

// loadUser reads the name of the git user, which comments show, off the
// main thread: running git takes a while.
func (w *window) loadUser() {
	repo := w.repo
	if repo == nil {
		return
	}
	w.background(func() {
		name := strings.TrimSpace(repo.ConfigValue("user.name"))
		if name == "" {
			name = strings.TrimSpace(repo.ConfigValue("user.email"))
		}
		w.update(func() {
			if w.repo == repo {
				w.user = name
			}
		})
	})
}

// checkChanges asks GitHub whether the pull request shown has new
// commits. It runs off the main thread.
func (w *window) checkChanges() {
	var src source
	var busy bool
	mygo.RunOnMain(func() { src, busy = w.source, w.loading || w.changed })
	if src.kind == sourcePull && !busy {
		w.checkPull(src)
	}
}

// refresh loads the source again.
func (w *window) refresh() {
	w.load()
}

// openInEditor opens a file of the repository in the user's editor, at a
// line.
func (w *window) openInEditor(path string, line int) {
	if w.repo == nil {
		return
	}
	root, command := w.repo.Root, w.settings.EditorCommand
	abs := filepath.Join(root, filepath.FromSlash(path))
	if _, err := os.Stat(abs); err != nil {
		abs = root
		line = 0
	}
	go func() {
		if err := openEditor(command, root, abs, line); err != nil {
			mygo.Dialog.Error("Could not open the file", err.Error())
		}
	}()
}

// abbreviateHome writes the home directory as ~.
func abbreviateHome(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && (p == home || strings.HasPrefix(p, home+string(filepath.Separator))) {
		return "~" + p[len(home):]
	}
	return p
}

// shortPath abbreviates the directories of a path to their first letter,
// as fish does: ~/P/fate.
func shortPath(p string) string {
	p = abbreviateHome(p)
	parts := strings.Split(p, "/")
	for i := 1; i < len(parts)-1; i++ {
		part := parts[i]
		if part == "" || part == "~" {
			continue
		}
		n := 1
		if strings.HasPrefix(part, ".") && len(part) > 1 {
			n = 2
		}
		r := []rune(part)
		parts[i] = string(r[:min(n, len(r))])
	}
	return strings.Join(parts, "/")
}

// errorText is the text of an error for the window.
func errorText(err error) string {
	var ge *git.Error
	if errors.As(err, &ge) && strings.TrimSpace(ge.Stderr) != "" {
		return strings.TrimSpace(ge.Stderr)
	}
	return fmt.Sprint(err)
}

// probeMainThread logs how long the main thread kept a function waiting,
// at worst, over a while: how long the window could not respond.
func probeMainThread(d time.Duration) {
	var worst, total time.Duration
	var slow, n int
	for end := time.Now().Add(d); time.Now().Before(end); n++ {
		start := time.Now()
		mygo.RunOnMain(func() {})
		wait := time.Since(start)
		total += wait
		worst = max(worst, wait)
		if wait > 16*time.Millisecond {
			slow++
		}
		time.Sleep(2 * time.Millisecond)
	}
	log.Printf("main thread: worst wait %v, average %v, %d of %d waits over 16ms", worst, total/time.Duration(max(n, 1)), slow, n)
}

// watchMainThread logs, for GODIFF_DEBUG, whenever the main thread keeps
// a function waiting over 30ms: when the window cannot respond.
func watchMainThread() {
	limit := 30 * time.Millisecond
	if ms, err := strconv.Atoi(os.Getenv("GODIFF_DEBUG_BLOCK_MS")); err == nil {
		limit = time.Duration(ms) * time.Millisecond
	}
	var gc debug.GCStats
	for {
		start := time.Now()
		mygo.RunOnMain(func() {})
		if wait := time.Since(start); wait > limit {
			n := gc.NumGC
			debug.ReadGCStats(&gc)
			pause := time.Duration(0)
			if len(gc.Pause) > 0 {
				pause = gc.Pause[0]
			}
			log.Printf("main thread blocked %v (GC cycles since last: %d, last pause %v)", wait, gc.NumGC-n, pause)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
