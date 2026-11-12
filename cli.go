package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
)

// cliName is the command that opens the app.
const cliName = "gopr"

// appBundle returns the .app holding the running executable, "" outside
// one.
func appBundle() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	for dir := filepath.Dir(exe); dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if strings.HasSuffix(dir, ".app") {
			return dir
		}
	}
	return ""
}

// cliScript is the gopr command: it opens the app, or a pull request in
// it.
func cliScript() string {
	if bundle := appBundle(); bundle != "" && runtime.GOOS == "darwin" {
		return fmt.Sprintf("#!/bin/sh\n# Opens GoPR, or a pull request in it.\nexec open -n -a %q --args \"$@\"\n", bundle)
	}
	exe, _ := os.Executable()
	return fmt.Sprintf("#!/bin/sh\n# Opens GoPR, or a pull request in it.\n%q \"$@\" >/dev/null 2>&1 &\n", exe)
}

// installCLI writes the gopr command into a directory of the PATH.
func installCLI() {
	go func() {
		home, _ := os.UserHomeDir()
		dirs := []string{"/usr/local/bin", "/opt/homebrew/bin", filepath.Join(home, ".local", "bin")}
		var errs []string
		for _, dir := range dirs {
			if _, err := os.Stat(dir); err != nil && dir != dirs[len(dirs)-1] {
				continue
			}
			os.MkdirAll(dir, 0o755)
			path := filepath.Join(dir, cliName)
			if err := os.WriteFile(path, []byte(cliScript()), 0o755); err != nil {
				errs = append(errs, err.Error())
				continue
			}
			os.Chmod(path, 0o755)
			mygo.Dialog.Message(mygo.MessageOptions{
				Type:    mygo.MessageInfo,
				Message: "The " + cliName + " command is installed",
				Detail:  "It is at " + path + ". Run gopr to open GoPR, or gopr owner/repo#123 to open a pull request.",
			})
			return
		}
		mygo.Dialog.Error("Could not install the gopr command", strings.Join(errs, "\n"))
	}()
}
