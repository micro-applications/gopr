package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/godiff/internal/agent"
	"github.com/egoist/mygo"
)

// Settings are the user's preferences, in ~/.gopr/gopr.jsonc as
// codiff keeps its own in ~/.codiff/codiff.jsonc.
type Settings struct {
	CodeFontFamily string `json:"codeFontFamily"`
	CodeFontSize   int    `json:"codeFontSize"`
	// CloneRoot is where pull requests' repositories are cloned, as
	// <cloneRoot>/<owner>/<name>, and looked for, when not opened before.
	CloneRoot           string `json:"cloneRoot"`
	CopyCommentsOnClose bool   `json:"copyCommentsOnClose"`
	DiffStyle           string `json:"diffStyle"` // split or unified
	EditorCommand       string `json:"editorCommand"`
	// GithubToken reads and reviews pull requests; empty, the token of
	// the GitHub CLI is used.
	GithubToken string `json:"githubToken"`
	// AIAgent reviews changes: claude, codex, opencode or pi, or empty for
	// the first installed. AIModel is the model it uses, of any of its
	// providers, as the AI Review card chooses it; empty for its own
	// default, which for OpenCode is the one its own interface starts
	// with.
	AIAgent string `json:"aiAgent"`
	AIModel string `json:"aiModel"`
	// MarkdownWidth is how wide Markdown previews read at most: narrow,
	// medium, wide or full.
	MarkdownWidth string `json:"markdownWidth"`
	// MarkdownFontSize is the size of Markdown previews' text.
	MarkdownFontSize     int    `json:"markdownFontSize"`
	ReviewCommentsPrefix string `json:"reviewCommentsPrefix"`
	SidebarPosition      string `json:"sidebarPosition"` // left or right
	ShowWhitespace       bool   `json:"showWhitespace"`
	Theme                string `json:"theme"` // system, light or dark
	WordWrap             bool   `json:"wordWrap"`
	// Zoom is how large the window shows everything, 1 for 100%: ⌘+ and
	// ⌘- step it.
	Zoom float64 `json:"zoom"`
}

func defaultSettings() Settings {
	return Settings{
		CloneRoot:            "~/Projects",
		CodeFontSize:         13,
		DiffStyle:            "split",
		MarkdownWidth:        "medium",
		MarkdownFontSize:     defaultMarkdownFontSize,
		ReviewCommentsPrefix: "# Address these Review Comments",
		SidebarPosition:      "left",
		Theme:                "system",
		Zoom:                 1,
	}
}

// normalize keeps the settings within what they may be.
func (s *Settings) normalize() {
	d := defaultSettings()
	s.CodeFontFamily = strings.TrimSpace(s.CodeFontFamily)
	s.CloneRoot = strings.TrimSpace(s.CloneRoot)
	if s.CloneRoot == "" {
		s.CloneRoot = d.CloneRoot
	}
	if s.CodeFontSize == 0 {
		s.CodeFontSize = d.CodeFontSize
	}
	s.CodeFontSize = min(max(s.CodeFontSize, 10), 32)
	if s.DiffStyle != "split" && s.DiffStyle != "unified" {
		s.DiffStyle = d.DiffStyle
	}
	if s.MarkdownFontSize == 0 {
		s.MarkdownFontSize = d.MarkdownFontSize
	}
	s.MarkdownFontSize = min(max(s.MarkdownFontSize, minMarkdownFontSize), maxMarkdownFontSize)
	if s.Zoom == 0 {
		s.Zoom = d.Zoom
	}
	s.Zoom = min(max(s.Zoom, zoomLevels[0]), zoomLevels[len(zoomLevels)-1])
	if markdownWidthOf(s.MarkdownWidth) == nil {
		s.MarkdownWidth = d.MarkdownWidth
	}
	if s.SidebarPosition != "left" && s.SidebarPosition != "right" {
		s.SidebarPosition = d.SidebarPosition
	}
	if s.Theme != "system" && s.Theme != "light" && s.Theme != "dark" {
		s.Theme = d.Theme
	}
	if s.ReviewCommentsPrefix == "" {
		s.ReviewCommentsPrefix = d.ReviewCommentsPrefix
	}
	s.GithubToken = strings.TrimSpace(s.GithubToken)
	s.AIModel = strings.TrimSpace(s.AIModel)
	if _, ok := agent.ByName(s.AIAgent); !ok {
		s.AIAgent = ""
	}
}

// cloneRoot is the directory repositories are cloned into, with ~ expanded.
func (s Settings) cloneRoot() string {
	root := s.CloneRoot
	if root == "" {
		root = defaultSettings().CloneRoot
	}
	if root == "~" || strings.HasPrefix(root, "~/") || strings.HasPrefix(root, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, root[1:])
		}
	}
	return root
}

// config holds the settings, shared by the windows, and keeps them in
// step with the file.
type config struct {
	mu       sync.Mutex
	path     string
	settings Settings
	modTime  time.Time
	onChange map[int]func(Settings)
	nextID   int
}

var cfg = newConfig()

func newConfig() *config {
	home, _ := os.UserHomeDir()
	c := &config{path: filepath.Join(home, ".gopr", "gopr.jsonc"), settings: defaultSettings()}
	c.load()
	return c
}

// migrateConfig copies the Godiff settings file to the GoPR one, once, and
// loads it. It runs from main, not at package init, so tests never touch
// the real home directory.
func (c *config) migrateConfig() {
	home, _ := os.UserHomeDir()
	migrateFile(c.path, filepath.Join(home, ".godiff", "godiff.jsonc"))
	c.load()
}

func (c *config) Get() Settings {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.settings
}

// Update changes the settings and writes them.
func (c *config) Update(fn func(s *Settings)) {
	c.mu.Lock()
	fn(&c.settings)
	c.settings.normalize()
	s := c.settings
	c.mu.Unlock()
	c.write(s)
	c.notify(s)
}

// OnChange calls fn whenever the settings change, from any goroutine, and
// returns a function that stops it.
func (c *config) OnChange(fn func(Settings)) (off func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.onChange == nil {
		c.onChange = map[int]func(Settings){}
	}
	c.nextID++
	id := c.nextID
	c.onChange[id] = fn
	return func() {
		c.mu.Lock()
		delete(c.onChange, id)
		c.mu.Unlock()
	}
}

func (c *config) notify(s Settings) {
	c.mu.Lock()
	fns := make([]func(Settings), 0, len(c.onChange))
	for _, fn := range c.onChange {
		fns = append(fns, fn)
	}
	c.mu.Unlock()
	for _, fn := range fns {
		fn(s)
	}
}

func (c *config) load() bool {
	fi, err := os.Stat(c.path)
	if err != nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if fi.ModTime().Equal(c.modTime) {
		return false
	}
	c.modTime = fi.ModTime()
	data, err := os.ReadFile(c.path)
	if err != nil {
		return false
	}
	var file struct {
		Settings json.RawMessage `json:"settings"`
	}
	if err := json.Unmarshal(stripJSONC(data), &file); err != nil {
		log.Printf("gopr: %s: %v", c.path, err)
		return false
	}
	s := defaultSettings()
	if len(file.Settings) > 0 {
		// Fields of the wrong type keep their defaults.
		var fields map[string]json.RawMessage
		json.Unmarshal(file.Settings, &fields)
		for k, v := range fields {
			one, _ := json.Marshal(map[string]json.RawMessage{k: v})
			json.Unmarshal(one, &s)
		}
	}
	s.normalize()
	c.settings = s
	return true
}

func (c *config) write(s Settings) {
	data, err := json.MarshalIndent(map[string]any{"settings": s}, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return
	}
	if err := os.WriteFile(c.path, append(data, '\n'), 0o644); err != nil {
		log.Printf("gopr: %v", err)
		return
	}
	if fi, err := os.Stat(c.path); err == nil {
		c.mu.Lock()
		c.modTime = fi.ModTime()
		c.mu.Unlock()
	}
}

// ensure writes the file with the defaults when there is none, for the
// user to edit.
func (c *config) ensure() string {
	if _, err := os.Stat(c.path); err != nil {
		c.write(c.Get())
	}
	return c.path
}

// watch reads the file again when it changes.
func (c *config) watch() {
	go func() {
		for range time.Tick(time.Second) {
			if c.load() {
				c.notify(c.Get())
			}
		}
	}()
}

// stripJSONC removes the comments and trailing commas of JSONC.
func stripJSONC(data []byte) []byte {
	var out bytes.Buffer
	inString := false
	for i := 0; i < len(data); i++ {
		ch := data[i]
		if inString {
			out.WriteByte(ch)
			if ch == '\\' && i+1 < len(data) {
				i++
				out.WriteByte(data[i])
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		switch {
		case ch == '"':
			inString = true
			out.WriteByte(ch)
		case ch == '/' && i+1 < len(data) && data[i+1] == '/':
			for i < len(data) && data[i] != '\n' {
				i++
			}
			out.WriteByte('\n')
		case ch == '/' && i+1 < len(data) && data[i+1] == '*':
			i += 2
			for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
				i++
			}
			i++
		case ch == ',':
			// A comma before a closing bracket goes.
			j := i + 1
			for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
				j++
			}
			if j < len(data) && (data[j] == '}' || data[j] == ']') {
				continue
			}
			out.WriteByte(ch)
		default:
			out.WriteByte(ch)
		}
	}
	return out.Bytes()
}

// store keeps what the app remembers apart from the settings: the files
// viewed in each repository, and the layout of the window.
type store struct {
	mu   sync.Mutex
	path string
	data storeData
	// saveMu keeps saves in order: the last snapshot taken is the last
	// written.
	saveMu sync.Mutex
}

type storeData struct {
	// Viewed maps repository roots to the fingerprints of the files
	// viewed, by path.
	Viewed       map[string]map[string]string `json:"viewed"`
	SidebarWidth float32                      `json:"sidebarWidth"`
	SidebarShown *bool                        `json:"sidebarShown"`
	// FilesWidth and FilesShown are the layout of the files pane.
	FilesWidth float32 `json:"filesWidth"`
	FilesShown *bool   `json:"filesShown"`
	// Recent are the repositories opened, the last first: the app opens
	// the first when started without one, from the Dock.
	Recent []string `json:"recent"`
	// Clones maps the lowercase owner/name of a GitHub repository to the
	// local clone chosen for it.
	Clones map[string]string `json:"clones"`
	// LastRepository is the repository opened last, as kept before Recent.
	LastRepository string `json:"lastRepository,omitempty"`
	// InboxDays is the period of the inbox, in days: 7 or 30.
	InboxDays int `json:"inboxDays,omitempty"`
	// InboxHidden are the organizations, and the viewer's login, unchecked
	// in the inbox's menu.
	InboxHidden []string `json:"inboxHidden,omitempty"`
	// InboxCollapsed are the sections of the inbox collapsed: mine, review.
	InboxCollapsed []string `json:"inboxCollapsed,omitempty"`
}

// maxRecent is how many repositories the store remembers.
const maxRecent = 10

var state = &store{}

func (s *store) open() {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return
	}
	s.path = filepath.Join(dir, "state.json")
	migrateFile(s.path, filepath.Join(filepath.Dir(dir), "Godiff", "state.json"))
	// The AI reviews saved, which analysisPath keeps beside the state.
	migrateDir(filepath.Join(dir, "analyses"), filepath.Join(filepath.Dir(dir), "Godiff", "analyses"))
	if data, err := os.ReadFile(s.path); err == nil {
		json.Unmarshal(data, &s.data)
	}
	if s.data.Viewed == nil {
		s.data.Viewed = map[string]map[string]string{}
	}
	if s.data.Clones == nil {
		s.data.Clones = map[string]string{}
	}
	if len(s.data.Recent) == 0 && s.data.LastRepository != "" {
		s.data.Recent = []string{s.data.LastRepository}
	}
	s.data.LastRepository = ""
}

func (s *store) viewed(root string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for k, v := range s.data.Viewed[root] {
		out[k] = v
	}
	return out
}

func (s *store) setViewed(root, path, fingerprint string) {
	s.mu.Lock()
	if s.data.Viewed == nil {
		s.data.Viewed = map[string]map[string]string{}
	}
	m := s.data.Viewed[root]
	if m == nil {
		m = map[string]string{}
		s.data.Viewed[root] = m
	}
	if fingerprint == "" {
		delete(m, path)
	} else {
		m[path] = fingerprint
	}
	s.mu.Unlock()
	s.save()
}

func (s *store) layout() (width float32, shown bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	width, shown = s.data.SidebarWidth, true
	if width == 0 {
		width = 280
	}
	if s.data.SidebarShown != nil {
		shown = *s.data.SidebarShown
	}
	return
}

func (s *store) setLayout(width float32, shown bool) {
	s.mu.Lock()
	s.data.SidebarWidth, s.data.SidebarShown = width, &shown
	s.mu.Unlock()
	s.save()
}

// filesLayout returns the width of the files pane, and whether it shows.
func (s *store) filesLayout() (width float32, shown bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	width, shown = s.data.FilesWidth, true
	if width == 0 {
		width = filesDefault
	}
	if s.data.FilesShown != nil {
		shown = *s.data.FilesShown
	}
	return
}

func (s *store) setFilesLayout(width float32, shown bool) {
	s.mu.Lock()
	s.data.FilesWidth, s.data.FilesShown = width, &shown
	s.mu.Unlock()
	s.save()
}

// recent returns the repositories opened, the last first.
func (s *store) recent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.data.Recent)
}

// addRecent puts a repository first among the recent ones, and reports
// whether that changed them.
func (s *store) addRecent(root string) bool {
	s.mu.Lock()
	if len(s.data.Recent) > 0 && s.data.Recent[0] == root {
		s.mu.Unlock()
		return false
	}
	recent := slices.DeleteFunc(slices.Clone(s.data.Recent), func(r string) bool { return r == root })
	recent = slices.Insert(recent, 0, root)
	s.data.Recent = recent[:min(len(recent), maxRecent)]
	s.mu.Unlock()
	s.save()
	return true
}

// inboxDays is the period of the inbox: 7 days unless 30 were chosen.
func (s *store) inboxDays() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.InboxDays == 30 {
		return 30
	}
	return 7
}

func (s *store) setInboxDays(days int) {
	s.mu.Lock()
	s.data.InboxDays = days
	s.mu.Unlock()
	s.save()
}

// inboxHidden returns the owners unchecked in the inbox's menu.
func (s *store) inboxHidden() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.data.InboxHidden)
}

func (s *store) setInboxHidden(hidden []string) {
	s.mu.Lock()
	s.data.InboxHidden = slices.Clone(hidden)
	s.mu.Unlock()
	s.save()
}

// inboxCollapsed reports whether a section of the inbox, mine or review, is
// collapsed.
func (s *store) inboxCollapsed(section string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Contains(s.data.InboxCollapsed, section)
}

func (s *store) setInboxCollapsed(section string, collapsed bool) {
	s.mu.Lock()
	list := slices.DeleteFunc(slices.Clone(s.data.InboxCollapsed), func(k string) bool { return k == section })
	if collapsed {
		list = append(list, section)
	}
	s.data.InboxCollapsed = list
	s.mu.Unlock()
	s.save()
}

// save writes the store, whole: to a file beside it, then in its place,
// so that a save cut short, or another made at once, leaves no half of it.
func (s *store) save() {
	if s.path == "" {
		return
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.mu.Lock()
	data, err := json.Marshal(s.data)
	s.mu.Unlock()
	if err == nil {
		writeFileAtomic(s.path, data)
	}
}

// writeFileAtomic writes a file through a temporary one in its directory,
// renamed over it.
func writeFileAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(f.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}
