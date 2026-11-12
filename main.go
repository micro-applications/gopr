// GoPR is a native desktop app for reviewing GitHub pull requests, after
// codiff, drawn by MyGo.
//
//	gopr                      open GoPR
//	gopr owner/repo#123       open a pull request in it, or its URL
package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/egoist/godiff/internal/github"
	"github.com/egoist/mygo"
)

const usage = `Usage: gopr [<pull request>]

Open GoPR, or open a pull request in it, named as owner/repo#123 or by
its URL.

Options:
  -h, --help   show this help
`

// request is what a command line asks to open: the pull request, or just
// the app for the zero value.
type request struct {
	repo   github.Repo
	number int
}

var errHelp = fmt.Errorf("help")

// parseArgs reads a command line: nothing, or a pull request.
func parseArgs(args []string) (request, error) {
	var req request
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			return request{}, errHelp
		case strings.HasPrefix(a, "-psn_"), a == "":
			// What macOS passes to apps opened from Finder.
		default:
			repo, n, ok := github.ParsePull(a)
			if !ok || repo == (github.Repo{}) || req.number != 0 {
				return request{}, fmt.Errorf("gopr opens pull requests: use owner/repo#123 or its URL")
			}
			req = request{repo: repo, number: n}
		}
	}
	return req, nil
}

// open shows the window, and the pull request a command line names in it.
func open(req request) {
	w := openApp()
	if req.number > 0 {
		w.win.Update(func() { w.openPull(req.repo, req.number) })
	}
}

func main() {
	req, err := parseArgs(os.Args[1:])
	if err == errHelp {
		fmt.Print(usage)
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	name := "GoPR"
	if n := os.Getenv("GODIFF_NAME"); n != "" {
		// Another name runs apart from the installed app, for testing.
		name = n
	}
	mygo.App.SetName(name)
	if !mygo.App.RequestSingleInstanceLock() {
		return // the running instance opens the window
	}
	mygo.App.OnSecondInstance(func(args []string, workingDir string) {
		if len(args) > 0 {
			args = args[1:]
		}
		req, err := parseArgs(args)
		if err != nil {
			go mygo.Dialog.Error("GoPR", err.Error())
			return
		}
		go open(req)
	})
	cfg.migrateConfig()
	state.open()
	applyTheme(cfg.Get())
	mygo.App.SetMenu(buildMenu())
	cfg.OnChange(func(s Settings) {
		mygo.RunOnMain(func() {
			syncMenu(s)
			applyTheme(s)
		})
	})
	mygo.App.OnWindowAllClosed(func() {
		if runtime.GOOS != "darwin" {
			mygo.App.Quit()
		}
	})
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		// A click on the Dock icon with no window open opens it again.
		if !hasVisibleWindows {
			go openApp()
		}
	})
	if path := os.Getenv("GODIFF_CPUPROFILE"); path != "" {
		// The whole session, written as the app quits.
		if f, err := os.Create(path); err == nil {
			pprof.StartCPUProfile(f)
			mygo.App.OnQuit(func() {
				pprof.StopCPUProfile()
				f.Close()
			})
		}
	}
	if path := os.Getenv("GODIFF_STARTUP_PROFILE"); path != "" {
		// The first seconds, which draw the window for the first time.
		if f, err := os.Create(path); err == nil {
			pprof.StartCPUProfile(f)
			go func() {
				time.Sleep(3 * time.Second)
				pprof.StopCPUProfile()
				f.Close()
			}()
		}
	}
	fetchAvatars, detectAgents = true, true
	mygo.App.WhenReady(func() {
		if debugFrames {
			go watchMainThread()
		}
		cfg.watch()
		go open(req)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
