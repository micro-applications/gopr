# Godiff as a GitHub PR reviewer — design

Date: 2026-11-12

## Intent

The user reviews GitHub pull requests in their orgs and wants every PR that
concerns them in one place, without first opening each repository. They
review their own PRs first, then the PRs waiting on their review. While
reviewing, they want to read the whole file around a change, and to read
changed Markdown rendered.

Godiff already reviews a PR well (grouped files, review threads, comments,
review submission, AI review), but only for the GitHub remote of the
repository a window has open, and each repository gets its own window. This
work turns Godiff into a **single-window PR reviewer**: an inbox of PRs in
the sidebar, and a PR workspace on the right. The user said explicitly that
the current behavior does not need to be kept.

## Three parts, in order

1. **Single window: PR inbox + PR workspace.** The new app shell; everything
   else lives inside it.
2. **Full-file tabs** in the PR workspace.
3. **Markdown preview** in the PR workspace.

Parts 2 and 3 do not depend on the inbox's data and can be built in either
order once the workspace exists. If it helps, they can be built first on
today's PR review surface and moved into the workspace with Part 1.

## Part 1 — Single window: inbox and PR workspace

### What the user sees

**User decisions:**
- **One window.** Not one window per repository.
- **Sidebar = PR inbox**, collapsible. Two sections, in this order:
  **My pull requests**, then **Waiting on my review**.
- **Click a PR → its PR workspace opens on the right**, showing the diff and
  file-view tabs.
- **Time filter:** only PRs opened in the last X days, chosen from a menu
  with **7 days** and **30 days**.
- **Org filter:** the user can choose which orgs the inbox covers.
- **Missing clones** are cloned into `~/Projects/<org>/` before the PR
  opens.
- The org's PRs come from **branches in the org's repositories**, not from
  forks.

Layout:

```
┌──────────────── Godiff ────────────────────────────────────────────┐
│ [Orgs ▾] [7 days ▾] ⟳ │ acme/api #412 · Add rate limiter  (header) │
│ filter…               │ ┌ Changes ┬ limiter.go ┬ README.md ┐       │
│ ▾ My pull requests  3 │ │ file tree │ diff surface             │  │
│   Add rate limiter    │ │ (PR files)│                          │  │
│   acme/api #412       │ │           │                          │  │
│ ▾ Waiting on review 2 │ │           │                          │  │
│   Migrate to PG 17    │ └───────────┴──────────────────────────┘  │
│   acme/infra #31 @dana│                                            │
└────────────────────────────────────────────────────────────────────┘
```

- **Sidebar (inbox).** Toggles with ⌘⇧B as today. Each section header
  collapses its section *(my call: "collapsible" covers both the sidebar and
  the sections)*. Rows show title, `owner/repo #number`, author (in "Waiting
  on my review"), age, draft state, review decision and checks — the fields
  the existing `pullsView` row draws (`pullview.go:18`), plus the repo name.
  At the top: the org menu, the period menu, refresh, and the filter field.
  ↑/↓ move through the rows across both sections.
- **PR workspace (right).** The existing PR review: header (`pullHeader`),
  grouped files, threads, comments, review dialog, AI review, "new commits
  pushed" banner. Above the surface, the tab row of Part 2 ("Changes" plus
  full-file tabs). *My call:* the PR's file tree (today's ⌘1 tree, with its
  groups and AI-note marks) moves inside the workspace as its own
  collapsible pane on the workspace's left, since the window sidebar now
  holds the inbox.
- **Nothing selected:** the workspace shows an empty state ("Choose a pull
  request"). Signed out: the whole window shows how to sign in (`gh auth
  login`, or `githubToken` / `$GH_TOKEN`).

### My calls (check these)

- **Local mode is removed from the UI:** reviewing uncommitted changes,
  History (⌘2), comparing branches, and committing go away, as does one
  window per repository, File → Open/Open Recent, and the welcome window. The
  user said the current behavior need not be kept; this is the reading that
  matches "a single window with PRs in the sidebar." The code under
  `internal/` (git, diff, highlight, agent, github) stays.
- **Command line:** `godiff`, with no argument, opens or focuses the window.
  `godiff owner/repo#123` and `godiff https://github.com/owner/repo/pull/123`
  open that PR's workspace (it need not be in the inbox). Other arguments
  (commits, branches, paths) are dropped, with a message.
- "My pull requests" = open PRs authored by the signed-in user. "Waiting on
  my review" = open, non-draft PRs where the user's review is requested,
  directly or through a team (`review-requested:@me`). PRs the user already
  reviewed drop out as GitHub's search drops them.
- Drafts show in "My pull requests" only.
- The period filters on **created** date, applies to both sections, defaults
  to **7 days**.
- **Orgs:** the org menu lists the orgs the signed-in user belongs to
  (GraphQL `viewer.organizations`), as checkboxes; the user's own account
  (personal repos) is listed too. Default: all checked. The choice is saved.
- One GitHub account, the one Godiff already finds (`githubToken()`): there
  is no multi-account support.
- Period, orgs, section collapse state, and sidebar width are saved in the
  store (`~/.godiff`) and survive restarts.
- Refresh: on window focus if older than 5 minutes, on the refresh button,
  and on ⌘R. No polling timer.
- Viewed-file state stays keyed per repository and PR (`viewedKey`), so it
  carries over from today.

### Data

- One GraphQL search per section, reusing `Client.graphql` and the field set
  of `openPullsQuery` (`internal/github/pulls.go`), plus
  `repository { nameWithOwner }`:
  - `is:pr is:open author:@me <org filter> created:>=<date>`
  - `is:pr is:open draft:false review-requested:@me <org filter> created:>=<date>`
  - `<org filter>` is `org:a org:b …` (GitHub ORs repeated `org:`) plus
    `user:<login>` when personal repos are checked. With everything checked,
    omit the filter.
- Up to 100 results per section *(my call)*. If a search fails or is capped,
  the section says so; PRs are never dropped silently.

### Finding and cloning the repository

PRs come from branches in the org's repositories, so a clone counts when one
of its remotes matches the PR's base `owner/name`.

1. Look for an existing clone with `findGitHubRemote(repo, want)`, in this
   order: a saved mapping `owner/name → path` (verified before use),
   `state.recent()`, then a scan of `~/Projects/*/*`. Save what is found.
   Clones are not reliably at `~/Projects/<owner>/<name>` (this repo is at
   `~/Projects/OpenSource/godiff`), so the scan matters.
2. Only if none is found, run `git clone` into `~/Projects/<owner>/<name>`
   with the user's git, so their SSH keys, credential helper and `gh`'s
   protocol apply. The workspace shows "Cloning acme/api…"; a failure shows
   the error with Try Again.
3. Fetch the PR as Godiff does today (`fetchPull`, `pull.go:184`).

The base folder is a setting, `cloneRoot`, defaulting to `~/Projects`
*(my call)*.

### Structure (constraint for the builder)

Today a `window` (`window.go:53`) is bound to one `*git.Repo` from
`openWindow` (`window.go:206`), and state hangs off it: the file watcher,
`w.gh`, history, the viewed map, the file tree. In the new shell, the window
outlives repositories: choosing a PR rebinds the workspace to that PR's
repository. Splitting repo/PR-bound state into a workspace struct that is
rebuilt per PR is the obvious shape; how is the builder's call. Drop the file
watcher on the work tree: a PR review reads commits, not the work tree.

## Part 2 — Full-file tabs

**User decision:** a full file opens in its own tab in the PR workspace, to
see the context around the changes.

- The workspace has a tab row. The first tab, "Changes", is the diff
  surface and can't be closed. Each opened file adds a closable tab named
  after the file.
- A file opens in a tab from its file card header (a button) and from the
  workspace's file tree. *Builder's call:* the shortcut and exact
  affordances.
- The tab shows the whole file at the PR's head commit, with line numbers,
  syntax highlighting, the code font, word wrap and whitespace settings.
- *My call:* changed lines are marked in the gutter; deleted lines are not
  shown. Opening from a hunk scrolls to that hunk's first line. ⌘F works in
  the tab. Commenting stays in "Changes".
- Tabs belong to the PR; choosing another PR closes them. *My call:*
  returning to a PR later does not restore its tabs.
- MyGo provides `ui.Tabs` / `coreTabsBase` (`ui/tabs.go`).

## Part 3 — Markdown preview

**User decision:** a changed Markdown file shows its rendered new version,
with a toggle to the diff.

- Applies to `.md`, `.markdown` and `.mdx` *(my call)*.
- In "Changes", a Markdown file card shows the rendered new version by
  default, with a **Preview / Diff** toggle in its header. *My call:* the
  choice is per file, kept while the PR is open, not saved.
- A deleted Markdown file shows the diff only.
- A Markdown file's full-file tab has a **Preview / Source** toggle.
- Render with MyGo's `ui/markdown` (`markdown.View`): CommonMark, tables,
  highlighted code blocks, images. Relative images resolve against the
  file's directory at the PR's head commit, read from the clone. Links open
  in the system browser; relative links to files in the PR may open a
  full-file tab *(builder's call)*.
- *My call:* in Preview, J/K hunk navigation skips the file; the toggle
  shows a count of review threads on the file, which are reachable in Diff.
- *Out of scope:* rendering the PR description as Markdown (`plainBody`,
  `pullview.go:179`). Cheap with the same renderer; not asked for.

## Non-goals

- Closed or merged PRs in the inbox.
- Multiple GitHub accounts.
- PRs from forks (the org uses branches).
- Notifications, badges, background polling.
- Creating, merging, or editing PR metadata.
- Commenting from a full-file tab or the Markdown preview.
- Local-changes review, history, and committing (removed; see my calls).

## Anti-goals

- Must not clone a repository the user already has elsewhere.
- Must not silently drop PRs: failed or capped searches say so.
- Must not touch the user's work tree in a clone beyond what `fetchPull`
  does today (fetching refs). No checkout, no reset.

## Left to the builder

The workspace/state split, the tab state model, shortcuts, copy and icons,
test layout (the project tests views without a window; keep to that), faking
GitHub search and `viewer.organizations` in tests (extend `fakeGitHub` in
`internal/github/github_test.go`), and updating the README to the new app.
