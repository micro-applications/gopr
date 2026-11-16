package main

import (
	"runtime"

	"github.com/egoist/godiff/internal/agent"
	"github.com/egoist/mygo"
)

// focused returns the review window with the focus, nil for none.
func focused(win *mygo.Window) *window {
	windowsMu.Lock()
	defer windowsMu.Unlock()
	for _, w := range windows {
		if w.win == win {
			return w
		}
	}
	return nil
}

// inWindow returns a menu item's action on the focused review window.
func inWindow(fn func(w *window)) func(*mygo.MenuItem, *mygo.Window) {
	return func(_ *mygo.MenuItem, win *mygo.Window) {
		if w := focused(win); w != nil {
			w.win.Update(func() { fn(w) })
		}
	}
}

var appMenu *mygo.Menu

// buildMenu makes the menu bar.
func buildMenu() *mygo.Menu {
	s := cfg.Get()
	setTheme := func(theme string) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) { cfg.Update(func(s *Settings) { s.Theme = theme }) }
	}
	appMenu = mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			mygo.Separator(),
			{Label: "Open Config File…", Accelerator: "CmdOrCtrl+,", Click: func(*mygo.MenuItem, *mygo.Window) { openConfig() }},
			{Label: "Install Command Line Tool…", Hidden: runtime.GOOS == "windows", Click: func(*mygo.MenuItem, *mygo.Window) { installCLI() }},
			mygo.Separator(),
			{Role: mygo.RoleServices},
			mygo.Separator(),
			{Role: mygo.RoleHide},
			{Role: mygo.RoleHideOthers},
			{Role: mygo.RoleUnhide},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Label: "File", Submenu: []*mygo.MenuItem{
			{Label: "Open Pull Request…", Accelerator: "CmdOrCtrl+O", Click: inWindow(func(w *window) { w.openDialog(dialogPull) })},
			mygo.Separator(),
			{Label: "Open File in Editor", Accelerator: "CmdOrCtrl+Shift+O", Click: inWindow(func(w *window) { w.openCurrent() })},
			mygo.Separator(),
			{Label: "Close", Accelerator: "CmdOrCtrl+W", Click: closeTabOrWindow},
		}},
		{Label: "Edit", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleUndo},
			{Role: mygo.RoleRedo},
			mygo.Separator(),
			{Role: mygo.RoleCut},
			{Role: mygo.RoleCopy},
			{Role: mygo.RolePaste},
			{Role: mygo.RoleSelectAll},
			mygo.Separator(),
			{Label: "Find in Diffs", Accelerator: "CmdOrCtrl+F", Click: inWindow(func(w *window) { *w.findOpen() = true })},
			{Label: "Filter Files", Accelerator: "CmdOrCtrl+P", Click: inWindow(func(w *window) { w.focusFilter() })},
			mygo.Separator(),
			{Label: "Copy Review Comments", Accelerator: "CmdOrCtrl+Shift+M", Click: inWindow(func(w *window) { w.copyComments() })},
		}},
		{Label: "View", Submenu: []*mygo.MenuItem{
			{Label: "Command Bar…", Accelerator: "CmdOrCtrl+K", Click: inWindow(func(w *window) { w.paletteOpen = !w.paletteOpen })},
			{Label: "Toggle Sidebar", Accelerator: "CmdOrCtrl+Shift+B", Click: inWindow(func(w *window) { w.toggleSidebar() })},
			{Label: "Focus Mode", Accelerator: "CmdOrCtrl+Shift+F", Click: inWindow(func(w *window) { w.toggleFocus() })},
			mygo.Separator(),
			{Label: "Review with AI", Accelerator: "CmdOrCtrl+Shift+I", Click: inWindow(func(w *window) { w.reviewWithAI() })},
			{Label: "Group Files", Submenu: []*mygo.MenuItem{
				{Label: "Not at All", Click: inWindow(func(w *window) { w.chooseGrouping(groupNone) })},
				{Label: "By Kind", Click: inWindow(func(w *window) { w.chooseGrouping(groupKind) })},
				{Label: "By AI Review", Click: inWindow(func(w *window) { w.chooseGrouping(groupAI) })},
			}},
			{Label: "AI Agent", Submenu: agentMenuItems(s)},
			mygo.Separator(),
			{Label: "Diff", Submenu: []*mygo.MenuItem{
				{ID: "split", Label: "Split", Type: mygo.MenuItemRadio, Checked: s.DiffStyle == "split", Click: func(*mygo.MenuItem, *mygo.Window) {
					cfg.Update(func(s *Settings) { s.DiffStyle = "split" })
				}},
				{ID: "unified", Label: "Unified", Type: mygo.MenuItemRadio, Checked: s.DiffStyle == "unified", Click: func(*mygo.MenuItem, *mygo.Window) {
					cfg.Update(func(s *Settings) { s.DiffStyle = "unified" })
				}},
				mygo.Separator(),
				{ID: "wrap", Label: "Word Wrap", Type: mygo.MenuItemCheckbox, Checked: s.WordWrap, Click: func(*mygo.MenuItem, *mygo.Window) { toggleWrap() }},
				{ID: "whitespace", Label: "Show Whitespace", Type: mygo.MenuItemCheckbox, Checked: s.ShowWhitespace, Click: func(*mygo.MenuItem, *mygo.Window) {
					cfg.Update(func(s *Settings) { s.ShowWhitespace = !s.ShowWhitespace })
				}},
				mygo.Separator(),
				{Label: "Collapse All Files", Click: inWindow(func(w *window) { w.setAllCollapsed(true) })},
				{Label: "Expand All Files", Click: inWindow(func(w *window) { w.setAllCollapsed(false) })},
			}},
			{Label: "Markdown Preview", Submenu: markdownWidthItems(s)},
			{Label: "Zoom", Submenu: []*mygo.MenuItem{
				{Label: "Zoom In", Accelerator: "CmdOrCtrl+=", Click: func(*mygo.MenuItem, *mygo.Window) { changeZoom(1) }},
				{Label: "Zoom Out", Accelerator: "CmdOrCtrl+-", Click: func(*mygo.MenuItem, *mygo.Window) { changeZoom(-1) }},
				{Label: "Actual Size", Accelerator: "CmdOrCtrl+0", Click: func(*mygo.MenuItem, *mygo.Window) { changeZoom(0) }},
			}},
			{Label: "Comments", Submenu: []*mygo.MenuItem{
				{ID: "copyOnClose", Label: "Copy Comments on Close", Type: mygo.MenuItemCheckbox, Checked: s.CopyCommentsOnClose, Click: func(*mygo.MenuItem, *mygo.Window) {
					cfg.Update(func(s *Settings) { s.CopyCommentsOnClose = !s.CopyCommentsOnClose })
				}},
			}},
			{Label: "Theme", Submenu: []*mygo.MenuItem{
				{ID: "theme-system", Label: "Match System", Type: mygo.MenuItemRadio, Checked: s.Theme == "system", Click: setTheme("system")},
				{ID: "theme-light", Label: "Light", Type: mygo.MenuItemRadio, Checked: s.Theme == "light", Click: setTheme("light")},
				{ID: "theme-dark", Label: "Dark", Type: mygo.MenuItemRadio, Checked: s.Theme == "dark", Click: setTheme("dark")},
			}},
			mygo.Separator(),
			{Label: "Refresh", Accelerator: "CmdOrCtrl+R", Click: inWindow(func(w *window) { w.refreshAll() })},
			mygo.Separator(),
			{Role: mygo.RoleToggleFullScreen},
		}},
		{Role: mygo.RoleWindowMenu, Submenu: windowMenuItems()},
		{Role: mygo.RoleHelp, Submenu: []*mygo.MenuItem{
			{Label: "Keyboard Shortcuts", Click: inWindow(func(w *window) { w.help = true })},
		}},
	})
	return appMenu
}

// closeTabOrWindow closes the file's tab shown, else the window.
func closeTabOrWindow(_ *mygo.MenuItem, win *mygo.Window) {
	w := focused(win)
	switch {
	case w != nil && w.activeTab > 0:
		w.win.Update(func() { w.closeFileTab(w.activeTab) })
	case win != nil:
		win.Close()
	}
}

// windowMenuItems are the items of the Window menu: MyGo's, apart from
// the Close it has off macOS, which the File menu's Close does.
func windowMenuItems() []*mygo.MenuItem {
	if runtime.GOOS == "darwin" {
		return nil
	}
	return []*mygo.MenuItem{{Role: mygo.RoleMinimize}}
}

// syncMenu shows the settings in the menu's check marks.
func syncMenu(s Settings) {
	if appMenu == nil {
		return
	}
	set := func(id string, on bool) {
		if it := appMenu.ItemByID(id); it != nil {
			it.SetChecked(on)
		}
	}
	set("split", s.DiffStyle == "split")
	set("unified", s.DiffStyle == "unified")
	set("wrap", s.WordWrap)
	for _, mw := range markdownWidths {
		set("mdwidth-"+mw.key, s.MarkdownWidth == mw.key)
	}
	set("whitespace", s.ShowWhitespace)
	set("copyOnClose", s.CopyCommentsOnClose)
	set("theme-system", s.Theme == "system")
	set("theme-light", s.Theme == "light")
	set("theme-dark", s.Theme == "dark")
	set("agent-", s.AIAgent == "")
	for _, a := range agent.All {
		set("agent-"+a.Name, s.AIAgent == a.Name)
	}
}

// applyTheme follows the theme of the settings.
func applyTheme(s Settings) {
	switch s.Theme {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

// agentMenuItems choose the agent that reviews with AI.
func agentMenuItems(s Settings) []*mygo.MenuItem {
	choose := func(name string) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			cfg.Update(func(s *Settings) {
				if s.AIAgent != name {
					s.AIAgent, s.AIModel = name, ""
				}
			})
		}
	}
	items := []*mygo.MenuItem{
		{ID: "agent-", Label: "Automatic", Type: mygo.MenuItemRadio, Checked: s.AIAgent == "", Click: choose("")},
		mygo.Separator(),
	}
	for _, a := range agent.All {
		items = append(items, &mygo.MenuItem{ID: "agent-" + a.Name, Label: a.Label, Type: mygo.MenuItemRadio, Checked: s.AIAgent == a.Name, Click: choose(a.Name)})
	}
	return items
}

// markdownWidthItems choose the width Markdown previews read at.
func markdownWidthItems(s Settings) []*mygo.MenuItem {
	var items []*mygo.MenuItem
	for _, mw := range markdownWidths {
		items = append(items, &mygo.MenuItem{ID: "mdwidth-" + mw.key, Label: mw.name, Type: mygo.MenuItemRadio, Checked: s.MarkdownWidth == mw.key,
			Click: func(*mygo.MenuItem, *mygo.Window) { setMarkdownWidth(mw.key) }})
	}
	return append(items, mygo.Separator(),
		&mygo.MenuItem{Label: "Bigger Text", Click: func(*mygo.MenuItem, *mygo.Window) { changeMarkdownFontSize(1) }},
		&mygo.MenuItem{Label: "Smaller Text", Click: func(*mygo.MenuItem, *mygo.Window) { changeMarkdownFontSize(-1) }},
		&mygo.MenuItem{Label: "Default Text Size", Click: func(*mygo.MenuItem, *mygo.Window) { changeMarkdownFontSize(0) }},
	)
}
