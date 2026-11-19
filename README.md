# Queen

A macOS terminal downloader built on [yt-dlp](https://github.com/yt-dlp/yt-dlp), featuring a full-screen
terminal user interface (TUI) implemented with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
and [Lip Gloss](https://github.com/charmbracelet/lipgloss), concurrent multi-task downloads, and
per-site compatibility profiles.

<p align="center">
<a href="/README.md"><b>English</b></a> · <a href="/README.zh-CN.md">中文</a>
</p>

<p align="center">
<img src="https://img.shields.io/badge/Go-1.26%2B-00ADD8?style=for-the-badge&logo=go&logoColor=white">
<img src="https://img.shields.io/badge/License-MIT-blue?style=for-the-badge&logo=open%20source%20initiative&logoColor=white">
<img src="https://img.shields.io/badge/Platform-macOS-8a2be2?style=for-the-badge&logo=apple&logoColor=white">
<img src="https://img.shields.io/badge/Build-CI_passing-success?style=for-the-badge&logo=githubactions&logoColor=white">
<img src="https://img.shields.io/github/v/release/kesonglab/queen?style=for-the-badge&logo=github&logoColor=white">
<img src="https://img.shields.io/badge/yt--dlp-powered-cc0000?style=for-the-badge">
</p>

---

## Overview

Queen is a self-contained, single-install command-line front end for yt-dlp. It wraps yt-dlp's
configuration and invocation behind an interactive menu, then renders every download as a live task
with its own phase (extracting, downloading, merging, converting, embedding metadata, fixing the
container), percentage, size, throughput and remaining time. Several links download concurrently, each
in its own worker and process group, so one stalled or failed task never blocks the rest of the batch.

The v1.6.0 interface is a full redesign: a theme system (auto / dark / light), a responsive layout that
stays readable from a 60×16 terminal up to an ultrawide one, scrollable lists, a status bar, a help
overlay, and a results table with one-key follow-up actions. The download pipeline consumes yt-dlp's
structured progress output instead of parsing human-readable text, and a site-profile layer supplies
per-site cookies, browser impersonation and concurrency values. Failures are classified into 12
actionable categories, optionally retried once with the right workaround, and written to a failure log
you can copy into a bug report.

## Features

### Interface

- Full-screen TUI with a semantic theme system: `auto` follows the terminal background, plus explicit
  `dark` and `light` palettes. Press `Ctrl+T` anywhere to cycle the theme; `NO_COLOR` disables colour.
- Responsive layout: the content area is capped at 100 columns and centred on wide terminals; on a
  terminal smaller than 60×16 Queen shows a resize hint instead of broken output.
- Scrollable task list and settings page. Settings are grouped into Content, Quality & container,
  Network & performance, Auth & sites and General sections with the actions at the bottom; text
  values are edited in a single-line overlay (Enter saves, Esc cancels, Ctrl+U clears).
- The bottom status bar shows the keys that apply to the current screen; the header shows the Queen
  version, the current screen, the yt-dlp version (with a warning when it is stale) and the
  configured concurrency.
- Download view with per-task phase labels, two-line active rows (progress bar, size, speed, ETA),
  playlist position (`item N/M`), fragment progress (`frag N/M`) when the stream has no known total
  size, a status filter (`all` / `active` / `failed`), single-task cancel (`x`) and abort-all
  (`q` / `Esc`).
- Results page with a per-task table (site, task, size, duration, failure reason) and one-key actions:
  retry failed tasks (`r`), open the download folder (`o`), copy a diagnostics report (`c`) and open
  the failure-log folder (`l`).
- `?` opens a scrollable keyboard-shortcut overlay on every screen except the input screen; the in-app
  changelog renders Markdown instead of raw source.
- Toast messages are rendered on every screen and expire automatically (4 s for notices, 6 s for
  errors).

### Download pipeline

- Structured progress: yt-dlp is invoked with `--progress-template` records built from fixed `|`
  separators and numeric/enum fields, so Queen no longer regular-expression-matches human-readable
  progress text.
- stdout and stderr are read on separate pipes; progress is coalesced every 120 ms before reaching the
  UI, and every batch carries an ID so messages from a previous batch are discarded.
- Each task is a separate yt-dlp process in its own process group; aborting a batch (or cancelling one
  task) kills the whole group, including ffmpeg children.
- After a task succeeds, the real size of the produced file is read with `stat` and shown in the
  results table instead of an estimate.
- Output is read line by line through a buffered reader; a single extremely long line can no longer
  stall the pipe and freeze the UI.

### Performance & network

- Removed the experimental `--http-chunk-size` tuning that could break range requests and signed CDN
  URLs.
- Added anti-throttling and retry controls: `--throttled-rate 100K` (re-extract a stream that is being
  throttled), `--retry-sleep 3` (wait between retries instead of hammering a 429/503),
  `--ignore-config` (a global yt-dlp config can no longer override Queen's flags) and
  `--trim-filenames 200` (keeps long CJK titles under the macOS filename limit).
- Global connection budget: each task borrows its `--concurrent-fragments` quota from a shared pool
  (default 32 connections). A same-site limit (default 2 parallel tasks per host) prevents hammering
  one site into rate limits, and site profiles can raise or lower fragment concurrency per site.
- Audio-only mode extracts to `m4a` by default with `--audio-quality 0`; `mp3`, `opus`, `flac` and
  `best` remain selectable. There is no forced lossy re-encode by default.
- Metadata/thumbnail embedding (`--embed-metadata --embed-thumbnail`) is off by default because it
  costs an extra network request and ffmpeg pass per file.
- Playlist mode is off by default, so pasting a URL that happens to carry a playlist ID downloads only
  the linked item. When it is enabled, playlist items land in a playlist-named subfolder with
  zero-padded indices.
- Retry policy: `--retries` / `--fragment-retries` follow the configured retry count,
  `--extractor-retries` is at least 2, `--socket-timeout` is 30 s and `--file-access-retries` is 3.

### Site compatibility & diagnostics

- Built-in site profiles for YouTube, Bilibili, X/Twitter, Instagram, Facebook, TikTok, Douyin,
  Twitch, Vimeo, Reddit, Niconico, SoundCloud and Rule34Video; override or extend them in
  `~/.config/queen/sites.json`.
- Per-site (or global) cookie strategies: auto-detect a local browser, disable cookies, pick a browser,
  or point at a Netscape `cookies.txt`; plus TLS impersonation (`--impersonate`), `--extractor-args`,
  Referer and User-Agent overrides.
- Error classifier with 12 categories and one automatic retry: browser cookies for auth/membership
  failures and `--impersonate chrome` for Cloudflare challenges.
- Failure logs under `~/.local/state/queen/logs` (latest 200 kept), a "copy diagnostics" action on the
  results page, and a failed-links file with one-key retry.
- yt-dlp freshness check: stale versions (older than 45 days) are flagged at startup, in the header and
  in the menu, with a one-key upgrade page that uses Homebrew when yt-dlp was installed with it.
- Privacy mode hides URLs, domains and titles from notifications and failure logs; Rule34Video
  enables it by default.
- Honest boundary: Queen downloads public, non-DRM media that yt-dlp can extract. DRM streaming
  services (Netflix, Disney+, Prime Video, Apple TV+, HBO Max, Spotify and similar) cannot be
  downloaded.

### Other

- Bilingual interface (Chinese / English) and bilingual Markdown changelog.
- Native macOS notifications when a batch finishes and when a task fails, with sounds and a per-URL
  throttle; delivered through `osascript` arguments so quotes and backslashes are safe.
- Failed links are de-duplicated, retryable from the menu, and removed automatically after a later
  success.
- Optional download archive (`--download-archive`) to skip already-downloaded items.
- Automatic dependency detection: missing `yt-dlp` / `ffmpeg` can be installed with Homebrew.
- Mole-inspired visual style.

## Requirements

| Requirement | Purpose |
| --- | --- |
| GNU/BSD [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) | Core download engine |
| [`ffmpeg`](https://ffmpeg.org) | Merging, audio conversion, subtitle/thumbnail embedding |
| macOS | Target platform |
| [Homebrew](https://brew.sh) | Optional, for dependency installation and yt-dlp upgrades |

`yt-dlp` and `ffmpeg` are detected at startup; if either is missing, Queen offers to install it via
Homebrew. Building from source additionally requires Go 1.26 or newer.

## Installation

### Download a binary

Prebuilt binaries are attached to each [release](https://github.com/kesonglab/queen/releases) for both
Apple Silicon (`darwin/arm64`) and Intel (`darwin/amd64`) Macs. Download the matching zip, unpack it,
and run `queen` from the extracted folder:

```bash
arch=$(uname -m); [ "$arch" = "x86_64" ] && arch=amd64
curl -fsSL -o queen.zip \
  https://github.com/kesonglab/queen/releases/latest/download/queen-darwin-$arch.zip
unzip queen.zip
./queen
```

Because the binary is not signed, macOS may block the first launch. If it does, open
**System Settings → Privacy & Security** and choose *Open Anyway*, or right-click the file and
select *Open*.

### Build from source

```bash
git clone https://github.com/kesonglab/queen.git
cd queen
make build
```

`make build` runs `go build` and injects the version from `git describe --tags --always --dirty`, so
a tagged checkout builds `vX.Y.Z` (with a `-dirty` suffix on a modified working tree). The equivalent
explicit command is:

```bash
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" -o queen .
```

The source tree carries the current release version (`v1.6.0`) as its built-in default, so a plain
`go build` already reports `v1.6.0`. A build that injects `1.6.0` also displays `v1.6.0`, and one
that injects `v1.6.0` displays it as-is; setting `dev` explicitly still shows `dev`.

### go install

```bash
make install   # equivalent to: go install ./...
```

## Usage

```bash
queen
```

Two flags work without an interactive terminal:

| Flag | Effect |
| --- | --- |
| `--version` / `-v` | Print the version (for example `v1.6.0`) and exit |
| `--help` / `-h` | Print usage and exit |

Queen opens on the main menu:

| Key | Menu item |
| --- | --- |
| `1` | Paste links to download (one per line, `Ctrl+D` starts) |
| `2` | Read links from the clipboard |
| `3` | Retry previously failed links |
| `4` | Settings |
| `5` | Site profiles |
| `6` | Open the download folder |
| `7` | Update yt-dlp |
| `8` | Changelog |
| `q` | Quit |

A typical batch:

1. Choose `1`, paste one or more links, press `Ctrl+D`.
2. Watch each task's phase (extracting / downloading / merging / converting / embedding), its progress
   bar, size, speed and ETA, plus the aggregated batch panel below the list.
3. When the batch ends, review the results table: `r` retries failures, `o` opens the download folder,
   `c` copies a diagnostics report and `l` opens the failure-log folder.
4. `q` / `Esc` aborts a running batch; `x` cancels only the selected task.

Settings can be edited in-app (`4`) or by hand in `~/.config/queen/config.json`.

## Keyboard shortcuts

### Global

| Key | Action |
| --- | --- |
| `Ctrl+C` | Quit (aborts a running batch first) |
| `?` | Toggle the help overlay |
| `Ctrl+T` | Cycle theme: auto → dark → light |
| Mouse wheel | Scroll the main menu, settings, download list, results table, site profiles, changelog and help |

### Main menu

| Key | Action |
| --- | --- |
| `↑↓` / `j` `k` | Move |
| `1`–`8` | Select a menu item directly |
| `Enter` / `Space` | Confirm |
| `q` | Quit |

### Input

| Key | Action |
| --- | --- |
| `Ctrl+D` | Confirm: start the batch / apply the download folder |
| `Esc` | Back |

When changing the download folder, the path must be absolute (start with `/` or `~`).

### Settings

| Key | Action |
| --- | --- |
| `↑↓` / `j` `k` | Move |
| `PgUp` / `PgDn` | Page up / page down |
| `g` / `G` | Jump to the top / bottom |
| `←→` / `h` `l` | Change the value (text fields open the editor) |
| `Enter` | Edit a text field, advance an option, or run an action |
| `Esc` | Save and return |

Text fields open a single-line editor: Enter saves, Esc cancels, Ctrl+U clears the field.

### Download

| Key | Action |
| --- | --- |
| `↑↓` / `j` `k` | Move |
| `PgUp` / `PgDn` | Page up / page down |
| `g` / `G` | Jump to the top / bottom |
| `x` | Cancel the selected task |
| `f` | Cycle the filter: all → active → failed |
| `q` / `Esc` | Abort the whole batch |

### Results

| Key | Action |
| --- | --- |
| `↑↓` / `j` `k` / `PgUp` `PgDn` / `g` `G` | Scroll the table |
| `r` | Retry all failed tasks |
| `o` | Open the download folder |
| `c` | Copy the diagnostics report |
| `l` | Open the failure-log folder |
| `Enter` / `q` / `Esc` | Back to the main menu |

On the remaining screens (site profiles, update yt-dlp, changelog) `↑↓` / `j` `k` scroll and `Esc`
returns; in the help overlay `?` or `Esc` closes and `j` / `k` scrolls.

## Configuration

The configuration file lives at `~/.config/queen/config.json`. It is created from the built-in defaults
on first launch, migrated automatically from the legacy `~/.config/videodl/config.json` when present,
and can be edited either through the in-app settings page (`4`) or by hand.

- Writes are atomic (temporary file + rename) and the file is created with mode `0600`.
- A corrupt file is backed up to `config.json.corrupt-<timestamp>`, defaults are restored, and a toast
  explains what happened. Save errors are surfaced as toasts instead of being discarded.
- Values outside the documented ranges are clamped on load and after every change.

### General

| Field | Default | Description |
| --- | --- | --- |
| `config_version` | `2` | Config schema version; managed automatically |
| `download_dir` | `~/Downloads` | Download directory (`~` is expanded) |
| `lang` | `"zh"` | Interface language: `zh` or `en` |
| `theme` | `"auto"` | Theme: `auto` (follow the terminal) / `dark` / `light`; `Ctrl+T` cycles it |

### Content

| Field | Default | Description |
| --- | --- | --- |
| `playlist` | `false` | Download the whole playlist when the URL contains one (`--yes-playlist`); off to avoid accidental full-list downloads |
| `playlist_folder` | `true` | When playlist mode is on, put items in a playlist-named subfolder with zero-padded indices |
| `audio_only` | `false` | Extract audio only (`-x --audio-format <audio_format> --audio-quality 0`) |
| `audio_format` | `"m4a"` | Audio container: `m4a` / `mp3` / `opus` / `flac` / `best` |
| `subs` | `false` | Download subtitles and embed them (`--write-subs --embed-subs`) |
| `auto_subs` | `false` | Also fetch auto-generated subtitles (`--write-auto-subs`) |
| `sub_langs` | `"zh.*,en.*"` | Subtitle language filter (`--sub-langs` syntax) |
| `embed` | `false` | Embed metadata and thumbnail (`--embed-metadata --embed-thumbnail`); off by default for speed |

### Quality & container

| Field | Default | Description |
| --- | --- | --- |
| `format_preset` | `"quality"` | Format selector preset: `quality` (`bv*+ba/b`), `compat` (prefers H.264 + AAC / MP4), or `custom` |
| `format` | `"bv*+ba/b"` | Custom yt-dlp format selector, used only when `format_preset` is `custom` |
| `merge_format` | `"mp4"` | Container for merged video+audio: `mp4` / `mkv` / `webm` (`--merge-output-format`) |

### Network & performance

| Field | Default | Description |
| --- | --- | --- |
| `concurrency` | `4` | Number of parallel download tasks (1–16) |
| `fragments` | `8` | Fragment concurrency requested per task (`--concurrent-fragments`, 1–16; borrowed from the connection budget, overridable per site) |
| `connection_budget` | `32` | Global connection budget shared by all tasks (4–64) |
| `same_site_limit` | `2` | Maximum number of parallel tasks hitting the same site (1–8) |
| `retry_times` | `2` | Per-link retries (`--retries` / `--fragment-retries`, 0–10) |
| `retry_sleep` | `3` | Seconds to wait between retries (`--retry-sleep`) |
| `throttled_rate` | `"100K"` | Re-extract when throughput stays below this rate (`--throttled-rate`); empty disables it |
| `limit_rate` | `""` | Bandwidth cap (`--limit-rate`), e.g. `2M`; empty means unlimited |
| `proxy` | `""` | Proxy passed to `--proxy`, e.g. `http://127.0.0.1:7890` |

### Authentication & site overrides

| Field | Default | Description |
| --- | --- | --- |
| `cookie_browser` | `""` | Cookie source: empty = auto-detect a browser, or `chrome` / `chromium` / `brave` / `firefox` / `safari` / `edge` (`--cookies-from-browser`) |
| `cookies_file` | `""` | Path to a Netscape `cookies.txt` (`--cookies`); takes precedence over browser cookies |
| `impersonate` | `""` | TLS fingerprint passed to `--impersonate`: `chrome` / `chrome-136` / `chrome-133` / `safari` / `safari-ios`; empty = off |
| `extractor_args` | `""` | Advanced `--extractor-args` passthrough |
| `user_agent` | `""` | Custom `User-Agent` (`--user-agent`) |
| `referer` | `""` | Custom `Referer` (`--referer`) |

### Files, privacy & startup

| Field | Default | Description |
| --- | --- | --- |
| `trim_filenames` | `200` | Maximum filename length (`--trim-filenames`); `0` = unlimited, otherwise 60–240 |
| `ignore_ytdlp_config` | `true` | Pass `--ignore-config` so a global yt-dlp config cannot override Queen's flags |
| `download_archive` | `false` | Enable `--download-archive` to skip already-downloaded items; the archive is `<download_dir>/.queen-archive.txt` |
| `privacy_mode` | `false` | Hide URLs, domains and titles in notifications and failure logs |
| `keep_failure_logs` | `true` | Save failure logs under `~/.local/state/queen/logs` (latest 200 kept) |
| `update_check` | `true` | Warn at startup when the installed yt-dlp is older than 45 days |

Related paths:

| Path | Meaning |
| --- | --- |
| `~/.config/queen/config.json` | Configuration file |
| `~/.config/queen/sites.json` | Optional user site-profile overrides |
| `<download_dir>/failed_links.txt` | Failed links, one URL per line (auto-migrated from the old bilingual filename) |
| `<download_dir>/.queen-archive.txt` | Download archive, when `download_archive` is enabled |
| `~/.local/state/queen/logs/` | Failure logs |

## Site profiles & compatibility

Queen does not implement site extractors itself: extraction is delegated to yt-dlp, which supports
well over a thousand sites. What Queen adds is a per-site policy layer. For every link it looks up the
host in a table of site profiles, merges the match with the global configuration, and applies the
result when building the yt-dlp command line.

Built-in profiles cover the most common video sites and set sensible defaults: cookies for sites that
usually need a login, browser impersonation for sites that sit behind Cloudflare-style checks, higher
fragment concurrency for sites that benefit from it, and privacy for adult sites.

Open the site-profiles screen from menu item `5` or from the *View site profiles…* action at the
bottom of the settings page; `Esc` returns to the screen you came from. It lists every built-in and
user profile with its matching domains, policy and flags, and reports the path of `sites.json` plus
any parse warning.

### Built-in profiles

| Profile | Matches | Built-in policy |
| --- | --- | --- |
| YouTube | `youtube.com`, `youtu.be`, `youtube-nocookie.com` | 16 concurrent fragments |
| Bilibili | `bilibili.com`, `b23.tv` | cookies auto, Referer `https://www.bilibili.com`, 8 fragments |
| X / Twitter | `x.com`, `twitter.com`, `t.co` | cookies auto |
| Instagram | `instagram.com`, `instagr.am` | cookies auto |
| Facebook | `facebook.com`, `fb.watch`, `fb.com` | cookies auto |
| TikTok | `tiktok.com` | cookies auto, Referer `https://www.tiktok.com/` |
| Douyin | `douyin.com`, `iesdouyin.com` | cookies auto, Referer `https://www.douyin.com/` |
| Twitch | `twitch.tv` | 8 concurrent fragments |
| Vimeo | `vimeo.com` | inherits the global settings |
| Reddit | `reddit.com`, `redd.it`, `v.redd.it` | cookies auto |
| Niconico | `nicovideo.jp`, `nico.ms` | cookies auto |
| SoundCloud | `soundcloud.com` | inherits the global settings |
| Rule34Video | `rule34video.com` | cookies auto, impersonate `chrome`, embed off, privacy on, 4 fragments |

### User overrides (`~/.config/queen/sites.json`)

Create `~/.config/queen/sites.json` to add profiles or override built-ins. The file accepts either a
top-level JSON array or an object with a `profiles` array:

```json
{
  "profiles": [
    {
      "name": "My Vimeo",
      "match": ["vimeo.com"],
      "cookies": "firefox",
      "fragments": 12,
      "embed": false
    },
    {
      "name": "Example",
      "match": ["example.com"],
      "cookies_file": "~/secrets/example-cookies.txt",
      "impersonate": "chrome",
      "referer": "https://example.com/",
      "privacy": true
    }
  ]
}
```

Matching and merging rules:

- `match` holds domain suffixes. A suffix matches the domain itself and all subdomains, so
  `youtube.com` matches `www.youtube.com` and `m.youtube.com`; when several profiles match, the longest
  suffix wins.
- A user profile is merged on top of the matching built-in profile: `name` and `match` replace the
  built-in values, non-empty/non-zero fields override, and `embed` may be set explicitly to `false`.
- `~` in `cookies_file` is expanded to the home directory.

### Profile fields

| Field | Type | Description |
| --- | --- | --- |
| `name` | string | Display name shown on the site-profiles screen (optional) |
| `match` | string array | Domain suffixes to match; the longest matching suffix wins |
| `cookies` | string | Cookie strategy: `""` inherit the global setting, `auto` auto-detect an installed browser, `off` never use cookies, or a browser name (`chrome`, `chromium`, `brave`, `firefox`, `safari`, `edge`) |
| `cookies_file` | string | Path to a Netscape `cookies.txt`; takes precedence over `cookies` |
| `impersonate` | string | TLS fingerprint for `--impersonate` (`chrome`, `chrome-136`, `chrome-133`, `safari`, `safari-ios`); empty = off |
| `extractor_args` | string | Extra `--extractor-args` value |
| `referer` | string | Custom `Referer` header |
| `user_agent` | string | Custom `User-Agent` header |
| `fragments` | integer | Fragment concurrency for this site; `0` = use the global value |
| `embed` | boolean | Override metadata/thumbnail embedding for this site; set `false` to disable |
| `privacy` | boolean | Hide URLs, domains and titles for this site in notifications and failure logs |

### Cookies, impersonation and advanced passthrough

- Cookie precedence: a `cookies_file` always wins; otherwise the site's `cookies` setting decides.
  `auto` uses the configured browser if any, otherwise it auto-detects one in the order Chrome →
  Chromium → Brave → Firefox → Edge → Safari. A browser name is used as-is, `off` disables cookies for
  that site, and an empty value falls back to the global `cookie_browser`.
- Browser cookies are read with `--cookies-from-browser`, which includes that browser's logged-in
  sessions. Only enable this on a machine you trust, and prefer per-site profiles over a global cookie
  browser.
- TLS impersonation (`--impersonate`) makes yt-dlp present a real browser's TLS/HTTP fingerprint. It is
  the usual fix for Cloudflare-style bot checks, but it can cost throughput, so it is off by default
  and can be enabled per site. Target support depends on the installed yt-dlp build; list the
  available targets with `yt-dlp --list-impersonate-targets`.
- `extractor_args`, `referer` and `user_agent` are advanced escape hatches for sites whose extractor
  needs a client hint or a Referer from the embedding page.
- DRM boundary: DRM-protected streaming services (Netflix, Disney+, Prime Video, Apple TV+, HBO Max,
  Spotify and similar) use Widevine/FairPlay/PlayReady and cannot be downloaded by yt-dlp. Queen
  reports them as `DRM` with a hint rather than retrying. The same applies to sites yt-dlp has no
  extractor for: upgrade yt-dlp first.

## Failure diagnostics & recovery

When yt-dlp exits non-zero, Queen keeps the actual error text (no more bare `exit status 1`),
classifies it, optionally retries once with a workaround, records the link for later retry, and — by
default — writes a failure log.

### Error categories

| Category | Meaning | Typical fix | Auto retry |
| --- | --- | --- | --- |
| `auth` | Login, age gate or private video | Pick a cookie browser or provide `cookies.txt` | Yes: retried with browser cookies |
| `membership` | Members-only / paid content | A logged-in account with the right membership | Yes: retried with browser cookies |
| `drm` | DRM-protected stream | Not downloadable | No |
| `geo` | Region-locked content | Use a proxy | No |
| `cloudflare` | Bot check / HTTP 403 | Browser impersonation or cookies | Yes: retried with `--impersonate chrome` |
| `rate_limit` | HTTP 429 / throttling | Retry later, lower concurrency or the connection budget | No |
| `unsupported` | No extractor or no formats | Upgrade yt-dlp | No |
| `network` | Timeouts, resets, 5xx, TLS errors | Check the network/proxy and retry | No |
| `not_found` | Removed, deleted or unavailable | Nothing to do | No |
| `storage` | Disk full or permission denied | Check disk space and folder permissions | No |
| `extractor` | Extractor bug or parse failure | Upgrade yt-dlp | No |
| `unknown` | Anything else | See the failure log | No |

Only one automatic retry happens per workaround: an auth/membership failure is retried once with
browser cookies when the first attempt had none, and a Cloudflare failure is retried once with
`--impersonate chrome` when impersonation was not already enabled. The retried task restarts from the
extracting phase.

### Failure logs and diagnostics

- Every failure is written to `~/.local/state/queen/logs/<timestamp>-<hash>.log` (mode `0600`, latest
  200 kept). A log contains the Queen and yt-dlp versions, the site, the error category and detail, the
  exact yt-dlp arguments, and the full captured output. In privacy mode the URL, domain and title
  are redacted (URLs echoed in the error detail or arguments are masked too).
- On the results page, `c` copies a diagnostics report to the clipboard: versions, key settings, and
  every failed task with its category, detail and log path — ready to paste into an issue. `l` opens
  the log folder.
- Failure logs can be disabled with `keep_failure_logs`.

### Failed links

Failed URLs are stored in `<download_dir>/failed_links.txt` (one per line, de-duplicated, mode `0600`)
and auto-migrated from the old bilingual filename `视频失败链接-failed_links.txt`. Menu item `3`
retries them, and a later success removes the URL automatically.

### yt-dlp freshness and upgrades

At startup Queen runs `yt-dlp --version` (a few milliseconds) and parses the release date. If the
installed yt-dlp is older than 45 days it warns in the main menu, in the header (`⚠`) and with a
startup toast — upgrading yt-dlp is usually the fastest fix for site breakage. Menu item `7` runs the
correct upgrade command: `brew upgrade yt-dlp` when the binary lives in a Homebrew prefix, otherwise
`yt-dlp -U`. The version is re-detected after a successful upgrade. The age check can be disabled with
`update_check`.

### Privacy mode

Privacy mode (`privacy_mode`, or a profile's `privacy` flag) hides URLs, domains and titles in
notifications and failure logs; batch notifications still report success/failure counts. Rule34Video
is private by default. Failed links are still recorded so that retry keeps working.

## File layout

| File | Responsibility |
| --- | --- |
| `config.go` | `Config` struct, defaults, clamping, config/sites/log/retry/archive paths, atomic save, corrupt-config backup, legacy migration, failed-link storage |
| `sites.go` | `SiteProfile`, built-in profiles, `sites.json` parsing/merging, effective per-URL policy |
| `ytdlp.go` | yt-dlp detection and version age, brew-aware upgrade command, `BuildArgs` (all flags), progress template and phase mapping |
| `download.go` | Batch downloader: worker pool, connection budget, same-site limiter, process groups, auto-retry, 120 ms event coalescing, cancel/abort |
| `classify.go` | 12-way error classifier plus hints and retry policy |
| `logs.go` | Failure-log writing/pruning and the diagnostics report |
| `theme.go` | Semantic colour tokens, auto/dark/light palettes, `NO_COLOR`, Markdown styles |
| `layout.go` | Responsive frame (minimum 60×16, maximum content width 100), clipping, centring, scrolling helpers |
| `widgets.go` | Progress bars, table cells, key hints, status lines, spinner |
| `view_*.go` | Menu, input, settings, site profiles, download, results, update, changelog, help and edit views |
| `keys.go` | Per-screen key dispatch, help contents, mouse-wheel handling |
| `notify.go` | macOS notifications via `osascript` arguments |
| `main.go` | Bubble Tea model/state machine, startup flow (dependency check, notices, migration, version label) |
| `i18n.go` | Chinese / English translation tables |
| `markdown.go` | Dependency-free Markdown renderer for the in-app changelog |
| `util.go` / `platform.go` | Formatting and URL helpers; PATH augmentation, clipboard, open/reveal, process-group kill, browser detection |

## Build & development

| Command | Description |
| --- | --- |
| `make dev` | Run the gofmt check, lint and tests |
| `make build` | Build the binary; the version comes from `git describe --tags --always --dirty` |
| `make lint` | Run `go vet` static analysis |
| `make fmt-check` | Fail if any file is not gofmt-formatted |
| `make test` | Run the test suite (`go test -race -cover ./...`) |
| `make release VERSION=1.6.0` | Produce a trimmed, versioned release artifact under `dist/` |
| `make install` | Install with `go install` |
| `make clean` | Remove build artifacts |

Building requires Go 1.26 or newer. The source tree carries the current release version (`v1.6.0`)
as the default, and `-ldflags "-X main.version=..."` overrides it at build time (Makefile uses
`git describe`, the release workflow uses the tag). CI runs the gofmt check, `go vet`,
`go test -race -cover` and a build, and tag pushes publish unsigned macOS binaries for both
`darwin/arm64` and `darwin/amd64` together with SHA-256 checksums.

## Changelog

See [CHANGELOG.md](CHANGELOG.md), or browse it from the in-application "Changelog" menu.

## Contributing

Issues and pull requests are welcome. Before submitting, ensure that the working tree passes
`make lint` and `make test`.

## Acknowledgements

The overall visual style and terminal interface design of Queen are inspired by, and borrow heavily
from, the excellent open-source project **Mole**. Many thanks to the Mole authors for their great work.

## License

Released under the [MIT](LICENSE) license.

---

*Maintained by kesonglab.*
