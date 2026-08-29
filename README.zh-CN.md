# Queen

一个基于 [yt-dlp](https://github.com/yt-dlp/yt-dlp) 的 macOS 终端视频下载器，全屏终端界面（TUI）由
[Bubble Tea](https://github.com/charmbracelet/bubbletea) 与 [Lip Gloss](https://github.com/charmbracelet/lipgloss)
构建，并支持多任务并发下载。

<p align="center">
<a href="/README.md">English</a> · <a href="/README.zh-CN.md"><b>中文</b></a>
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

## 概览

Queen 是 yt-dlp 的一个开箱即用、单文件安装的终端前端。它将 yt-dlp 的配置与调用抽象到交互式菜单之后，
并以定宽对齐的方式渲染单个任务的进度（百分比、大小、速度与剩余时间），使数值更新不再产生布局抖动。
多条链接并发下载、各自独立 worker，因此某个任务卡住时不会阻塞整批下载。

## 特性

- 全屏终端用户界面（Bubble Tea / Lip Gloss）。
- 并发多任务下载：每条链接独立 worker，进度彼此不阻塞。
- 单任务实时进度：百分比、大小、吞吐量、预估剩余时间。
- 批量进度：聚合进度条、已用时间与预估总时长。
- 自动记录失败链接，支持一键重试。
- 可配置播放列表模式、仅音频（mp3）、字幕、元数据嵌入与 Cookie 浏览器。
- 中英双语界面。
- 批量完成与单任务失败时发送原生 macOS 通知。
- 多种视觉主题（界面风格大量借鉴了 Mole）。

## 环境要求

| 依赖 | 用途 |
| --- | --- |
| [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) | 核心下载引擎 |
| [`ffmpeg`](https://ffmpeg.org) | 合并、转码与封面嵌入 |
| macOS | 目标平台 |
| [Homebrew](https://brew.sh) | 可选，用于自动安装依赖 / 升级 |

启动时会检测 `yt-dlp` 与 `ffmpeg`；若任一缺失，Queen 会询问是否通过 Homebrew 安装。

## 安装

### 下载预编译二进制

预编译二进制随每次 [release](https://github.com/kesonglab/queen/releases) 附带，分别面向 Apple Silicon
（`darwin/arm64`）与 Intel（`darwin/amd64`）Mac。下载对应 zip、解压，然后在解压目录中运行 `queen`：

```bash
arch=$(uname -m); [ "$arch" = "x86_64" ] && arch=amd64
curl -fsSL -o queen.zip \
  https://github.com/kesonglab/queen/releases/latest/download/queen-darwin-$arch.zip
unzip queen.zip
./queen
```

由于二进制未签名，macOS 可能在首次启动时拦截。若是，请打开 **系统设置 → 隐私与安全性** 选择
*仍要打开*，或右键该文件选择 *打开*。

### 手动构建

```bash
git clone https://github.com/kesonglab/queen.git
cd queen
make build
```

等价命令：

```bash
go build -ldflags "-X main.version=$(git describe --tags --always)" -o queen .
```

### go install

```bash
make install   # 等价于：go install ./...
```

应用版本在构建时通过 `-ldflags "-X main.version=..."` 注入，因此在源码中不会重复维护。

## 使用

```bash
queen
```

主菜单按键说明：

| 按键 | 功能 |
| --- | --- |
| `1` / `↑↓` / `Enter` | 选择菜单项 |
| `2` | 从剪贴板读取链接 |
| `3` | 重试失败链接 |
| `Ctrl+D` | 开始下载（在输入区域内） |
| `q` / `Esc` | 返回 / 退出 |
| `←→` | 切换选项（在设置页） |

每行粘贴一个链接，然后按 `Ctrl+D` 开始批量下载。

## 配置

配置文件位于 `~/.config/videodl/config.json`。可在应用的设置页编辑，也可手动修改：

| 字段 | 说明 |
| --- | --- |
| `download_dir` | 下载目录（默认 `~/Downloads`） |
| `cookie_browser` | 使用的 Cookie 浏览器（自动检测 / chrome / firefox / safari / edge） |
| `playlist` | 是否以播放列表方式下载 |
| `audio_only` | 仅提取音频并转码为 mp3 |
| `subs` | 下载字幕（zh/en）并嵌入 |
| `embed` | 嵌入标题 / 封面元数据 |
| `retry_times` | 每链接重试次数（0–5） |
| `concurrency` | 最大并发下载数（1–16） |
| `format` | yt-dlp 格式选择器（默认 `bv*+ba/b`） |
| `merge_format` | 输出容器格式（默认 `mp4`） |
| `lang` | 界面语言（`zh` / `en`） |

失败链接记录在 `~/Downloads/视频失败链接-failed_links.txt`。

## 开发

| 命令 | 说明 |
| --- | --- |
| `make dev` | 运行 lint 与测试 |
| `make build` | 构建二进制 |
| `make lint` | 运行 `go vet` 静态检查 |
| `make test` | 运行测试套件 |
| `make release VERSION=1.3.0` | 生成带版本号的 release 产物 |

## 更新日志

参见 [CHANGELOG.md](CHANGELOG.md)，或在应用内「更新日志」菜单中阅读。

## 贡献

欢迎提交 Issue 与 Pull Request。提交前请确保工作区通过 `make lint` 与 `make test`。

## 致谢

Queen 的整体视觉风格与终端界面设计深受开源项目 **Mole** 的启发，并大量借鉴其做法。在此向 Mole 作者表示诚挚感谢。

## 许可证

以 [MIT](LICENSE) 许可证发布。

---

*由 kesonglab 维护。*
