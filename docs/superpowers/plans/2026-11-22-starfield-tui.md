# Starfield — Claude Code Session Manager TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> On approval, copy this file to `/Users/karansinghasane/Developer/starfield/docs/superpowers/plans/2026-11-22-starfield-tui.md`.

**Goal:** A Bubble Tea TUI (`starfield`) that shows every Claude Code session (running and stopped, across all directories) as cards. You can attach to one with full terminal fidelity, detach back home with a hotkey, resume stopped ones and start new ones from a fuzzy directory picker.

**Architecture:** Every session the TUI manages runs `caffeinate -is claude …` inside a detached tmux session on a private socket (`tmux -L starfield`). Pressing Enter suspends the TUI with `tea.ExecProcess(tmux attach)`, and the detach key brings you back. Discovery merges three sources:
- Claude history (`~/.claude/projects/*/*.jsonl`), for stopped sessions.
- The live registry (`~/.claude/sessions/<pid>.json`), for running sessions and busy/idle status.
- tmux panes plus the process tree, which tell sessions the TUI manages apart from ones running in other terminals ("external").

The UI copies stardocker's pattern: independent `tea.Model` screens in `src/`, the `gridView` card grid, the palette and the starfield animation.

**Tech Stack:** Go 1.24, bubbletea v1.3.10, bubbles v0.21.0, lipgloss v1.1.0, charmbracelet/x/ansi, muesli/reflow, and the tmux CLI (≥3.3). There is no Docker dependency.

**Spec:** this document's "Spec" section. It comes from the user's request and their answers in planning. The reference implementation is `/Users/karansinghasane/Developer/other/stardocker`, especially its `CLAUDE.md`.

## Spec

1. Run `starfield` from any directory. The home screen shows Claude sessions from every directory as cards, both running and stopped.
2. `a` toggles between all sessions and running only. The default is all.
3. Each card shows: project (the basename of cwd), title (custom title, else registry name, else last prompt), path and git branch, last prompt, model, cost, message count, and state with uptime or last-active time.
4. Arrow keys select a card and `enter` goes into the session. Inside, it behaves like a normal Ghostty terminal: shift+enter, colors, mouse scroll and clipboard all work. The detach key (default `C-]`) returns to the home screen, and the session keeps running.
5. `enter` on a stopped session resumes it (`claude --resume <id>`) in a new managed tmux session and attaches.
6. Sessions running in other terminals show as **external** cards (read-only). Enter on one shows a status message and does not attach.
7. `n` opens the new-session picker. It lists recent Claude dirs and dirs scanned under the roots (git repos, depth 2), and accepts any path you type. Typing fuzzy-filters the list, and `enter` starts `caffeinate -is claude --session-id <uuid>` there and attaches.
8. The design should look modern and nerdy: Claude coral accent, Nerd-font-free glyphs (●◐○◌✦⎇), starfield animation in the empty state, and a tmux status bar themed to match.
9. Quitting the TUI (`q`) leaves managed sessions running. `x` stops a managed session after a confirmation.

## Global Constraints

- Module `github.com/stardust1420/starfield`, binary `starfield`, Go 1.24. Repo root: `/Users/karansinghasane/Developer/starfield`.
- All tmux calls use `-L starfield -f <conf>`. Never touch the user's default tmux server.
- Managed tmux session names are `sf-` + the first 8 characters of the Claude session UUID. The full UUID goes in the tmux session option `@sf_session` and the dir in `@sf_dir`.
- Default launch command: `caffeinate -is claude`. Override it with `STARFIELD_CLAUDE_CMD`, which is split on whitespace.
- Env config: `STARFIELD_ROOTS` (colon-separated; default `~/Developer`), `STARFIELD_HISTORY_DAYS` (default `14`), `STARFIELD_DETACH_KEY` (tmux key syntax; default `C-]`), `STARFIELD_REDUCED_MOTION=1`.
- Palette: copy stardocker's `src/layout.go` but set `colorAccent = #d97757` (Claude coral). Running is green `#5fd787`, external is steel `#7aa2f7`, stopped is muted `245`. Glyphs: `●` running/idle, `◐` busy (pulsing between `◐◓◑◒` on the frame tick), `◌` external, `○` stopped, `◔` starting.
- If tmux is not on PATH, `main.go` prints `starfield needs tmux: brew install tmux` and exits 1.
- Platforms: darwin and linux only. GoReleaser has no windows target.
- No new third-party dependencies beyond the Charm stack. Generate UUIDs with `crypto/rand` (v4), write the fuzzy matcher by hand, and get the process table from `ps -axo pid=,ppid=,comm=`.
- The TUI needs a TTY. Verify with unit tests, `go vet` and `gofmt`, then ask the user to run `! go run .` (as stardocker's CLAUDE.md says).

## Review Focus

1. **Starfield launched inside another tmux** (`$TMUX` set): attach must still work. The attach command removes `TMUX` from the child environment, and the private socket avoids nesting confusion. Test: `TestAttachCmdDropsTMUXEnv` (Task 4).
2. **Large history files** (100 MB+ jsonl): discovery must read only a bounded head and tail per file and cache by (path, mtime, size). Test: `TestParseSessionFileReadsBoundedBytes` (Task 2).
3. **Claude exits while attached** (`/exit`): the tmux session vanishes and `attach` returns non-zero. The TUI must treat that as a normal return, refresh, and show the card as stopped with no error. Test: `TestAttachDoneWithExitedSessionIsNotAnError` (Task 8).
4. **Stale registry file whose PID was reused** by a non-claude process: it must not show as running. Test: `TestReadRegistrySkipsDeadOrReusedPID` (Task 3).
5. **Resume or new session in a directory that no longer exists**, or a typed path that doesn't exist: show an inline error and never call tmux. Tests: `TestResumeMissingDirShowsError` (Task 8) and `TestPickerTypedMissingPathShowsError` (Task 9).

---

## File Structure

```
main.go                       flags (-v/-h), tmux preflight, config, tea.NewProgram
internal/config/config.go     Config struct from env (+ test)
internal/claude/history.go    jsonl head/tail parser, ScanHistory, cache (+ test, testdata/)
internal/claude/registry.go   ~/.claude/sessions/*.json reader (+ test)
internal/procs/procs.go       ps table parse, ancestry (+ test)
internal/tmux/tmux.go         Client: list/new/attach/kill/capture (+ test, integration test)
internal/tmux/conf.go         generated tmux.conf (+ test)
internal/sessions/merge.go    Merge → []Session; State; Discover (+ test)
internal/dirs/dirs.go         Scan roots, Candidates (+ test)
internal/dirs/fuzzy.go        Fuzzy matcher (+ test)
src/layout.go cards.go starfield.go input.go confirm.go help.go   copied UI kit from stardocker (+ their tests)
src/home.go                   home screen model
src/home_cards.go             Session → gridEntry builder (+ test)
src/actions.go                attach/resume/start/stop tea.Cmds (+ test)
src/picker.go                 new-session directory picker (+ test)
src/preview.go                live pane preview panel
CLAUDE.md README.md .gitignore .github/workflows/ci.yml .goreleaser.yaml
```

---

### Task 1: Scaffold, config, and the UI kit copied from stardocker

**Files:**
- Create: `go.mod`, `main.go`, `internal/config/config.go`, `internal/config/config_test.go`, `.gitignore`, `.github/workflows/ci.yml`, `.goreleaser.yaml`, `CLAUDE.md`
- Copy and adapt from stardocker: `src/layout.go`, `src/cards.go`, `src/cards_test.go`, `src/starfield.go`, `src/starfield_test.go`, `src/input.go`, `src/confirm.go`, `src/help.go`

**Interfaces:**
- Produces:
  - `config.Load(getenv func(string) string, home string) Config`, where `Config{Roots []string; HistoryDays int; DetachKey string; ClaudeCmd []string; ReducedMotion bool; ClaudeDir string /* ~/.claude */; StateDir string /* ~/.config/starfield */}`.
  - From `src/`, everything stardocker exports internally: `gridView`, `gridEntry`, `renderHeader`, `renderBottomBar`, `renderPanelColored`, `ansiTruncate`, `truncate`, `newStarfield`, `textField`, `filterBar`, `matches`, `confirmPrompt`, `statusLine`, `newHelp`, `newTickID`, `tickMsg`.

- [ ] **Step 1:** Run `git init` and `go mod init github.com/stardust1420/starfield`, then add deps at stardocker's versions (bubbletea v1.3.10, bubbles v0.21.0, lipgloss v1.1.0, x/ansi v0.10.1, reflow v0.3.0). The sandbox may block `go mod tidy`, so run it with the sandbox disabled, as stardocker's `.claude.local.md` notes.
- [ ] **Step 2:** Copy the listed `src/` files. Strip anything Docker-specific (`containerTypes` refs, `shortStatus`, `usedByLine`, the `TestShortStatus` case). Re-theme `layout.go` per Global Constraints and add `colorExternal`, `externalStyle`, and the glyph constants `glyphBusy`, `glyphExternal`, `glyphStopped`, `glyphStarting`. In `starfield.go`, read `STARFIELD_REDUCED_MOTION`.
- [ ] **Step 3: Write the failing test** `TestLoadDefaultsAndOverrides`:
  - With an empty env: `Roots == []string{home+"/Developer"}`, `HistoryDays == 14`, `DetachKey == "C-]"`, `ClaudeCmd == []string{"caffeinate","-is","claude"}`.
  - With `STARFIELD_ROOTS="~/a:/b"`: `Roots == [home+"/a", "/b"]`.
  - With `STARFIELD_HISTORY_DAYS="x"`: falls back to 14.
- [ ] **Step 4:** Implement `config.Load`.
- [ ] **Step 5:** `main.go` mirrors stardocker's: version and usage text that lists the env vars, then the tmux `LookPath` preflight (message from Global Constraints), then `tea.NewProgram(src.InitHomeModel(deps, 0, 0), tea.WithAltScreen())`. Until Task 7 lands, use a stub `InitHomeModel` that renders the header.
- [ ] **Step 6:** Copy `ci.yml` (gofmt, vet, build, test) and `.goreleaser.yaml` (goos `[linux, darwin]`). Write `CLAUDE.md` in stardocker's style; extend it as later tasks land.
- [ ] **Step 7: Verify.** Run `go build . && go vet ./... && go test ./...`. Expected: PASS, including the copied grid and starfield tests.
- [ ] **Step 8:** Commit `chore: scaffold starfield with stardocker UI kit`.

### Task 2: Claude history parser

**Files:** Create `internal/claude/history.go`, `internal/claude/history_test.go`, `internal/claude/testdata/*.jsonl`

**Interfaces:**
- Produces:
  - `type HistorySession struct { ID, Dir, Path, Title, LastPrompt, Branch, Model string; CostUSD float64; UserMessages int; Started, LastActive time.Time }`
  - `func ParseSessionFile(path string) (HistorySession, error)`
  - `func ScanHistory(projectsDir string, since time.Time, cache *HistoryCache) ([]HistorySession, error)`
  - `type HistoryCache` keyed by (path, mtime, size); `NewHistoryCache() *HistoryCache`

**Parsing rules:**
- Read the first 64 KiB and the last 256 KiB. When the file is ≤ 320 KiB, read the whole thing.
- Decode line by line and skip lines that fail to decode. That covers a partial line where a read window is cut and a half-written last line.
- **From the head:**
  - The first record that has `cwd`, for `Dir` and `Branch` (`gitBranch`).
  - The first `timestamp`, for `Started`.
- **From the tail:**
  - The last `custom-title.customTitle`, for `Title`.
  - The last `last-prompt.lastPrompt`, for `LastPrompt`.
  - The last `cost-state.totalCostUSD`.
  - The last `assistant.message.model`, with any `claude-` prefix stripped.
  - The last `timestamp`, for `LastActive`. Use the file mtime if there is none.
- **Counting user messages:** `UserMessages` counts `type=="user"` records whose `turnOrigin=="human"` in the windows read. It is a lower bound for large files. The card renders it as `N+ msgs` when the file was truncated.
- **Identity and filtering:**
  - `ID` comes from `sessionId`, falling back to the filename without `.jsonl`.
  - Take `Dir` from `cwd`, never from the encoded folder name, which loses information (`_` becomes `-`).
- **ScanHistory:**
  - Only top-level `*.jsonl` files in each project dir; skip subdirectories, which hold subagents.
  - Skip files whose mtime is before `since`.
  - Skip sessions with `UserMessages == 0` and no `LastPrompt`.

- [ ] **Step 1: Write failing tests** using hand-made fixtures, each a few lines in the real record shapes seen in `~/.claude/projects`:
  - `TestParseSessionFileBasic`: with cwd `/x/my_proj`, asserts `Dir == "/x/my_proj"`, `Title`, `LastPrompt`, `Branch == "main"`, `Model == "opus-5-5"`, `CostUSD == 10.22`, `UserMessages == 2`, and the `Started`/`LastActive` timestamps.
  - `TestParseSessionFileTruncatedLastLine`: still parses.
  - `TestParseSessionFileReadsBoundedBytes`: writes a fixture padded with 5 MB of filler records in the middle and uses an injectable `openFile` that counts bytes read. Asserts the count is ≤ 320 KiB and that head and tail fields are still correct.
  - `TestScanHistorySkipsOldEmptyAndSubdirs`
  - `TestHistoryCacheReusesUnchangedFile`: a second scan must not re-open the file (counting opener).
- [ ] **Step 2:** Run `go test ./internal/claude/ -run History -v`. Expected: FAIL (undefined).
- [ ] **Step 3:** Implement with `os.File.ReadAt` for the two windows and `json.Unmarshal` into a narrow struct with only the needed fields.
- [ ] **Step 4:** Run `go test ./internal/claude/...`. Expected: PASS.
- [ ] **Step 5:** Commit `feat: parse claude session history`.

### Task 3: Live registry and process table

**Files:** Create `internal/claude/registry.go`, `internal/claude/registry_test.go`, `internal/procs/procs.go`, `internal/procs/procs_test.go`

**Interfaces:**
- Produces:
  - `type LiveSession struct { PID int; SessionID, Dir, Name, Status string; StartedAt time.Time }`. `Status` is the registry's raw string, e.g. `busy` or `idle`.
  - `func ReadRegistry(sessionsDir string, table procs.Table) ([]LiveSession, error)`
  - `type Proc struct { PID, PPID int; Comm string }`
  - `type Table map[int]Proc`
  - `func ParsePS(out string) Table` parses `ps -axo pid=,ppid=,comm=` output.
  - `func Snapshot() (Table, error)` runs that `ps` command.
  - `func (t Table) IsAncestor(anc, pid int) bool`: walks up PPIDs, max 64 hops.
  - `func (t Table) IsClaude(pid int) bool`: the basename of `Comm` is `claude`, or the comm path contains `/claude`.

- [ ] **Step 1: Write failing tests:**
  - `TestParsePS` covers leading spaces, comm paths with spaces, and junk lines.
  - `TestIsAncestor`: chain 1→50→60→70 gives `IsAncestor(50,70)==true` and `IsAncestor(60,50)==false`. A cycle in the table terminates.
  - `TestReadRegistrySkipsDeadOrReusedPID`: three files, `{pid 70 claude}`, `{pid 80 not in table}` and `{pid 90 comm "node-other"}`. Only pid 70 is returned. A malformed json file is skipped and does not cause an error. `startedAt` is in epoch milliseconds.
- [ ] **Step 2:** Run `go test ./internal/claude/ ./internal/procs/`. Expected: FAIL.
- [ ] **Step 3:** Implement. A process is alive and really Claude only if `table.IsClaude(pid)` holds; there is no `kill -0`, so the code can be tested from the table alone.
- [ ] **Step 4:** Run the tests again. Expected: PASS.
- [ ] **Step 5:** Commit `feat: read live claude registry and process table`.

### Task 4: tmux client and themed config

**Files:** Create `internal/tmux/tmux.go`, `internal/tmux/conf.go`, `internal/tmux/tmux_test.go`, `internal/tmux/integration_test.go`

**Interfaces:**
- Produces:
  - `type Client struct { Bin, Socket, ConfPath string }`
  - `func New(cfg config.Config) (*Client, error)`: writes the conf to `cfg.StateDir/tmux.conf`.
  - `type Pane struct { Session string; PanePID int; ClaudeID, Dir string; Created time.Time; Attached bool }`
  - `func (c *Client) List() ([]Pane, error)`: an empty slice, not an error, when no server is running (stderr contains `no server running` or `error connecting`).
  - `func (c *Client) NewSession(name, dir, claudeID string, argv []string, width, height int) error`
  - `func (c *Client) AttachCmd(name string) *exec.Cmd`
  - `func (c *Client) Kill(name string) error`
  - `func (c *Client) Capture(name string, lines int) (string, error)`
  - `func SessionName(claudeID string) string`, which returns `"sf-"+claudeID[:8]`
  - `func RenderConf(detachKey string) string`

**Details:**
- `List` uses `list-panes -a -F '#{session_name}\t#{pane_pid}\t#{@sf_session}\t#{@sf_dir}\t#{session_created}\t#{session_attached}'`.
- `NewSession` runs `new-session -d -s name -c dir -x W -y H -- argv…`, then `set-option -t name @sf_session id` and `set-option -t name @sf_dir dir`.
- `Capture` uses `capture-pane -p -e -t name -S -lines`.
- `RenderConf` writes these lines:
  - `set -g default-terminal "tmux-256color"`
  - `set -as terminal-features ",xterm-ghostty:RGB:extkeys,xterm-256color:RGB"`
  - `set -s extended-keys on` and `set -as terminal-features 'xterm*:extkeys'`, so shift+enter reaches Claude
  - `set -s escape-time 0`, `set -g history-limit 100000`, `set -g mouse on`
  - `set -s set-clipboard on`, `set -g allow-passthrough on`, `set -g focus-events on`
  - `set -g prefix None`, `unbind C-b`, and `bind -n <detachKey> detach-client`
  - A bottom status bar: `status-style bg=#1a1b26,fg=#a9b1d6`, `status-left " ✦ starfield │ #{b:@sf_dir} "` with the accent `#d97757`, and `status-right " #{?client_prefix,,<detachKey> ⏎ home} "`.

- [ ] **Step 1: Write failing unit tests:**
  - `TestSessionName`
  - `TestParsePanes`: tab-separated fixture to `[]Pane`.
  - `TestRenderConfBindsDetachKey`: contains `bind -n C-] detach-client` and `extended-keys on`.
  - `TestNewSessionArgv`: make the exec runner injectable as `run func(args ...string) ([]byte, error)` and assert the exact argv sequence.
  - `TestAttachCmdDropsTMUXEnv`: with `t.Setenv("TMUX","x")`, `cmd.Env` has no `TMUX=` entry, and the args are `-L starfield -f <conf> attach-session -t name`.
- [ ] **Step 2: Write the integration test** `TestTmuxLifecycle`, which uses `t.Skip` when `exec.LookPath("tmux")` fails. It runs on a temp socket name and checks `NewSession("sf-test", t.TempDir(), id, ["sleep","30"],80,24)`, then `List` (contains the session with `ClaudeID==id`), then `Capture` (no error), then `Kill`, then `List` (empty).
- [ ] **Step 3:** Run `go test ./internal/tmux/`. Expected: FAIL.
- [ ] **Step 4:** Implement.
- [ ] **Step 5:** Run `brew install tmux` (the user must allow it), then `go test ./internal/tmux/ -v`. Expected: PASS, with the integration test running rather than skipped.
- [ ] **Step 6:** Commit `feat: tmux client with themed private config`.

### Task 5: Merge sources into sessions

**Files:** Create `internal/sessions/merge.go`, `internal/sessions/discover.go`, `internal/sessions/merge_test.go`

**Interfaces:**
- Consumes: `claude.HistorySession`, `claude.LiveSession`, `tmux.Pane`, `procs.Table`
- Produces:
  - `type State int`, with constants `Running` (managed, Claude alive), `Starting` (managed tmux pane, no Claude registry entry yet), `External`, `Stopped`
  - `type Session struct { ID, Dir, Title, LastPrompt, Branch, Model, Status, TmuxName string; CostUSD float64; UserMessages int; Truncated bool; Started, LastActive time.Time; State State; PID int }`
  - `func Merge(hist []claude.HistorySession, live []claude.LiveSession, panes []tmux.Pane, table procs.Table) []Session`
  - `type Discoverer struct{...}` with `func NewDiscoverer(cfg config.Config, tm *tmux.Client) *Discoverer` and `func (d *Discoverer) Discover() ([]Session, error)`. It calls `ScanHistory(since = now - HistoryDays)`, `procs.Snapshot`, `ReadRegistry` and `tm.List`, then `Merge`.

**Merge rules:**
- **Live sessions:** each live session becomes one `Session` keyed by `SessionID`.
  - It is `Running` with `TmuxName` set when some pane's `PanePID` is an ancestor of the live PID. Otherwise it is `External`.
  - Fill in metadata from the history entry with the same ID when one exists. A live session with no history file still appears, with `Title` taken from the registry `Name`.
- **Leftover panes:** each pane not matched to a live session becomes `Starting`, with the ID from `ClaudeID` and history metadata if present. A history entry whose ID is already live is not emitted again.
- **History:** the remaining history entries become `Stopped`.
- **Sort:** by state (Running, Starting, External, Stopped), then `LastActive` descending.

- [ ] **Step 1: Write failing tests**, one per rule:
  - `TestMergeManagedRunning`
  - `TestMergeExternal`
  - `TestMergeDedupesResumedHistory`
  - `TestMergeStartingPane`
  - `TestMergeLiveWithoutHistoryUsesRegistryName`
  - `TestMergeSortOrder`
- [ ] **Step 2:** Run `go test ./internal/sessions/`. Expected: FAIL.
- [ ] **Step 3:** Implement `Merge`, then `Discoverer`. `Discoverer` holds the `HistoryCache`.
- [ ] **Step 4:** Run the tests again. Expected: PASS.
- [ ] **Step 5:** Commit `feat: merge history, registry and tmux into sessions`.

### Task 6: Directory candidates and fuzzy matcher

**Files:** Create `internal/dirs/dirs.go`, `internal/dirs/fuzzy.go`, `internal/dirs/dirs_test.go`, `internal/dirs/fuzzy_test.go`

**Interfaces:**
- Produces:
  - `type Source int`, with constants `Recent`, `Scanned`, `Typed`
  - `type Candidate struct { Path string; Source Source; LastUsed time.Time }`
  - `func Scan(root string, depth int) []string`: a directory is included if it contains `.git`, or if it is a direct child of root. Hidden dirs, `node_modules` and `vendor` are skipped, and nothing is descended into below a `.git` repo.
  - `func Candidates(hist []claude.HistorySession, roots []string, depth int) []Candidate`: Recent entries (unique `Dir`s, LastUsed descending) come first, then Scanned entries not already present. Only existing directories are included.
  - `func Expand(path, home string) string`: handles `~`.
  - `type Match struct { Index, Score int; Positions []int }`
  - `func Fuzzy(query string, paths []string) []Match`

**Fuzzy rules:**
- Case-insensitive subsequence match over the path.
- Scoring:
  - +10 per matched char inside the basename
  - +5 for a char consecutive with the previous match
  - +8 for a match right after `/`, `-`, `_` or `.`
  - −1 for each gap char
- An empty query returns every item in order. Ties keep the input order (stable sort).

- [ ] **Step 1: Write failing tests:**
  - `TestScanFindsReposAndChildren`: a temp tree with `a/.git`, `b/c/.git`, `.hidden/.git` and `node_modules/x/.git`.
  - `TestCandidatesRecentFirstDeduped`
  - `TestCandidatesSkipsMissingDirs`
  - `TestFuzzyPrefersBasename`: query `core` ranks `/dev/core` above `/dev/core-tools/x` and `/core/legacy/a`.
  - `TestFuzzyNoMatch`
  - `TestFuzzyEmptyQueryKeepsOrder`
  - `TestExpandTilde`
- [ ] **Step 2:** Run `go test ./internal/dirs/`. Expected: FAIL.
- [ ] **Step 3:** Implement. The matcher uses greedy leftmost matching, then a second pass anchored at the basename, keeping the better score.
- [ ] **Step 4:** Run the tests again. Expected: PASS.
- [ ] **Step 5:** Commit `feat: directory candidates and fuzzy matcher`.

### Task 7: Home screen with session cards

**Files:** Create `src/home.go`, `src/home_cards.go`, `src/home_cards_test.go`. Modify `src/help.go` (add `homeKeys`) and `main.go`.

**Interfaces:**
- Consumes: `sessions.Session`, `sessions.Discoverer`, `gridView`, `newStarfield`, `filterBar`
- Produces:
  - `type Deps struct { Cfg config.Config; Tmux *tmux.Client; Disc *sessions.Discoverer }`
  - `func InitHomeModel(d Deps, width, height int) homeModel`
  - `func (m homeModel) resume(width, height int) homeModel` (stardocker's pattern)
  - `func sessionEntries(ss []sessions.Session, now time.Time, home string) ([]gridEntry, []*sessions.Session)`
  - `type discoveredMsg struct{ id int; sessions []sessions.Session; err error }`

**Grid layout:**
- Group cards under one selectable-false heading per directory, `▸ <basename>  ~/path  ·  2 running`. Groups are ordered by their best session's sort position.
- Each card title is the session `Title`, truncated.

**Card body:**
1. `⎇ main  ·  opus-5-5`
2. `"<last prompt>"`, muted and italic
3. `$10.22 · 133 msgs`
4. One of:
   - Running: `● idle · up 2h` or `◐ working · up 2h`
   - Starting: `◔ starting…`
   - External: `◌ external · pid 11435`
   - Stopped: `○ 3h ago`

  Lines that don't apply are left out.

**Header and refresh:**
- Header: `✦ starfield` brand, the `/filter` crumb, then right-aligned `3 running · 1 external · 24 stopped` plus `[all]` or `[running]`.
- Discovery runs async on a 2 s tick tagged with `tickID`. A `frameMsg` at 10 fps drives the busy pulse, and the starfield only when the grid is empty.

**Keys:**
- Grid movement as in stardocker.
- `a` toggles running-only, which hides Stopped and keeps Running, Starting and External.
- `/` filters on title, dir, branch and prompt.
- `n`, `enter`, `x`, `p` are wired in Tasks 8–10.
- `?` toggles help, `q`/`ctrl+c` quit.

**Empty states:**
- With no sessions at all, render the starfield behind `No Claude sessions yet — press n to start one`.
- In running-only mode with none running, show `Nothing running · a to show all`.

- [ ] **Step 1: Write failing tests** in `home_cards_test.go`:
  - `TestSessionEntriesGroupsByDirWithHeadings`: 3 sessions in 2 dirs produce `[heading, card, card, heading, card]`, and items stay parallel to entries (nil for headings).
  - `TestCardBodyPerState`: strip ANSI with the regex from `starfield_test.go` and check the exact line 4 text for each state, using fixed `now`.
  - `TestCardOmitsMissingFields`: no branch and no model means line 1 is omitted.
  - `TestRunningOnlyFilter`
- [ ] **Step 2:** Run `go test ./src/ -run 'SessionEntries|CardBody|CardOmits|RunningOnly'`. Expected: FAIL.
- [ ] **Step 3:** Implement `home_cards.go`, then `home.go`, and wire `main.go` to build `Deps`.
- [ ] **Step 4:** Run `go test ./... && go vet ./...`. Expected: PASS.
- [ ] **Step 5:** Commit `feat: home screen with session cards`.

### Task 8: Attach, resume, stop

**Files:** Create `src/actions.go`, `src/actions_test.go`. Modify `src/home.go`.

**Interfaces:**
- Consumes: `tmux.Client.{AttachCmd,NewSession,Kill}`, `tmux.SessionName`, `confirmPrompt`, `statusLine`
- Produces:
  - `type sessionOps interface { AttachCmd(string) *exec.Cmd; NewSession(name, dir, id string, argv []string, w, h int) error; Kill(string) error }`. `*tmux.Client` satisfies it, and tests use a fake.
  - `func attachCmd(ops sessionOps, name string) tea.Cmd` wraps `tea.ExecProcess` and returns `attachDoneMsg{name, err}`.
  - `func resumeCmd(ops sessionOps, cfg config.Config, s sessions.Session, w, h int) tea.Cmd` runs NewSession with argv `cfg.ClaudeCmd + ["--resume", s.ID]`, then returns `startedMsg{name}`, and home then calls `attachCmd`.
  - `func startCmd(ops sessionOps, cfg config.Config, dir string, w, h int) tea.Cmd` creates a new UUID v4 with argv `cfg.ClaudeCmd + ["--session-id", id]`, then returns `startedMsg`.
  - `func newUUID() string`

**Behavior in home `Update`:**

| Key / message | Selected session | Result |
|---|---|---|
| `enter` | Running or Starting | `attachCmd` |
| `enter` | Stopped | Checks that `os.Stat(s.Dir)` is a dir (else status error `directory gone: <dir>`), then `resumeCmd` |
| `enter` | External | Status `running in another terminal (pid N) — can't attach` |
| `x` | Running or Starting | Confirm prompt `stop <title>?` (yes label `stop`), then `Kill` |
| `x` | External or Stopped | No-op, with a status hint |
| `attachDoneMsg` | — | Always triggers an immediate refresh. An `*exec.ExitError` is not shown as an error, because the session may have exited while attached. |

- [ ] **Step 1: Write failing tests** with a fake `sessionOps` that records calls:
  - `TestResumeArgv`: argv is exactly `["caffeinate","-is","claude","--resume","<id>"]` and the name is `sf-<id[:8]>`.
  - `TestStartArgvUsesFreshUUID`: matches `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`.
  - `TestResumeMissingDirShowsError`: home `Update` with enter on a Stopped session whose dir doesn't exist produces a status containing `directory gone`, and the fake records no NewSession call.
  - `TestEnterOnExternalDoesNotAttach`
  - `TestAttachDoneWithExitedSessionIsNotAnError`: `attachDoneMsg{err: &exec.ExitError{}}` leaves the status empty and returns a non-nil refresh cmd.
  - `TestStopRequiresConfirm`: `x` opens the prompt, `y` makes the fake record `Kill("sf-…")`.
- [ ] **Step 2:** Run `go test ./src/ -run 'Resume|Start|External|AttachDone|Stop'`. Expected: FAIL.
- [ ] **Step 3:** Implement. `homeModel` holds `ops sessionOps`, set from `Deps.Tmux`.
- [ ] **Step 4:** Run `go test ./...`. Expected: PASS.
- [ ] **Step 5:** Commit `feat: attach, resume and stop sessions via tmux`.

### Task 9: New-session directory picker

**Files:** Create `src/picker.go`, `src/picker_test.go`. Modify `src/home.go` (`n` opens it) and `src/help.go`.

**Interfaces:**
- Consumes: `dirs.Candidates`, `dirs.Fuzzy`, `dirs.Expand`, `textField`, `startCmd`, `homeModel.resume`
- Produces: `func initPickerModel(back homeModel, cands []dirs.Candidate, width, height int) pickerModel`

**Layout:**
- A centered panel titled `new session` (`renderPanelColored`, accent border) with a prompt line `❯ <query>▏`.
- Below it, up to `height-8` rows of results: a source badge (`◷ recent` / `⌂ repo` / `↳ path`), the path with `~` substituted and matched chars in the accent colour, and a right-aligned muted `LastUsed` relative time for recent entries.
- The selected row is highlighted `▌` in the accent colour.

**Keys:**
- Printable characters edit the query.
- `↑`/`↓` and `ctrl+p`/`ctrl+n` move the selection.
- `enter` starts the session.
- `tab` completes the query to the selected path.
- `esc` returns `back.resume(w,h)`.

**Typed paths:** when the query starts with `/`, `~` or `.`, a synthetic `Typed` row for `Expand(query)` is pinned at the top. If that path is not an existing directory, the row shows `✗ not a directory`, and enter on it sets an inline error and does not start anything.

- [ ] **Step 1: Write failing tests:**
  - `TestPickerFiltersAsYouType`: typing `co` narrows the results to fuzzy matches, best first.
  - `TestPickerEnterStartsInSelectedDir`: with a fake ops, enter returns a cmd whose msg is `startedMsg`, and the fake recorded NewSession with that `dir`.
  - `TestPickerTypedPathRowPinned`: query `t.TempDir()` puts the first row's Source == Typed.
  - `TestPickerTypedMissingPathShowsError`: query `/nope/zzz` plus enter leaves no NewSession call and the view contains `not a directory`.
  - `TestPickerEscReturnsHome`
- [ ] **Step 2:** Run `go test ./src/ -run Picker`. Expected: FAIL.
- [ ] **Step 3:** Implement. Build the candidates when `n` is pressed, from the history the home model already holds plus `dirs.Scan` over `cfg.Roots` at depth 2.
- [ ] **Step 4:** Run `go test ./...`. Expected: PASS.
- [ ] **Step 5:** Commit `feat: fuzzy directory picker for new sessions`.

### Task 10: Live preview panel, README and end-to-end check

**Files:** Create `src/preview.go` and `README.md`. Modify `src/home.go`, `src/help.go` and `CLAUDE.md`.

**Interfaces:**
- Consumes: `tmux.Client.Capture`
- Produces: `func previewPanel(content string, width, lines int) string`

**Behavior:**
- `p` toggles a bottom panel, 12 lines tall, titled `preview · <tmux name>`. It shows the last lines of the selected managed session's pane (ANSI kept, each line truncated with `ansiTruncate`) and refreshes on the 2 s tick.
- For External or Stopped sessions the panel shows the full last prompt instead.
- `listGridHeight` must account for the panel. The panel height is subtracted from the grid height, the same way stardocker handles its confirmation panel.

- [ ] **Step 1: Write the failing test** `TestPreviewPanelExactHeight`: for any content, `lipgloss.Height(previewPanel(c, 80, 12)) == 12` and each line has width ≤ 80.
- [ ] **Step 2:** Implement, run `go test ./...`, and expect PASS.
- [ ] **Step 3:** README covers: install (`brew install tmux`, `go install github.com/stardust1420/starfield@latest`), the shell alias tip (`alias sf=starfield`), the keybinding table per screen, the env vars, how detach works, and why external sessions can't be attached. Finalize the architecture section of `CLAUDE.md` in stardocker's format.
- [ ] **Step 4: Manual end-to-end check.** The user runs `! go run .` in Ghostty and confirms each item:
  1. Cards appear for past sessions across dirs, and this planning session shows as `◌ external`.
  2. `a` toggles the view.
  3. `n`, type `stard`, enter: Claude starts in stardocker. Shift+enter inserts a newline, mouse wheel scrolls, and colors are correct.
  4. `C-]` returns home. The card shows `● idle`, or `◐ working` while Claude thinks.
  5. Enter re-attaches to the same conversation.
  6. `/exit` inside Claude returns home and the card becomes `○ just now`.
  7. Enter on that stopped card resumes it.
  8. `x` then `y` stops it.
  9. `q` quits, and `tmux -L starfield ls` still lists the sessions that are still running.
- [ ] **Step 5:** Run `gofmt -l . && go vet ./... && go test ./...`. Expected: no output, then PASS.
- [ ] **Step 6:** Commit `feat: live pane preview and docs`.
