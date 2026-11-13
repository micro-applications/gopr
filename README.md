# GoPR

A native desktop app for reviewing GitHub pull requests, built from
[Godiff](https://github.com/egoist/godiff), a reimplementation of
[codiff](https://github.com/nkzw-tech/codiff) in Go with
[MyGo](https://mygo.egoist.dev/)'s native UI. No webview, no JavaScript: the
window is drawn by MyGo on the GPU, with the system's fonts, accent color,
dark mode, menus and vibrancy.

GoPR has one window. It starts with your pull requests in the sidebar and
needs no repository open: choosing a pull request finds its repository on
your disk, or clones it, and shows the pull request there.

## Features

- **Inbox**: the sidebar lists "My pull requests" and "Waiting on my review",
  from GitHub's search. A menu limits them to those created in the last 7 or
  30 days, and another to the organizations you belong to, or your own
  account. Each section shows at most 100 pull requests and says so when it
  is capped or fails. The inbox refreshes when the window gets the focus after
  five minutes, on the refresh button and on <kbd>⌘R</kbd>.
- **Workspace**: opening a pull request fetches its commits into your clone
  of the repository (cloned to `<cloneRoot>/<owner>/<name>` when you have
  none). The files pane lists the changed files, grouped by kind, each group
  with how much of it you viewed. GoPR never checks out, resets or otherwise
  touches a clone's work tree.
- **Split and unified diffs** with syntax highlighting (codiff's Licht and
  Dunkel palettes), word-level change highlighting, and unchanged lines that
  expand 100 at a time or all at once. Pictures show as previews.
- **Full-file tabs**: double-click a file in the files pane (or use its
  card's button, its context menu, or <kbd>F</kbd> on a hunk) to read its
  whole contents in a tab beside the diff, and close the tab when done.
- **PR descriptions, reviews and review comments** read as Markdown, in
  the previews' type: headings, lists, code and links (which open in the
  browser); a long description shows its start, faded, until **Show more**.
- **Markdown preview** of `.md`, `.markdown` and `.mdx` files in their tabs
  (Changes always shows their diff; a card's **Preview** button opens the
  tab). Previews are centered at a readable width — Narrow, Medium (the
  default), Wide or Full Width — and their text can be made bigger or
  smaller, from the icon buttons in the tab's header, **View → Markdown
  Preview** or the command bar.
- **Mermaid diagrams** in Markdown previews: fenced `mermaid` blocks draw
  as diagrams — all 33 of Mermaid's types — laid out by
  [pele-go](https://github.com/Broderick-Westrope/pele-go) and drawn
  natively, in the app's fonts and light or dark colors. A block that does
  not parse shows its source and the error. Click a diagram to open it over
  the window — it grows from its place in the preview, and goes back there
  as it closes — fitted to it: zoom from 25% to 600% with the toolbar or
  <kbd>+</kbd>, <kbd>-</kbd>, <kbd>0</kbd> (actual size) and <kbd>F</kbd>
  (fit), pan by dragging or scrolling, and close it with <kbd>Esc</kbd>.
- **Viewed files**: marking a file viewed collapses it, and it opens again
  once it changes. Generated files (lockfiles, `*.min.js`,
  `linguist-generated`, …) and dependency folders start collapsed.
- **Review comments and threads**: click a line, or press <kbd>J</kbd>/<kbd>K</kbd>
  to choose a hunk and <kbd>Enter</kbd>. Review threads show under their lines,
  to reply to and resolve; comments go out alone or into your review, which you
  submit as a comment, an approval or a request for changes. A banner says
  when new commits are pushed.
- **AI review**: a coding agent installed on your machine (Claude Code, Codex,
  OpenCode or Pi) reviews the pull request: it summarizes it, groups the files
  by intent, and notes the lines that deserve attention. It runs read-only,
  with the agent's own sign-in, and its review is kept until you ask for
  another. Press <kbd>N</kbd> and <kbd>⇧N</kbd> to go from note to note.
- **Find in diffs** (<kbd>⌘F</kbd>), a **file filter** (<kbd>⌘P</kbd>) and a
  **command bar** (<kbd>⌘K</kbd>).

## Usage

```sh
gopr                       # open GoPR
gopr owner/repo#123        # open a pull request in it
gopr https://github.com/owner/repo/pull/123
```

`gopr` opens the app, or brings the running one to the front, and opens the
pull request in its window. **GoPR → Install Command Line Tool…** installs
the `gopr` command. Press <kbd>⌘O</kbd> in the app to open a pull request
from its name or URL.

### Keyboard

| Keys | |
|---|---|
| <kbd>⌘O</kbd> | Open a pull request |
| <kbd>⌘⇧E</kbd> | Toggle the files pane |
| <kbd>⌘⇧B</kbd> | Toggle the sidebar |
| <kbd>⌘⇧F</kbd> | Focus mode: hide the sidebar and files pane, and center the review |
| <kbd>⌘R</kbd> | Refresh |
| <kbd>⌘F</kbd> | Find in diffs |
| <kbd>⌘P</kbd> | Filter files |
| <kbd>⌘K</kbd> | Command bar |
| <kbd>F</kbd> | View full file |
| <kbd>⌘W</kbd> | Close file tab |
| <kbd>J</kbd> / <kbd>K</kbd> | Next / previous hunk |
| <kbd>N</kbd> / <kbd>⇧N</kbd> | Next / previous AI note |
| <kbd>Enter</kbd> | Comment on the hunk chosen |
| <kbd>⌘↩</kbd> | Add the comment |
| <kbd>⌘⇧I</kbd> | Review with AI |
| <kbd>⌥Z</kbd> | Toggle word wrap |
| <kbd>⌘⇧O</kbd> | Open the file in your editor |
| <kbd>⌘+</kbd> / <kbd>⌘-</kbd> / <kbd>⌘0</kbd> | Zoom the window: text, code, previews and room alike |
| <kbd>⇧?</kbd> | All the shortcuts |

## Configuration

Settings live in `~/.gopr/gopr.jsonc` (**GoPR → Open Config File…**),
with codiff's names, and apply to open windows as the file changes:

```jsonc
{
  "settings": {
    "codeFontFamily": "",          // e.g. "JetBrains Mono"; empty is SF Mono
    "uiFontFamily": "",            // the window's text, e.g. "Inter"; empty is the system's
    "markdownFontFamily": "",      // Markdown's text: previews, descriptions, reviews, comments and diagrams; empty is the window's
    "cloneRoot": "~/Projects",     // where pull requests' repositories are cloned: <cloneRoot>/<owner>/<name>
    "codeFontSize": 13,
    "copyCommentsOnClose": false,
    "diffStyle": "split",          // or "unified"
    "editorCommand": "",           // e.g. "zed {file}:{line}"
    "githubToken": "",             // empty uses the GitHub CLI's (gh auth login)
    "aiAgent": "",                 // claude, codex, opencode or pi; empty is the first installed
    "aiModel": "",                 // e.g. "sonnet" or "deepseek/deepseek-flash"; empty is the agent's default
    "markdownWidth": "medium",     // Markdown previews at most 640, 820 or 1080 points wide: "narrow", "medium", "wide", or "full"
    "markdownFontSize": 14,        // the size of Markdown previews' text, 11 to 28
    "reviewCommentsPrefix": "# Address these Review Comments",
    "sidebarPosition": "left",     // or "right"
    "showWhitespace": false,
    "theme": "system",             // or "light", "dark"
    "wordWrap": false,
    "zoom": 1                      // how large everything shows: ⌘+ and ⌘- step it, 0.5 to 3
  }
}
```

Files open in `$GODIFF_EDITOR` or `editorCommand` (`{file}`, `{line}` and
`{repo}` are replaced), else VS Code, else the app the system opens them with.

GoPR finds a pull request's repository under `cloneRoot` (and among the
repositories opened before) and clones it to `<cloneRoot>/<owner>/<name>` when
it is not there. It only fetches into a clone: it never checks out or resets
its work tree. The first time it runs, it copies `~/.godiff/godiff.jsonc` to
`~/.gopr/gopr.jsonc` if the new file does not exist.

Pull requests need no token to be read when they are public; to comment
and review, or to read private ones, sign in with the GitHub CLI (`gh auth
login`), or set `githubToken` or `$GH_TOKEN`.

## Development

```sh
go tool mygo dev          # the app, rebuilt as you edit
go test ./...             # including the views, run without a window
go tool mygo build        # GoPR.app and a disk image in build/
scripts/install-app.sh    # build GoPR.app and install it in Applications
go run ./tools/genicon    # render resources/icon.svg to the app icon
go run ./tools/genfileicons <@pierre/trees package>  # update the file icons
```

`GODIFF_SNAPSHOTS=<dir> go test .` saves PNGs of the views the tests drive, and
`GODIFF_CAPTURE=<file.png>` makes the app save a picture of its window once
loaded, then quit. `GODIFF_DEBUG=1` logs every git command with its duration, the
frames that take more than 4ms to build, and whenever the main thread keeps
work waiting over 30ms. `GODIFF_NAME=<name>` runs a build apart from the
installed app, which would otherwise take its windows, and
`GODIFF_STARTUP_PROFILE=<file>` writes a CPU profile of the first seconds.

`scripts/install-app.sh` builds the app without the disk image and copies
`GoPR.app` into `/Applications`, or into `~/Applications` when that is not
writable; set `INSTALL_DIR` to choose another folder. It replaces a copy
already there, and refuses to run while GoPR is open.

The code is in a few parts:

- `internal/git`: the repository through the `git` command: diffs, commits,
  file contents (`git cat-file --batch`).
- `internal/github`: the GitHub API: pull requests, reviews, threads, and the
  inbox's searches.
- `internal/diff`: parsing patches, and word-level differences.
- `internal/highlight`: syntax highlighting with Chroma.
- The `main` package: the window and its views (`view.go`, `sidebar.go`,
  `inbox.go`, `workspace.go`, `diffview.go`, `palette.go`), the rows of the
  diff surface (`rows.go`), clones, comments, find, settings and menus.
