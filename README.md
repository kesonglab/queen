# Queen

A macOS terminal downloader built on [yt-dlp](https://github.com/yt-dlp/yt-dlp), featuring a full-screen
terminal user interface (TUI) implemented with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
and [Lip Gloss](https://github.com/charmbracelet/lipgloss), and concurrent multi-task download support.

<p align="center">
<a href="/README.md"><b>English</b></a> · <a href="/README.zh-CN.md">中文</a>
</p>

<p align="center">
<img src="https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white">
<img src="https://img.shields.io/badge/License-MIT-blue?style=for-the-badge&logo=open%20source%20initiative&logoColor=white">
<img src="https://img.shields.io/badge/Platform-macOS-8a2be2?style=for-the-badge&logo=apple&logoColor=white">
<img src="https://img.shields.io/badge/Build-CI_passing-success?style=for-the-badge&logo=githubactions&logoColor=white">
<img src="https://img.shields.io/github/v/release/kesonglab/queen?style=for-the-badge&logo=github&logoColor=white">
<img src="https://img.shields.io/badge/yt--dlp-powered-cc0000?style=for-the-badge">
</p>

---

## Overview

Queen is a self-contained, single-install command-line front end for yt-dlp. It abstracts the
configuration and invocation of yt-dlp behind an interactive menu, and renders per-task progress
(percentage, size, throughput, and remaining time) with fixed-width alignment so that numeric
updates do not cause layout jitter. Multiple links are downloaded concurrently, each isolated in its
own worker, so a stalled task does not block the remainder of the batch.

## Features

- Full-screen terminal user interface (Bubble Tea / Lip Gloss).
- Concurrent multi-task downloads: independent worker per link, non-blocking progress.
- Real-time per-task progress: percentage, size, throughput, and estimated time remaining.
- Batch progress: aggregated progress bar, elapsed time, and projected total time.
- Automatic recording of failed links, with one-key retry.
- Configurable playlist mode, audio-only extraction (mp3), subtitles, metadata embedding, and cookie
  browser selection.
- Bilingual interface (Chinese / English).
- Native macOS notifications upon batch completion and per-task failure.
- Multiple visual themes (inherited from Mole).

## Requirements

| Requirement | Purpose |
| --- | --- |
| GNU/BSD [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) | Core download engine |
| [`ffmpeg`](https://ffmpeg.org) | Merging, transcoding, and thumbnail embedding |
| macOS | Target platform |
| [Homebrew](https://brew.sh) | Optional, for automatic dependency installation / upgrades |

`yt-dlp` and `ffmpeg` are detected at startup; if either is missing, Queen offers to install them
via Homebrew.

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

Because the binary is not signed, macOS may block the first launch. If it does, open **System Settings →
Privacy & Security** and choose *Open Anyway*, or right-click the file and select *Open*.

### Manual build

```bash
git clone https://github.com/kesonglab/queen.git
cd queen
make build
```

The equivalent explicit command is:

```bash
go build -ldflags "-X main.version=$(git describe --tags --always)" -o queen .
```

### go install

```bash
make install   # equivalent to: go install ./...
```

The application version is injected at build time via `-ldflags "-X main.version=..."` and is
therefore not duplicated in source.

## Usage

```bash
queen
```

From the main menu:

| Key | Action |
| --- | --- |
| `1` / `↑↓` / `Enter` | Select a menu item |
| `2` | Read links from the clipboard |
| `3` | Retry previously failed links |
| `Ctrl+D` | Start the download (inside the input area) |
| `q` / `Esc` | Go back / quit |
| `←→` | Toggle an option (on the settings page) |

Paste one link per line, then press `Ctrl+D` to begin a batch download.

## Configuration

The configuration file lives at `~/.config/videodl/config.json`. It can be edited through the
application's settings page or by hand:

| Field | Description |
| --- | --- |
| `download_dir` | Download directory (default `~/Downloads`) |
| `cookie_browser` | Cookie browser to use (auto-detect / chrome / firefox / safari / edge) |
| `playlist` | Whether to download as a playlist |
| `audio_only` | Extract audio only and transcode to mp3 |
| `subs` | Download subtitles (zh/en) and embed them |
| `embed` | Embed title / thumbnail metadata |
| `retry_times` | Retries per link (0–5) |
| `concurrency` | Maximum concurrent downloads (1–16) |
| `format` | yt-dlp format selector (default `bv*+ba/b`) |
| `merge_format` | Output container format (default `mp4`) |
| `lang` | Interface language (`zh` / `en`) |

Failed links are recorded to `~/Downloads/视频失败链接-failed_links.txt`.

## Development

| Command | Description |
| --- | --- |
| `make dev` | Run lint and tests |
| `make build` | Build the binary |
| `make lint` | Run `go vet` static analysis |
| `make test` | Run the test suite |
| `make release VERSION=1.3.0` | Produce a versioned release artifact |

## Changelog

See [CHANGELOG.md](CHANGELOG.md), or browse it from the in-application "Changelog" menu.

## Contributing

Issues and pull requests are welcome. Before submitting, ensure that the working tree passes
`make lint` and `make test`.

## License

Released under the [MIT](LICENSE) license.

---

*Maintained by kesonglab.*
