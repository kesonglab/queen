# Changelog

## v1.6.0 (2026-11-19)

### Added
- Full interface redesign: a semantic theme system (`auto` / `dark` / `light`, `Ctrl+T` to cycle,
  `NO_COLOR` for plain text), a responsive layout (content capped at 100 columns and centred, with a
  friendly resize screen below 60x16), scrollable lists, a bottom status bar and a `?` help overlay.
- Results page: a per-task table (site, task, size, duration, failure reason) with one-key actions —
  `r` retry failed tasks, `o` open the download folder, `c` copy a diagnostics report and `l` open
  the failure-log folder.
- Per-task phase labels (extracting / downloading / merging / converting / embedding / metadata /
  fixing), playlist progress (`item N/M`) and fragment progress (`frag N/M`) when a stream has no
  known total size.
- Connection budget and same-site parallel limit: all tasks share a global connection budget and at
  most 2 tasks hit the same site by default (both configurable).
- Built-in site profiles for YouTube, Bilibili, X/Twitter, Instagram, Facebook, TikTok, Douyin,
  Twitch, Vimeo, Reddit, Niconico, SoundCloud and Rule34Video.
- User site overrides in `~/.config/queen/sites.json` (top-level array or `{"profiles":[...]}`),
  matched by domain suffix and merged field by field over the built-in profiles.
- Per-site cookie strategies (`auto` / `off` / browser name / `cookies_file`), TLS impersonation
  (`--impersonate`), `--extractor-args`, `Referer` and `User-Agent` overrides.
- Error classifier with 12 categories and actionable hints, plus one automatic retry: browser cookies
  for auth/membership failures and `--impersonate chrome` for Cloudflare challenges.
- Failure logs under `~/.local/state/queen/logs` (latest 200 kept), a "copy diagnostics" report and
  an "open logs folder" action on the results page.
- yt-dlp freshness check (warn once the installed release is older than 45 days) with a one-key
  upgrade page that uses `brew upgrade yt-dlp` for Homebrew installs and `yt-dlp -U` otherwise.
- Privacy mode hides URLs, domains and titles in notifications and failure logs; Rule34Video enables
  it by default.
- Mouse-wheel scrolling on the main menu, lists and pages.
- The in-app changelog now renders Markdown (headings, lists, inline code and links) instead of raw
  source.
- Single-task cancel (`x`) in the download view; abort-all stays on `q` / `Esc`.
- Optional download archive (`--download-archive`, stored as `<download_dir>/.queen-archive.txt`).
- `--version` / `-v` and `--help` / `-h` command-line flags that work without an interactive terminal.

### Changed
- Download progress is now parsed from structured `--progress-template` records with fixed
  separators instead of regular expressions over human-readable text; stdout and stderr are read on
  separate pipes and progress is coalesced every 120 ms.
- Audio-only mode now extracts to `m4a` with `--audio-quality 0` by default instead of forcing an MP3
  transcode; `mp3`, `opus`, `flac` and `best` remain selectable.
- Metadata/thumbnail embedding is off by default, since it costs an extra network request and ffmpeg
  pass per file.
- Playlist mode is off by default; when enabled, items are stored in a playlist-named subfolder with
  zero-padded indices.
- Default network profile retuned: 8 fragment concurrency per task (was 16), global connection budget
  32, same-site limit 2, 2 retries, 3-second retry sleep, `--throttled-rate 100K`,
  `--trim-filenames 200` and `--ignore-config` enabled.
- Format selection is split into `quality` / `compat` / `custom` presets; the compatibility preset
  prefers H.264 + AAC and MP4, and the custom format selector is editable.
- The settings page is reorganised into Content / Quality & container / Network & performance /
  Auth & sites / General sections with a scrolling list and a text-editor overlay.
- Version display is unified: builds with no injected version show `dev`, an injected `1.6.0` shows
  `v1.6.0`, an injected `v1.6.0` is not double-prefixed, and `make build` derives the version from
  `git describe`.
- Configuration is written atomically (`config.json`, mode `0600`); a corrupt file is backed up to
  `config.json.corrupt-<timestamp>` and defaults are restored with a toast; save errors are reported.

### Fixed
- Failure reasons now show the real yt-dlp error text instead of `exit status 1`.
- Toasts appear on every screen and expire automatically (4 seconds for notices, 6 seconds for
  errors) instead of being invisible or lingering forever.
- Batch progress distinguishes successes from failures with separate counters and colours instead of
  counting failed tasks as completed.
- A single very long output line can no longer stall the pipe and freeze the interface.
- Post-processing phases (merge, convert, embed) are shown explicitly instead of the progress bar
  appearing stuck at 100%.
- Final file sizes are read from the produced file after merging/conversion, so the results table
  shows the real size.
- A batch started right after aborting the previous one no longer receives stale messages from the
  old batch (events are isolated by batch ID).
- Notifications containing quotes or backslashes are delivered correctly, and repeated failures for
  the same URL are throttled to avoid notification floods.

### Removed
- Removed the experimental `--http-chunk-size` tuning (and the fixed `--buffer-size` override),
  which could break range requests and signed CDN URLs.
- Removed the regular-expression parser for yt-dlp's human-readable progress output.
- Removed the old bilingual failed-links filename `视频失败链接-failed_links.txt`; existing files are
  migrated to `failed_links.txt` automatically.

## v1.4.0 (2026-10-05)

### Added
- Settings page can cycle the yt-dlp format selector and merge container (`mp4` / `mkv` / `webm`).
- Download view truncates overflow tasks and shows a "N more tasks" summary.

### Fixed
- Settings "Retries per link" now actually passed to yt-dlp (`--retries` / `--fragment-retries`);
  previously the UI value was ignored and retries were hard-coded to 5.
- Concurrent append/remove of the failed-links file is now mutex-protected to avoid lost or
  corrupted entries under high concurrency.
- User abort (`q` / `Esc`) no longer marks tasks as Failed or writes them to the retry file;
  queued tasks are marked Aborted instead of staying Pending.
- Multi-stream downloads (`bestvideo+bestaudio`) no longer freeze the progress bar at 100% after
  the first stream; a new Destination / stream resets the tracker.
- Abort now kills the whole process group so ffmpeg children are not left behind.
- Chinese changelog scrolling used the English line count; it now uses the active language content.
- Update-yt-dlp screen spinner now animates.
- Brave / Chromium are detected as `brave` / `chromium` (not mislabeled as `chrome`).
- Config concurrency / retry values are clamped on load and at runtime.
- `renderBar` no longer panics on negative percentages; `cleanErr` truncates on rune boundaries.

### Changed
- Config directory moved to `~/.config/queen/` (auto-migrates from `~/.config/videodl/`).
- Failed-links file is stored under the configured download directory.
- Cookie browser options include chromium and brave.
- Settings rows are identified by id rather than numeric index (format/merge can be inserted without breaking Enter / ←→).
- Fragment concurrency scales down when task concurrency is high (avoids 16×16 overload).
- Config and retry files are written with mode `0600`.

## v1.3.0 (2026-08-30)

### Added
- Best-quality support for X.com / Twitter videos: when a link is an X/Twitter (or `t.co`) URL,
  Queen now auto-detects a local browser and passes its cookies to yt-dlp. This unlocks the highest
  quality stream X serves (anonymous requests are capped — many videos throw "Requires authentication"),
  and fixes downloads of auth-gated / age-limited content.
- Default format selector is now the explicit "best quality" form `bestvideo*+bestaudio/best`.

## v1.2.2 (2026-08-29)

### Docs
- Documented that the visual / TUI style is borrowed from the open-source project Mole, added an
  Acknowledgements section, and noted the same in the README and this changelog.

## v1.2.1 (2026-08-22)

### Added
- Desktop notifications: batch completion (with success/failure counts) and per-task failure are
  surfaced through the macOS notification center with an accompanying alert sound.
- Throughput tuning: fragment concurrency (`--concurrent-fragments`) raised from 4 to 16, with
  `--http-chunk-size 10M` and `--buffer-size 16K`; the default concurrent download count raised from
  3 to 4.

### Fixed
- Corrected the settings page mapping for the language toggle: the language row was being bound to the
  wrong index (overlapping the download-directory row), so the directory row silently changed the
  language while the language row had no effect. The mapping is now correct, and the concurrency upper
  bound is relaxed to 16.

### Stack
- Go + BubbleTea + Lip Gloss.
- Configuration file: `~/.config/videodl/config.json`.

## v1.2.0 (2026-08-22)

### Added
- Concurrent multi-task downloads: the concurrency count is configurable in Settings (1–8, default 3),
  allowing several links to download in parallel without blocking one another.
- The download view now renders all in-progress / completed / failed tasks in real time; remaining
  tasks are summarized as "queued × N".

### Fixed
- Resolved an intermittent "frozen" download state: during progress-silent phases (e.g. ffmpeg merging
  or thumbnail embedding) the UI no longer stops repainting, and the spinner continues to animate.
- Resolved missing progress / throughput / remaining-time values: the previous regular expressions
  required a fixed speed unit and `MM:SS` formatting, so yt-dlp's `Unknown B/s`, `ETA Unknown` and the
  `HH:MM:SS` completion line all failed to match. Parsing is now tolerant, leaving unknown fields blank
  instead of discarding the whole line.
- Fixed the abort logic so that quitting terminates all running yt-dlp processes.
- Task state is now accumulated across updates, preventing loss of fields such as the title when a
  progress line is rebuilt.

### Stack
- Go + BubbleTea + Lip Gloss.
- Configuration file: `~/.config/videodl/config.json`.

## v1.1.0 (2026-08-19)

### Added
- Renamed the interface to Queen with a refreshed visual style; the style and TUI design were borrowed
  from the open-source project Mole.
- Batch download: aggregated progress bar, elapsed time, and projected remaining time.
- In-application changelog viewer (this file, readable from the TUI menu).
- Bilingual interface (Chinese / English, switchable in Settings).
- Automatic dependency detection with a one-key install prompt when missing.
- Download settings: playlist / audio-only (mp3) / subtitles / metadata embedding / cookie browser /
  retry count / download directory.
- Failed-link recording with one-key retry.
- Real-time per-task progress: percentage, size, throughput, and remaining time.

### Fixed
- Numeric progress updates no longer cause horizontal layout jitter (fixed-width alignment).

### Stack
- Go + BubbleTea + Lip Gloss, and the TUI style borrowed from Mole.
- Configuration file: `~/.config/videodl/config.json`.
- Failure log: `~/Downloads/视频失败链接-failed_links.txt`.

## v1.0.0 (2026-08-19)
- Initial release, migrated from the bash-based video download script.
