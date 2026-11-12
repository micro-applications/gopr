# Godiff as a GitHub PR reviewer — design

Date: 2026-11-12

## Intent

The user reviews GitHub pull requests in their org and wants every PR that
concerns them in one place, without first opening each repository in
Godiff. They review their own PRs first, then the PRs waiting on their
review. While reviewing, they want to read the whole file around a change,
and to read changed Markdown rendered.

Godiff already reviews a PR well (grouped files, threads, comments, review
submission, AI review), but only for the GitHub remote of the repository a
window has open (⌘3 lists that one repository's open PRs). This work adds a
cross-repository inbox on top of the existing review, plus two viewing
features.

## Three parts, in order

The parts are independent and can be planned and shipped separately:

1. **Full-file tab** (smallest, no dependencies).
2. **Markdown preview** (uses MyGo's `ui/markdown`).
3. **PR inbox** (largest; changes how a window relates to a repository).

Parts 1 and 2 apply to every source Godiff reviews (local changes, commits,
branches, PRs), not only PRs.

## Part 1 — Full-file tab

**User decision:** the full file opens in a separate tab, to see the context
around the changes.

- The diff surface gets a tab row. The first tab is always "Changes" (today's
  surface). Opening a full file adds a tab named after the file; tabs can be
  closed; "Changes" cannot.
- A file opens in a tab from its file card header (a button) and from the
  file tree (context menu or a shortcut). *Builder's call:* the shortcut
  and the exact affordance.
- The tab shows the whole file at the new side of the review: the PR's head
  commit for a PR, the commit for a commit, the work tree for local changes.
  It uses the existing syntax highlighting, code font, word wrap and
  whitespace settings, with line numbers.
- *My call:* changed lines are marked in the gutter (added/modified), so the
  reader can see where the diff sits in the file; deleted lines are not
  shown. Opening the tab from a hunk scrolls to that hunk's first line.
- *My call:* Find (⌘F) works inside the full-file tab. Commenting from the
  full-file tab is out of scope; comments stay in "Changes".
- Tabs belong to the current review; switching to another PR or source
  closes them.
- MyGo has `ui.Tabs`/`coreTabsBase` (`ui/tabs.go`) for the tab row.

## Part 2 — Markdown preview

**User decision:** a changed Markdown file shows the rendered new version,
with a toggle back to the diff.

- Applies to `.md`, `.markdown` and `.mdx` *(my call on the list)*.
- In "Changes", a Markdown file card shows the rendered new version by
  default, with a toggle in its header: **Preview / Diff**. *My call:* the
  choice is per file and remembered for the session, not saved.
- A deleted Markdown file shows the diff only (nothing to render).
- The full-file tab (Part 1) of a Markdown file has the same toggle,
  Preview / Source.
- Render with MyGo's `ui/markdown` (`markdown.View`): CommonMark plus its
  tables, code blocks (highlighted) and images. Relative image paths resolve
  against the file's directory at the reviewed commit, read from the local
  clone. Links open in the system browser; relative links to other files in
  the review *(builder's call)* may open them in a full-file tab.
- *My call:* while in Preview, J/K hunk navigation skips the file, and
  existing review threads on it remain reachable by switching to Diff (a
  count of threads shows on the toggle).
- *Out of scope:* rendering the PR description as Markdown (`plainBody`,
  `pullview.go:179`, shows it as text today). Cheap to add with the same
  renderer, but not asked for.

## Part 3 — PR inbox

### What the user sees

**User decisions:**
- One reviewer window with the inbox in the sidebar (mockup option B, a
  mail-app layout), not a separate inbox window.
- Two sections, in this order: **My pull requests**, then **Waiting on my
  review**.
- Only PRs opened in the last X days, with a menu offering **7 days** and
  **30 days**.
- PRs in repositories not cloned on this Mac are cloned into
  `~/Projects/<org>/` before they open.

Layout, reusing what exists: the inbox replaces the contents of the sidebar's
Pull Requests tab (⌘3). Today, choosing a PR there calls `setSource` and the
list stays visible while the review fills the main area, with the PR's file
tree on ⌘1. The inbox keeps that behavior; the difference is that its rows
come from many repositories, so choosing one may switch the window's
repository.

Each row shows: title, `owner/repo #number`, author (in "Waiting on my
review"), age, draft state, review decision and checks — the fields the
existing `pullsView` row already draws. The repo name is new.

At the top of the inbox: the period menu (Last 7 days / Last 30 days), a
refresh button, and the existing filter field.

### My calls (the user didn't object, but didn't confirm)

- "My pull requests" = open PRs authored by the signed-in user. "Waiting on
  my review" = open PRs where the user's review is requested, directly or
  through a team (`review-requested:@me`, which includes team requests).
  PRs the user already reviewed drop out once the request is satisfied, as
  GitHub's search does.
- Drafts show in "My pull requests" and are left out of "Waiting on my
  review".
- The period filters on **created** date, applies to both sections, defaults
  to **7 days**, and the choice is saved in the store (`~/.godiff`) so it
  survives restarts. It is also a setting, `inboxDays` (7 or 30), in
  `godiff.jsonc`.
- The local mode stays: ⌘1 Files and ⌘2 History still review the window's
  current repository (uncommitted changes, commits, branches, committing).
- Refresh: on window focus if older than 5 minutes, on the refresh button,
  and on ⌘R while the inbox tab is showing. No polling timer.

### Data

- One GraphQL search per section, reusing `Client.graphql` and the field set
  of `openPullsQuery` (`internal/github/pulls.go`), plus
  `repository { nameWithOwner }` so each row knows its repo:
  - `is:pr is:open author:@me org:<org> created:>=<date>`
  - `is:pr is:open review-requested:@me draft:false org:<org> created:>=<date>`
- `@me` needs a token. Signed out, the inbox shows an empty state saying to
  run `gh auth login` or set `githubToken`, and nothing else. (Public-repo
  reads without a token keep working elsewhere.)
- Search results are capped at 100 per section *(my call)*; with the 7/30-day
  window this is enough.

### One window, many repositories (the main constraint)

Today a `window` is bound to one `*git.Repo` (`window.go:53`, `openWindow`
at `window.go:206`), and "every repository opens in a window of its own"
(README). State keyed by repository root includes the viewed-files store
(`viewedKey`), recents, the file watcher, the GitHub remote (`w.gh`) and
history. Option B means choosing an inbox row from another repository
**rebinds the window to that repository**: reopen `git.Repo`, recompute
`w.gh`, restart watchers, reset history and the file tree, then
`setSource(pullSource(...))`. How to structure that (a `switchRepo` on
`window`, or splitting repo-bound state into its own struct) is the
builder's call.

*My call:* the window's title and ⌘1/⌘2 follow the repository of the PR
being reviewed. Opening a repository from the command line or File → Open
still opens its own window, as today.

### Finding and cloning the repository

Clones are not reliably at `~/Projects/<owner>/<name>` (this repo itself is
`~/Projects/OpenSource/godiff` for `egoist/godiff`). So:

1. Look for an existing clone whose remote matches `owner/name`, using
   `findGitHubRemote` with a non-zero `want`, in this order: the windows
   already open, `state.recent()`, then a scan of `~/Projects` two levels
   deep (`~/Projects/*/*`). Cache the mapping `owner/name → path` in the
   store; verify it still matches before use.
2. Only if none is found, clone into `~/Projects/<owner>/<name>` with `git
   clone` (the user's git, so their SSH keys/credential helper and `gh`'s
   protocol apply). Show progress in the main area ("Cloning acme/api…");
   failure shows the error with Try Again.
3. Then fetch the PR as Godiff does today (`fetchPull`).

The base folder `~/Projects` is a setting, `cloneRoot`, defaulting to
`~/Projects` *(my call)*.

## Non-goals

- Closed or merged PRs in the inbox.
- Notifications, badges, or background polling.
- Creating PRs, merging, or editing PR metadata from Godiff.
- Commenting from the full-file tab or the Markdown preview.
- Windows/Linux-specific work beyond what MyGo already gives.

## Anti-goals

- The inbox must not slow down opening a single repository's local review,
  which is how the app is used today.
- Must not clone a repository the user already has elsewhere (re-cloning
  would split their work across two copies).
- Must not silently drop PRs: if a search fails or is capped, say so in the
  inbox.

## Open questions for the user

- Which org (or orgs) the inbox covers, and whether "my GitHub accounts"
  means more than one signed-in account. *Pending.*

## Left to the builder

Code structure for repo switching, the tab state model, shortcuts, exact
copy and icons, test layout (the project tests views without a window; keep
to that), and how to fake GitHub search in tests (extend `fakeGitHub` in
`internal/github/github_test.go`).
