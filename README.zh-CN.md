# Queen

一个基于 [yt-dlp](https://github.com/yt-dlp/yt-dlp) 的 macOS 终端视频下载器，全屏终端界面（TUI）由
[Bubble Tea](https://github.com/charmbracelet/bubbletea) 与 [Lip Gloss](https://github.com/charmbracelet/lipgloss)
构建，支持多任务并发下载，并内置按站点生效的兼容策略。

<p align="center">
<a href="/README.md">English</a> · <a href="/README.zh-CN.md"><b>中文</b></a>
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

## 概览

Queen 是 yt-dlp 的一个开箱即用、单文件安装的终端前端。它把 yt-dlp 的配置与调用封装在交互式菜单之后，
并把每个下载渲染成一条实时任务：独立的阶段（解析、下载、合并、转码、嵌入元数据、修复容器）、百分比、
大小、速度与剩余时间一应俱全。多条链接各自在独立的 worker 与进程组中并发下载，因此某个任务卡住或失败
不会阻塞整批下载。

v1.6.0 的界面是一次完整重做：主题系统（auto / dark / light）、从 60×16 小终端到超宽屏都可读的响应式
布局、可滚动列表、状态栏、帮助覆盖层，以及带一键后续操作的结果表格。下载管线改为消费 yt-dlp 的结构化
进度输出，不再解析人类可读文本；站点档案层按站点提供 Cookie、浏览器指纹与并发参数。失败会被归入 12 个
可操作的分类，必要时用一种对应手段自动重试一次，并写入可直接粘贴进 issue 的失败日志。

## 特性

### 界面

- 全屏 TUI 与语义化主题系统：`auto` 跟随终端背景，另有显式的 `dark` 与 `light` 调色板。任意界面按
  `Ctrl+T` 循环切换主题；`NO_COLOR` 环境变量会关闭彩色输出。
- 响应式布局：内容区最大 100 列并在宽终端上水平居中；终端小于 60×16 时显示友好的尺寸提示，而不是
  错乱的界面。
- 可滚动的任务列表与设置页。设置按「内容 / 画质与容器 / 网络与性能 / 认证与站点 / 通用」分节，操作项
  位于底部；文本值在单行编辑弹层中修改（Enter 保存、Esc 取消、Ctrl+U 清空）。
- 底部状态栏显示当前界面的可用按键；顶部标题行显示 Queen 版本、当前界面、yt-dlp 版本（过旧时带警告）
  与当前并发数。
- 下载页：每个任务带阶段标签，进行中的任务占两行（进度条、大小、速度、剩余时间），并显示播放列表位置
  （`列表 N/M`）与无总大小时的 `frag N/M` 分片进度；支持状态筛选（全部 / 进行中 / 失败）、单任务取消
  （`x`）与中止全部（`q` / `Esc`）。
- 结果页：按任务列出站点、任务名、大小、耗时与失败原因的表格，并提供一键操作——重试失败任务（`r`）、
  打开下载目录（`o`）、复制诊断信息（`c`）、打开失败日志目录（`l`）。
- `?` 在除输入页之外的所有界面打开可滚动的快捷键帮助覆盖层；应用内更新日志按 Markdown 渲染，而不
  是显示原始源码。
- Toast 提示在所有界面都会渲染，并会自动过期（普通提示 4 秒，错误提示 6 秒）。

### 下载管线

- 结构化进度：调用 yt-dlp 时传入 `--progress-template` 模板，字段以固定 `|` 分隔且都是数值或枚举值，
  Queen 不再用正则解析人类可读的进度文本。
- stdout 与 stderr 分别用独立管道读取；进度在送达 UI 之前按 120 ms 合并，每个批次都带批次 ID，上一批次
  的消息会被丢弃。
- 每个任务都是一个独立的 yt-dlp 进程，并处于独立进程组；中止批次（或取消单个任务）会杀掉整个进程组，
  包括 ffmpeg 子进程。
- 任务成功后用 `stat` 读取产物文件的真实大小并显示在结果表格中，而不是沿用估算值。
- 输出通过带缓冲的读取器逐行读取；单行特别长时不再可能写满管道导致卡死。

### 性能与网络

- 移除了实验性的 `--http-chunk-size` 调优——它可能破坏 Range 请求与带签名的 CDN URL。
- 新增反限速与重试控制：`--throttled-rate 100K`（流被限速时自动重新提取）、`--retry-sleep 3`
  （429/503 后等待再重试，而不是立刻重试）、`--ignore-config`（全局 yt-dlp 配置无法再覆盖 Queen 的
  参数）与 `--trim-filenames 200`（让长中文标题不超出 macOS 文件名上限）。
- 全局连接预算：每个任务从共享池（默认 32 条连接）申请自己的 `--concurrent-fragments` 额度。同站并行
  上限（默认每个域名 2 个任务）防止把某个站点打到限流；站点档案可以为具体站点调高或调低分片并发。
- 仅音频模式默认以 `--audio-quality 0` 提取为 `m4a`；`mp3`、`opus`、`flac`、`best` 仍可选择，默认不再
  强制有损二次编码。
- 元数据/封面嵌入（`--embed-metadata --embed-thumbnail`）默认关闭，因为它会让每个文件多一次网络请求与
  ffmpeg 处理。
- 播放列表模式默认关闭：粘贴一个恰好带列表 ID 的链接时只下载该条目。开启后，列表条目会放进以列表名
  命名的子目录，并按零填充序号命名。
- 重试策略：`--retries` / `--fragment-retries` 跟随配置的重试次数，`--extractor-retries` 至少为 2，
  `--socket-timeout` 为 30 秒，`--file-access-retries` 为 3。

### 站点兼容与诊断

- 内置站点档案：YouTube、Bilibili、X/Twitter、Instagram、Facebook、TikTok、抖音、Twitch、Vimeo、
  Reddit、Niconico、SoundCloud、Rule34Video；可在 `~/.config/queen/sites.json` 中覆盖或扩展。
- 按站点（或全局）配置 Cookie 策略：自动探测本机浏览器、关闭 Cookie、指定浏览器，或指向 Netscape
  `cookies.txt`；另支持 TLS 指纹（`--impersonate`）、`--extractor-args`、Referer 与 User-Agent 覆盖。
- 12 类错误分类器 + 一次自动重试：auth/membership 类错误自动改用浏览器 Cookie 重试，Cloudflare 类错误
  自动改用 `--impersonate chrome` 重试。
- 失败日志位于 `~/.local/state/queen/logs`（保留最近 200 个）；结果页可一键复制诊断信息；失败链接文件
  支持菜单一键重试。
- yt-dlp 新鲜度检查：超过 45 天未更新会在启动提示、标题行与主菜单中标记，并提供一键升级页；用 Homebrew
  安装的走 brew 升级。
- 隐私模式会在通知与失败日志中隐藏 URL、域名与标题；Rule34Video 默认开启隐私。
- 明确的能力边界：Queen 下载 yt-dlp 能提取的公开、非 DRM 媒体。Netflix、Disney+、Prime Video、
  Apple TV+、HBO Max、Spotify 等 DRM 流媒体无法下载。

### 其他

- 中英双语界面与双语 Markdown 更新日志。
- 批量完成与单任务失败时发送原生 macOS 通知，带提示音与每 URL 节流；通过 `osascript` 参数传递文本，
  引号与反斜杠都安全。
- 失败链接自动去重、可从菜单重试，并在之后成功时自动移除。
- 可选的下载归档（`--download-archive`）用于跳过已下载条目。
- 依赖自动检测：缺少 `yt-dlp` / `ffmpeg` 时可用 Homebrew 安装。
- Mole 风格的终端视觉界面。

## 环境要求

| 依赖 | 用途 |
| --- | --- |
| [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) | 核心下载引擎 |
| [`ffmpeg`](https://ffmpeg.org) | 合并、音频转换、字幕/封面嵌入 |
| macOS | 目标平台 |
| [Homebrew](https://brew.sh) | 可选，用于自动安装依赖与升级 yt-dlp |

启动时会检测 `yt-dlp` 与 `ffmpeg`；若任一缺失，Queen 会询问是否通过 Homebrew 安装。从源码构建还需要
Go 1.26 或更新版本。

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

### 从源码构建

```bash
git clone https://github.com/kesonglab/queen.git
cd queen
make build
```

`make build` 执行 `go build`，并从 `git describe --tags --always --dirty` 注入版本号，因此打了 tag 的
检出会构建出 `vX.Y.Z`（工作区有改动时带 `-dirty` 后缀）。等价的显式命令：

```bash
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" -o queen .
```

源码内置当前发布版本 `v1.6.0`，因此直接 `go build` 也会显示 `v1.6.0`；注入 `1.6.0` 同样显示
`v1.6.0`，注入 `v1.6.0` 则原样显示；显式设置 `dev` 时仍显示 `dev`。

### go install

```bash
make install   # 等价于：go install ./...
```

## 使用

```bash
queen
```

有两个参数无需交互式终端即可使用：

| 参数 | 作用 |
| --- | --- |
| `--version` / `-v` | 打印版本号（例如 `v1.6.0`）后退出 |
| `--help` / `-h` | 打印用法后退出 |

Queen 启动后进入主菜单：

| 按键 | 菜单项 |
| --- | --- |
| `1` | 粘贴链接下载（每行一个，`Ctrl+D` 开始） |
| `2` | 从剪贴板读取链接 |
| `3` | 重试上次失败的链接 |
| `4` | 下载设置 |
| `5` | 站点档案 |
| `6` | 打开下载目录 |
| `7` | 更新 yt-dlp |
| `8` | 更新日志 |
| `q` | 退出 |

一次典型的批量下载：

1. 选择 `1`，粘贴一个或多个链接，按 `Ctrl+D`。
2. 观察每个任务的阶段（解析 / 下载 / 合并 / 转码 / 嵌入）、进度条、大小、速度与剩余时间，以及列表下方
   的聚合批量面板。
3. 批次结束后查看结果表格：`r` 重试失败任务、`o` 打开下载目录、`c` 复制诊断信息、`l` 打开失败日志
   目录。
4. `q` / `Esc` 中止正在运行的批次；`x` 只取消当前选中的任务。

设置既可在应用内（`4`）修改，也可直接编辑 `~/.config/queen/config.json`。

## 键盘快捷键

### 全局

| 按键 | 功能 |
| --- | --- |
| `Ctrl+C` | 退出（若正在下载，会先中止批次） |
| `?` | 打开/关闭帮助覆盖层 |
| `Ctrl+T` | 循环切换主题：auto → dark → light |
| 鼠标滚轮 | 滚动主菜单、设置、下载列表、结果表格、站点档案、更新日志与帮助 |

### 主菜单

| 按键 | 功能 |
| --- | --- |
| `↑↓` / `j` `k` | 移动 |
| `1`–`8` | 直接选择菜单项 |
| `Enter` / `Space` | 确认 |
| `q` | 退出 |

### 输入

| 按键 | 功能 |
| --- | --- |
| `Ctrl+D` | 确认：开始批量下载 / 应用下载目录 |
| `Esc` | 返回 |

修改下载目录时，路径必须是绝对路径（以 `/` 或 `~` 开头）。

### 设置

| 按键 | 功能 |
| --- | --- |
| `↑↓` / `j` `k` | 移动 |
| `PgUp` / `PgDn` | 上翻 / 下翻页 |
| `g` / `G` | 跳到开头 / 末尾 |
| `←→` / `h` `l` | 修改取值（文本项会打开编辑器） |
| `Enter` | 编辑文本项、前进一个选项，或执行操作项 |
| `Esc` | 保存并返回 |

文本项会打开单行编辑弹层：Enter 保存，Esc 取消，Ctrl+U 清空。

### 下载

| 按键 | 功能 |
| --- | --- |
| `↑↓` / `j` `k` | 移动 |
| `PgUp` / `PgDn` | 上翻 / 下翻页 |
| `g` / `G` | 跳到开头 / 末尾 |
| `x` | 取消选中的任务 |
| `f` | 循环切换筛选：全部 → 进行中 → 失败 |
| `q` / `Esc` | 中止整个批次 |

### 结果

| 按键 | 功能 |
| --- | --- |
| `↑↓` / `j` `k` / `PgUp` `PgDn` / `g` `G` | 滚动表格 |
| `r` | 重试所有失败任务 |
| `o` | 打开下载目录 |
| `c` | 复制诊断报告 |
| `l` | 打开失败日志目录 |
| `Enter` / `q` / `Esc` | 返回主菜单 |

其余界面（站点档案、更新 yt-dlp、更新日志）用 `↑↓` / `j` `k` 滚动、`Esc` 返回；帮助覆盖层中 `?` 或
`Esc` 关闭，`j` / `k` 滚动。

## 配置

配置文件位于 `~/.config/queen/config.json`。首次启动会按内置默认值创建，若存在旧路径
`~/.config/videodl/config.json` 会自动迁移；既可在应用内设置页（`4`）编辑，也可手动修改。

- 写入采用原子方式（临时文件 + rename），文件权限为 `0600`。
- 配置损坏时会备份为 `config.json.corrupt-<时间戳>` 并恢复默认值，同时用 toast 说明发生了什么；保存
  失败会以 toast 提示，而不是被静默吞掉。
- 超出文档范围的值会在加载时以及每次修改后自动收敛到合法范围。

### 通用

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `config_version` | `2` | 配置结构版本；由程序自动维护 |
| `download_dir` | `~/Downloads` | 下载目录（`~` 会被展开） |
| `lang` | `"zh"` | 界面语言：`zh` 或 `en` |
| `theme` | `"auto"` | 主题：`auto`（跟随终端）/ `dark` / `light`；`Ctrl+T` 可循环切换 |

### 内容

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `playlist` | `false` | 链接带播放列表时下载整个列表（`--yes-playlist`）；默认关闭以免误下整个列表 |
| `playlist_folder` | `true` | 开启播放列表模式时，把条目放进以列表名命名的子目录，并按序号零填充命名 |
| `audio_only` | `false` | 仅提取音频（`-x --audio-format <audio_format> --audio-quality 0`） |
| `audio_format` | `"m4a"` | 音频容器：`m4a` / `mp3` / `opus` / `flac` / `best` |
| `subs` | `false` | 下载字幕并嵌入（`--write-subs --embed-subs`） |
| `auto_subs` | `false` | 同时下载自动生成字幕（`--write-auto-subs`） |
| `sub_langs` | `"zh.*,en.*"` | 字幕语言过滤（`--sub-langs` 语法） |
| `embed` | `false` | 嵌入元数据与封面（`--embed-metadata --embed-thumbnail`）；默认关闭以提升速度 |

### 画质与容器

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `format_preset` | `"quality"` | 画质预设：`quality`（`bv*+ba/b`）、`compat`（优先 H.264 + AAC / MP4）或 `custom` |
| `format` | `"bv*+ba/b"` | 自定义 yt-dlp 格式选择器，仅在 `format_preset` 为 `custom` 时使用 |
| `merge_format` | `"mp4"` | 合并音视频的容器：`mp4` / `mkv` / `webm`（`--merge-output-format`） |

### 网络与性能

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `concurrency` | `4` | 并行下载任务数（1–16） |
| `fragments` | `8` | 每个任务请求的分片并发数（`--concurrent-fragments`，1–16；从连接预算中借用，可按站点覆盖） |
| `connection_budget` | `32` | 所有任务共享的全局连接预算（4–64） |
| `same_site_limit` | `2` | 同一站点允许的并行任务上限（1–8） |
| `retry_times` | `2` | 每链接重试次数（`--retries` / `--fragment-retries`，0–10） |
| `retry_sleep` | `3` | 两次重试之间的等待秒数（`--retry-sleep`） |
| `throttled_rate` | `"100K"` | 吞吐持续低于该值时重新提取（`--throttled-rate`）；留空关闭 |
| `limit_rate` | `""` | 限速（`--limit-rate`），例如 `2M`；留空表示不限速 |
| `proxy` | `""` | 传给 `--proxy` 的代理，例如 `http://127.0.0.1:7890` |

### 认证与站点覆盖

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `cookie_browser` | `""` | Cookie 来源：空 = 自动探测浏览器，或 `chrome` / `chromium` / `brave` / `firefox` / `safari` / `edge`（`--cookies-from-browser`） |
| `cookies_file` | `""` | Netscape `cookies.txt` 路径（`--cookies`）；优先于浏览器 Cookie |
| `impersonate` | `""` | 传给 `--impersonate` 的 TLS 指纹：`chrome` / `chrome-136` / `chrome-133` / `safari` / `safari-ios`；留空关闭 |
| `extractor_args` | `""` | 高级 `--extractor-args` 透传 |
| `user_agent` | `""` | 自定义 `User-Agent`（`--user-agent`） |
| `referer` | `""` | 自定义 `Referer`（`--referer`） |

### 文件、隐私与启动

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `trim_filenames` | `200` | 文件名长度上限（`--trim-filenames`）；`0` = 不限，否则 60–240 |
| `ignore_ytdlp_config` | `true` | 传 `--ignore-config`，避免全局 yt-dlp 配置覆盖 Queen 的参数 |
| `download_archive` | `false` | 启用 `--download-archive` 跳过已下载条目；归档文件为 `<download_dir>/.queen-archive.txt` |
| `privacy_mode` | `false` | 在通知与失败日志中隐藏 URL、域名与标题 |
| `keep_failure_logs` | `true` | 失败日志保存到 `~/.local/state/queen/logs`（保留最近 200 个） |
| `update_check` | `true` | 启动时若 yt-dlp 超过 45 天未更新则提示 |

相关路径：

| 路径 | 含义 |
| --- | --- |
| `~/.config/queen/config.json` | 配置文件 |
| `~/.config/queen/sites.json` | 可选的用户站点档案覆盖 |
| `<download_dir>/failed_links.txt` | 失败链接，每行一个（旧文件名会自动迁移） |
| `<download_dir>/.queen-archive.txt` | 下载归档，启用 `download_archive` 时使用 |
| `~/.local/state/queen/logs/` | 失败日志 |

## 站点档案与兼容性

Queen 自己不实现站点提取器：提取完全交给 yt-dlp（支持上千个站点）。Queen 增加的是按站点生效的策略层：
对每条链接，它会用主机名在站点档案表中查找，把命中的档案与全局配置合并，再据此构建 yt-dlp 命令行。

内置档案覆盖最常见的视频站点，并给出合理默认值：通常需要登录的站点启用 Cookie，位于 Cloudflare 类
防护之后的站点启用浏览器指纹，能从分片并发受益的站点提高并发，成人站点启用隐私。

站点档案界面可从主菜单 `5` 或设置页底部的「查看站点档案…」操作进入；`Esc` 返回进入前的界面。界面列出
所有内置与用户档案及其匹配域名、策略与标记，并显示 `sites.json` 的路径与解析警告。

### 内置站点档案

| 档案 | 匹配域名 | 内置策略 |
| --- | --- | --- |
| YouTube | `youtube.com`、`youtu.be`、`youtube-nocookie.com` | 16 分片并发 |
| Bilibili | `bilibili.com`、`b23.tv` | Cookie auto、Referer `https://www.bilibili.com`、8 分片 |
| X / Twitter | `x.com`、`twitter.com`、`t.co` | Cookie auto |
| Instagram | `instagram.com`、`instagr.am` | Cookie auto |
| Facebook | `facebook.com`、`fb.watch`、`fb.com` | Cookie auto |
| TikTok | `tiktok.com` | Cookie auto、Referer `https://www.tiktok.com/` |
| 抖音 | `douyin.com`、`iesdouyin.com` | Cookie auto、Referer `https://www.douyin.com/` |
| Twitch | `twitch.tv` | 8 分片并发 |
| Vimeo | `vimeo.com` | 继承全局设置 |
| Reddit | `reddit.com`、`redd.it`、`v.redd.it` | Cookie auto |
| Niconico | `nicovideo.jp`、`nico.ms` | Cookie auto |
| SoundCloud | `soundcloud.com` | 继承全局设置 |
| Rule34Video | `rule34video.com` | Cookie auto、指纹 `chrome`、不嵌封面、隐私、4 分片 |

### 用户覆盖（`~/.config/queen/sites.json`）

创建 `~/.config/queen/sites.json` 可以新增档案或覆盖内置档案。文件既接受顶层 JSON 数组，也接受带
`profiles` 数组的对象：

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

匹配与合并规则：

- `match` 是域名后缀列表。后缀既匹配域名本身，也匹配其所有子域名，例如 `youtube.com` 匹配
  `www.youtube.com` 与 `m.youtube.com`；多个档案命中时，命中的最长后缀优先。
- 用户档案合并到命中的内置档案之上：`name` 与 `match` 会替换内置值，非空/非零字段会覆盖，`embed`
  可以显式设为 `false`。
- `cookies_file` 中的 `~` 会展开为用户主目录。

### 档案字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `name` | string | 站点档案界面显示的名称（可选） |
| `match` | string 数组 | 匹配的域名后缀；命中的最长后缀优先 |
| `cookies` | string | Cookie 策略：`""` 继承全局设置、`auto` 自动探测已安装浏览器、`off` 不使用 Cookie，或浏览器名（`chrome`、`chromium`、`brave`、`firefox`、`safari`、`edge`） |
| `cookies_file` | string | Netscape `cookies.txt` 路径；优先于 `cookies` |
| `impersonate` | string | 传给 `--impersonate` 的 TLS 指纹（`chrome`、`chrome-136`、`chrome-133`、`safari`、`safari-ios`）；留空关闭 |
| `extractor_args` | string | 额外的 `--extractor-args` 值 |
| `referer` | string | 自定义 `Referer` 请求头 |
| `user_agent` | string | 自定义 `User-Agent` 请求头 |
| `fragments` | integer | 该站点的分片并发数；`0` = 使用全局值 |
| `embed` | boolean | 覆盖该站点的元数据/封面嵌入；设为 `false` 可关闭 |
| `privacy` | boolean | 在该站点的通知与失败日志中隐藏 URL、域名与标题 |

### Cookie、浏览器指纹与高级透传

- Cookie 优先级：`cookies_file` 永远优先；否则由站点的 `cookies` 决定。`auto` 会先使用已配置的浏览器，
  没有配置则按 Chrome → Chromium → Brave → Firefox → Edge → Safari 的顺序自动探测。浏览器名按原样
  使用，`off` 对该站点禁用 Cookie，空值则回落到全局 `cookie_browser`。
- 浏览器 Cookie 通过 `--cookies-from-browser` 读取，其中包含该浏览器的登录态。请只在自己信任的机器上
  开启，并优先使用按站点档案而不是全局 Cookie 浏览器。
- TLS 指纹（`--impersonate`）会让 yt-dlp 呈现真实浏览器的 TLS/HTTP 指纹，是应对 Cloudflare 类人机
  校验的常用手段；但它可能影响吞吐，所以默认关闭、可按站点开启。可用的指纹目标取决于本机 yt-dlp 版本，
  用 `yt-dlp --list-impersonate-targets` 查看。
- `extractor_args`、`referer` 与 `user_agent` 是高级逃生口：当某站点的提取器需要 client 提示，或 CDN
  需要来自嵌入页的 Referer 时使用。
- DRM 边界：带 DRM 的流媒体（Netflix、Disney+、Prime Video、Apple TV+、HBO Max、Spotify 等）使用
  Widevine/FairPlay/PlayReady，yt-dlp 无法下载。Queen 会把它们归类为 `DRM` 并给出提示，而不是反复
  重试。yt-dlp 没有提取器的站点同理——先升级 yt-dlp。

## 失败诊断与恢复

当 yt-dlp 以非零状态退出时，Queen 会保留真实的错误文本（不再是干巴巴的 `exit status 1`），对其分类，
必要时用对应手段自动重试一次，把链接记入待重试文件，并在默认情况下写入失败日志。

### 错误分类

| 分类 | 含义 | 常见解法 | 自动重试 |
| --- | --- | --- | --- |
| `auth` | 需要登录、年龄验证或私有视频 | 选择 Cookie 浏览器或提供 `cookies.txt` | 是：带浏览器 Cookie 重试 |
| `membership` | 会员/付费内容 | 使用有相应会员权限的已登录账号 | 是：带浏览器 Cookie 重试 |
| `drm` | DRM 加密流 | 无法下载 | 否 |
| `geo` | 地区限制 | 使用代理 | 否 |
| `cloudflare` | 反爬校验 / HTTP 403 | 浏览器指纹或 Cookie | 是：用 `--impersonate chrome` 重试 |
| `rate_limit` | HTTP 429 / 被限流 | 稍后重试，降低并发数或连接预算 | 否 |
| `unsupported` | 无提取器或无可用格式 | 升级 yt-dlp | 否 |
| `network` | 超时、连接重置、5xx、TLS 错误 | 检查网络/代理后重试 | 否 |
| `not_found` | 已删除或不可用 | 无需处理 | 否 |
| `storage` | 磁盘已满或权限不足 | 检查磁盘空间与下载目录权限 | 否 |
| `extractor` | 提取器 bug 或解析失败 | 升级 yt-dlp | 否 |
| `unknown` | 其他错误 | 查看失败日志 | 否 |

每种兜底手段最多自动重试一次：auth/membership 类错误在首次未带 Cookie 时带浏览器 Cookie 重试一次；
Cloudflare 类错误在未启用指纹时用 `--impersonate chrome` 重试一次。重试的任务会从解析阶段重新开始。

### 失败日志与诊断

- 每次失败都会写入 `~/.local/state/queen/logs/<时间戳>-<哈希>.log`（权限 `0600`，保留最近 200 个）。
  日志包含 Queen 与 yt-dlp 版本、站点、错误分类与详情、完整的 yt-dlp 参数以及全部输出。隐私模式下
  URL、域名与标题会被隐藏（错误详情与参数中出现的 URL 也会被掩码）。
- 在结果页按 `c` 会把诊断报告复制到剪贴板：版本信息、关键配置，以及每个失败任务的分类、详情与日志
  路径——可直接粘贴进 issue。按 `l` 打开日志目录。
- 可以用 `keep_failure_logs` 关闭失败日志。

### 失败链接

失败 URL 记录在 `<download_dir>/failed_links.txt`（每行一个、自动去重、权限 `0600`），并会从旧的中英
混排文件名 `视频失败链接-failed_links.txt` 自动迁移。主菜单 `3` 可重试这些链接，之后下载成功会自动移除
对应 URL。

### yt-dlp 新鲜度与升级

启动时 Queen 会执行 `yt-dlp --version`（只需几毫秒）并解析发布日期。若本机 yt-dlp 超过 45 天未更新，
会在主菜单、标题行（`⚠`）与启动 toast 中提示——升级 yt-dlp 通常是修复站点失效最快的办法。主菜单 `7`
会执行正确的升级命令：二进制位于 Homebrew 前缀时用 `brew upgrade yt-dlp`，否则用 `yt-dlp -U`。升级成功
后会重新检测版本。可用 `update_check` 关闭版本年龄检查。

### 隐私模式

隐私模式（`privacy_mode`，或站点档案的 `privacy` 标记）会在通知与失败日志中隐藏 URL、域名与标题；批量
通知仍会报告成功/失败数量。Rule34Video 默认处于隐私模式。失败链接仍会照常记录，以保证重试可用。

## 文件布局

| 文件 | 职责 |
| --- | --- |
| `config.go` | `Config` 结构、默认值、范围收敛，配置/站点/日志/重试/归档路径，原子保存、损坏备份、旧配置迁移与失败链接存储 |
| `sites.go` | `SiteProfile`、内置档案、`sites.json` 解析与合并、按 URL 计算有效策略 |
| `ytdlp.go` | yt-dlp 探测与版本年龄、brew 感知的升级命令、`BuildArgs`（全部参数）、进度模板与阶段映射 |
| `download.go` | 批量下载器：worker 池、连接预算、同站限流、进程组、自动重试、120 ms 事件合并、取消/中止 |
| `classify.go` | 12 类错误分类器，以及提示与重试策略 |
| `logs.go` | 失败日志写入/清理与诊断报告 |
| `theme.go` | 语义化颜色令牌、auto/dark/light 调色板、`NO_COLOR`、Markdown 样式 |
| `layout.go` | 响应式框架（最小 60×16、内容最大宽度 100）、裁剪、居中、滚动辅助 |
| `widgets.go` | 进度条、表格单元格、键位提示、状态行、spinner |
| `view_*.go` | 主菜单、输入、设置、站点档案、下载、结果、更新、更新日志、帮助与编辑视图 |
| `keys.go` | 各界面按键分发、帮助内容、鼠标滚轮处理 |
| `notify.go` | 通过 `osascript` 参数发送 macOS 通知 |
| `main.go` | Bubble Tea 模型/状态机，启动流程（依赖检查、提示、迁移、版本标签） |
| `i18n.go` | 中英文翻译表 |
| `markdown.go` | 用于应用内更新日志的零依赖 Markdown 渲染器 |
| `util.go` / `platform.go` | 格式化与 URL 辅助；PATH 增强、剪贴板、打开/定位、进程组终止、浏览器探测 |

## 构建与开发

| 命令 | 说明 |
| --- | --- |
| `make dev` | 运行 gofmt 检查、lint 与测试 |
| `make build` | 构建二进制；版本取自 `git describe --tags --always --dirty` |
| `make lint` | 运行 `go vet` 静态检查 |
| `make fmt-check` | 若有文件未按 gofmt 格式化则失败 |
| `make test` | 运行测试套件（`go test -race -cover ./...`） |
| `make release VERSION=1.6.0` | 在 `dist/` 下生成裁剪过的带版本号发布产物 |
| `make install` | 通过 `go install` 安装 |
| `make clean` | 清理构建产物 |

构建需要 Go 1.26 或更新版本。版本在构建时通过 `-ldflags "-X main.version=..."` 注入，源码中不重复
维护；源码内置当前发布版本 `v1.6.0`。CI 会运行 gofmt 检查、`go vet`、`go test -race -cover` 与构建；推送 tag 会
为 `darwin/arm64` 与 `darwin/amd64` 发布未签名的 macOS 二进制，并附带 SHA-256 校验和。

## 更新日志

参见 [CHANGELOG.md](CHANGELOG.md)（中文见 [CHANGELOG.zh-CN.md](CHANGELOG.zh-CN.md)），或在应用内
「更新日志」菜单中阅读。

## 贡献

欢迎提交 Issue 与 Pull Request。提交前请确保工作区通过 `make lint` 与 `make test`。

## 致谢

Queen 的整体视觉风格与终端界面设计深受开源项目 **Mole** 的启发，并大量借鉴其做法。在此向 Mole 作者
表示诚挚感谢。

## 许可证

以 [MIT](LICENSE) 许可证发布。

---

*由 kesonglab 维护。*
