package main

import (
	"log"
	"os"
	"runtime/pprof"
	"time"

	"github.com/egoist/mygo"
)

// captureIfAsked writes a PNG of the window to $GODIFF_CAPTURE once it has
// loaded, then quits: for checking how the app looks from scripts.
func (w *window) captureIfAsked() {
	path := os.Getenv("GODIFF_CAPTURE")
	if path == "" {
		return
	}
	go func() {
		time.Sleep(2500 * time.Millisecond)
		// A pull request loads from GitHub, which takes its time.
		w.waitFor(time.Minute, func() bool { return !w.loading })
		if setup := os.Getenv("GODIFF_CAPTURE_SETUP"); setup != "" {
			var prof *os.File
			if path := os.Getenv("GODIFF_CPUPROFILE"); path != "" {
				prof, _ = os.Create(path)
				pprof.StartCPUProfile(prof)
			}
			w.win.Update(func() { w.debugSetup(setup) })
			time.Sleep(1200 * time.Millisecond)
			if setup == "ai" {
				w.waitFor(10*time.Minute, func() bool {
					st := w.analyses[w.source]
					return st == nil || !st.running
				})
				time.Sleep(500 * time.Millisecond)
			}
			if prof != nil {
				pprof.StopCPUProfile()
				prof.Close()
			}
		}
		png, err := w.win.CapturePage()
		if err != nil {
			log.Println("capture:", err)
		} else if err := os.WriteFile(path, png, 0o644); err != nil {
			log.Println("capture:", err)
		}
		mygo.App.Quit()
	}()
}

// debugSetup puts the window in a state to capture: none without a
// repository.
func (w *window) debugSetup(setup string) {
	if w.repo == nil {
		return
	}
	switch setup {
	case "unified":
		w.settings.DiffStyle = "unified"
		w.rowsDirty = true
	case "palette":
		w.paletteOpen = true
	case "find":
		w.finding = true
		w.query = "grip"
	case "comment":
		w.nextHunk(1)
		w.commentOnSelection()
		for _, c := range w.comments {
			c.text = "Should the grip be wider on vertical splits too?"
		}
	case "wrap":
		w.settings.WordWrap = true
		w.rowsDirty = true
	case "ai":
		w.runAnalysis()
	}
}

// waitFor waits until a condition of the window holds, or a while.
func (w *window) waitFor(limit time.Duration, done func() bool) {
	for end := time.Now().Add(limit); time.Now().Before(end); time.Sleep(200 * time.Millisecond) {
		var ok bool
		mygo.RunOnMain(func() { ok = done() })
		if ok {
			return
		}
	}
}
