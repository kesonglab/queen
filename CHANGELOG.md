# Changelog

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
