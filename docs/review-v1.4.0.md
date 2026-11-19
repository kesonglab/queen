# Queen 项目评审（v1.4.0 基线）与 v1.6.0 交付说明

> 评审日期：2026-11-19 · 评审对象：`main` 分支（HEAD `618a09a`，约 v1.4.0）
>
> **状态（2026-11-19 更新）**：本报告列出的 P0/P1 问题已在 **v1.6.0** 中全部处理
> （失败原因保留真实错误、全局 toast、批量进度区分成功/失败、配置原子写与损坏备份、
> 结构化进度管线、连接预算与同站限流、站点档案、错误分类与自动重试、失败日志、
> yt-dlp 版本检查、TUI 主题/栅格/滚动/帮助、双语 README 与 CHANGELOG 等）。
> 详见仓库 CHANGELOG 的 v1.6.0 段落。
>
> **评审方法**：通读全部 2,479 行代码（4 个实现文件 + 2 个测试文件），并额外用**发布版二进制**
> 在受控的 80×24 伪终端中配合假 `yt-dlp` 实际跑了一遍主菜单 / 设置 / 输入 / 下载 / 汇总 /
> 更新日志 / 失败 7 个界面，下文标注「实测」的结论均来自真实渲染帧，不是推测。
> 另做了一轮**独立的第二遍代码审查**交叉验证（重点查并发/消息生命周期/解析器），
> 已合并其确认的结论。
>
> **环境限制**：本机未安装 Go 工具链（`/opt/homebrew` 里只有 brew 的 `go` 候选包，未安装），
> 因此 **`go vet` / `go test -race` 未执行**；代码级结论来自静态阅读，行为级结论来自实跑二进制。
> 若需要我把 `make dev` 跑通，需要先装 Go（约 1 分钟，或授权我 `brew install go`）。

---

## 0. 一句话结论

Queen 是一个**小而认真**的 yt-dlp 前端：进程模型、并发、进度平滑这些「难做对」的地方做对了，
代码量只有 ~2.5k 行却覆盖了下载器该有的闭环。但它目前处在「**能用的原型**」而非「**可靠的产品**」阶段：
失败原因显示、布局自适应、参数配置、站点兼容策略这四块都有明显的短板，其中 2 个是会直接影响
用户判断的 P0 级问题。

你提的三个目标（TUI 样式、下载性能、全网站点）中：

| 目标 | 结论 |
| --- | --- |
| TUI 样式优化 | **不需要换框架**，需要补「布局系统 + 主题系统 + 组件化」三件事，约 1–2 周 |
| 下载性能优化 | 瓶颈**不在 Go 代码**，一半在 yt-dlp 参数（当前配置里有反效果项），一半在进度/状态机设计 |
| 支持全网主流站点 | **yt-dlp 已经支持**（含 rule34video.com）；Queen 要做的是「别挡路 + 自动诊断降级 + 及时升级」，而不是自己写提取器 |

---

## 1. 评分速览（主观，10 分制）

| 维度 | 分数 | 一句话 |
| --- | --- | --- |
| 架构可读性 | 7 | 小而清晰，但 `main.go` 973 行三合一，正在变胖 |
| 正确性 / 可靠性 | 5 | 失败原因被 `exit status 1` 覆盖（P0）；批次间状态未隔离 |
| 下载性能 | 5 | 骨架正确，但 `--http-chunk-size` 等参数是负优化；进度管线浪费 |
| 站点兼容性 | 6 | 完全依赖 yt-dlp 的广度，但没有站点策略、没有降级、错误不可读 |
| TUI 样式 / 交互 | 6 | 配色统一、对齐花了心思；但无栅格、小终端裁切、信息密度低 |
| 工程化（CI/发布） | 7 | Makefile + CI + 双架构 release 齐全；缺签名公证、缺覆盖率门槛 |
| 文档 / i18n | 8 | 双语 README + 双语 Changelog 很少见，质量高 |

---

## 2. 优点（做得好的地方，建议保留）

1. **进程模型正确，这是同类工具最容易翻车的地方。**
   每个任务一个独立 `yt-dlp` 子进程，并且 `Setpgid: true` 建独立进程组，中止时 `kill(-pid)` 连
   `ffmpeg` 子进程一起清理（`download.go:326-338`、`download.go:287-302`）。很多下载器中止后留下
   ffmpeg 僵尸进程，这里处理到位。`TestAbortDoesNotFail` 也覆盖了「中止 ≠ 失败」的语义。

2. **并发模型简洁有效。**
   信号量 worker 池（`download.go:229-247`）+ 分片并发随任务并发下调（`config.go:304-311`），
   并用 `-race` 跑测试；`TestRetryFileConcurrency` 覆盖了失败链接文件的并发读写。

3. **进度解析的细节是有「产品感」的。**
   进度单调不回退、总大小锁存、速度指数平滑、`Destination` 变化时重置多流进度
   （`download.go:78-109`、`download.go:392-399`）——这些是只有自己天天用才会想到的补丁。
   定宽对齐（`padW` + `%5.1f%%`）确实消除了数字跳动，思路对（但见 §6.1 的宽度问题）。

4. **失败闭环完整：** 失败链接去重写入 → 菜单一键重试 → 成功后自动移除
   （`config.go:146-193`），`0600` 权限，没有为了「看起来完整」而写一堆假功能。

5. **工程化底子好：** Makefile（build/install/test/lint/release）、tag 触发的
   darwin/arm64+amd64 双架构 release + sha256（`.github/workflows/release.yml`）、
   版本用 `-ldflags` 注入、CI 跑 vet + `-race` + build。

6. **双语与内嵌 Changelog** 是很讨喜的细节：`//go:embed` 两个 Changelog 按语言切换
   （`main.go:18-22`、`main.go:463-468`），i18n 键在 zh/en 两侧目前**严格对齐（83/83，实测脚本比对）**。

7. **配置迁移**（`~/.config/videodl` → `~/.config/queen`，`config.go:91-112`）说明作者有版本迭代
   意识，这在个人项目里不常见。

8. **依赖自举：** 缺 yt-dlp/ffmpeg 时用 brew 一键安装（`main.go:913-943`），降低新用户门槛。

---

## 3. P0 — 会直接误导用户的缺陷（建议 v1.4.1 立刻修）

### 3.1 失败原因被 `exit status 1` 覆盖（实测确认）

`worker()` 在子进程结束后用 `exec` 的退出码错误**覆盖**了进度解析时抓到的真实报错：

- 解析侧本来抓到了真实原因：`handleLine` 里 `reError` 匹配 `ERROR: ...` 写入 `t.Error`
  （`download.go:501-506`）
- 但 `worker()` 随后执行 `t.Error = cleanErr(err.Error())`（`download.go:263-275`），
  而 `err` 是 `*exec.ExitError`，`.Error()` 恒为 `exit status 1`

实测画面（假 yt-dlp 打印 `ERROR: [youtube] abc123: Video unavailable...` 后退出 1）：

```
  ✗ example.com/bad-a
      exit status 1
  ✗ example.com/bad-b
      exit status 1
```

**影响**：失败原因、通知内容（`main.go:256-259`）、以及用户报障时的信息全部丢失。这是当前
「站点兼容性差」体感的最大单一来源——用户根本不知道是网络、需要 cookie、还是站点不支持。
**修法**：解析器维护 `lastErr`，worker 结束时优先使用解析到的错误文本；`exec.ExitError` 只作为兜底。

### 3.2 Toast 只在菜单页渲染，且无定时重绘（实测确认）

`toast` 只在 `menuView()` 里输出（`main.go:650-653`），但 `setToast()` 被下载页、设置页、
汇总页大量调用，于是分成两类：

- **看不到的**：剪贴板读取成功 / 重试成功（会立刻跳下载页）、设置页改下载目录成功或失败
  （停在设置页）、批量完成提示（`main.go:277-278`，跳到汇总页）——只有用户 4 秒内手动回到
  菜单才闪一下；其中「目录创建失败」是最该被看见的提示，现在完全无声。
- **能看到的**：打开目录、剪贴板失败这类留在菜单页的提示。

另外菜单页没有周期性 tick，所以 toast 过期后如果不按键，**旧 toast 会一直留在屏幕上**。
**修法**：toast 提到顶层布局（所有页面共用 footer 区域）+ 一个 1s ticker 触发重绘，
或改用独立的状态栏组件。

### 3.3 配置损坏时静默回到默认值，`save()` 吞掉所有错误

- `loadConfig()` 在 JSON 解析失败时直接 `return defaultConfig()`（`config.go:103-105`），
  既不备份也不提示，下次 `save()` 会把用户的配置**静默覆盖**。
- `save()` 里 `os.MkdirAll` / `os.WriteFile` 的错误全部 `_=` 丢弃（`config.go:114-121`），
  只读目录下用户会以为设置保存成功了。
- 没有原子写（临时文件 + rename），进程被 kill 时可能留下半截 JSON。

### 3.4 批量进度把「失败」算成「完成」

`batchProgressMsg.done` 在所有 worker 结束时 +1，不区分成功/失败（`download.go:283-284`）。
实测：3 个任务里 2 个失败、1 个还在 45%，批量进度条显示 **66% (2/3)**——用户看到的是
「快下完了」，实际是「已经失败两个了」。批量条应该分色（成功绿 / 失败红）或分别计数。

### 3.5 版本号三处不一致

- `main.go:25` 硬编码 `var version = "1.4.0"`（作为 ldflags 缺省值）
- `Makefile` 里 `VERSION ?= dev` → `make build` 构建出的是 **`vdev`**
- 发布流程注入的是 `1.4.0`（无 `v` 前缀），而 UI 又拼接 `"v" + version`

**修法**：源码默认改 `dev`，UI 只在 version 不为 `dev` 时加 `v`；`make build` 默认取
`git describe`。

### 3.6 失败链接文件名中英混杂

`retryFileName = "视频失败链接-failed_links.txt"`（`config.go:36`）。英文界面用户看到中英混排
文件名 + 中文路径提示，体验割裂；也影响 shell 里引用。建议 `failed_links.txt`（可配置）。

### 3.7 AppleScript 通知转义不完整（已实测）

`notify()` 把 `"` 替换成 `""`（`config.go:342-354`）。实测 AppleScript 字符串字面量：

```
$ osascript -e 'return "a""b"'   → syntax error (-2740)
$ osascript -e 'return "a\b"'    → syntax error (-2741)
```

即 `""` **不是**合法转义、反斜杠也没处理；失败信息里带引号或反斜杠时 `osascript` 编译失败、
通知静默丢失（`.Start()` 的错误被丢弃）。好消息是：由于先替换了引号，远程文本**无法**逃逸注入
（不存在注入风险，只是通知会失败）。**建议**：改用
`osascript -e 'on run argv' ... -- 文本` 参数传递；顺带每 4 秒 `Wait()` 一次回收子进程
（现在 `.Start()` 后从不 `Wait()`，每次通知留一个僵尸）。

---

## 4. P1 — 可靠性与正确性

### 4.1 stdout 与 stderr 合并进同一个管道

`cmd.Stdout = pw; cmd.Stderr = pw`（`download.go:336-339`）后共用 `bufio.Scanner` 按行解析。
yt-dlp 的进度写 stdout、警告/错误写 stderr，两个流并行写同一管道时可能在**行中间交错**，
造成进度行解析失败（表现为「进度偶尔卡住/跳变」）或错误文本被拼接。应开两个管道分别解析，
或给 stderr 加 `[stderr]` 前缀区分。

### 4.2 批次之间没有隔离（消息可能串台）

`waitForUpdates` 只返回 `batchDoneMsg{}`，**不携带批次 ID**（`download.go:522-530`）；
`m.tasks` 与 `d.tasks` 是两份互不引用的状态。场景：中止后立刻从菜单开新一批，旧批次残留的
`waitForUpdates` 很可能在新批次进入下载页后返回 `batchDoneMsg`，把新批次**提前切成汇总页**。
建议：每个 downloader 一个 `batchID`，所有消息带 ID，UI 丢弃非当前 ID 的消息；`m.tasks`
直接从 downloader 读取（单一事实来源）。

### 4.3 死代码 / 死字段（会误导后续开发）

| 位置 | 问题 |
| --- | --- |
| `Task.Total`（`download.go:39`） | 设置 3 处、**从未渲染**（UI 只有已下载，没有 `/ 总大小`） |
| `Task.Title`（`download.go:41`） | 从 `Destination` 解析、**从未使用** |
| `abortMsg`（`download.go:60-61`） | 声明 + 处理，**从未发送** |
| `batchProgressMsg.elapsed`（`download.go:52-55`） | 赋值后未使用（视图用 `time.Since`） |
| `update() (tea.Cmd, error)` | error 恒为 nil，签名多余（`main.go:240`） |
| `m.update` 的 `stSummary` 不处理 `ctrl+c` | 汇总页按 Ctrl+C 无反应 |

### 4.4 playlist 语义有坑

- 默认 `Playlist: true`（`config.go:44`）：粘一个带 `&list=` 的 YouTube 链接会**下载整个播放列表**，
  这是大多数用户没预期的行为。
- `Playlist=false` 时只加 `--no-playlist`，但仍是一 URL 一任务；`Playlist=true` 时**整个列表
  算 1 个任务**，`[info] Downloading item N of M` 只用来刷状态（`download.go:495-500`），
  批量进度永远是 0%→100% 的两段式，用户看不到「第 3/25 个」。
- 建议：两阶段队列（先 `--flat-playlist -J` 取条目列表 → 每项一个任务），播放列表落子目录
  （输出模板加 `%(playlist_title)s/%(playlist_index)03d - %(title)s`）。

### 4.5 文件名长度没有防护

输出模板 `%(title)s [%(id)s].%(ext)s`（`config.go:316`）没有 `--trim-filenames`。macOS 文件名上限
255 **字节**，中文标题 40 字 + 后缀就会超限，报 `File name too long`。建议
`--trim-filenames 200`，并 `--windows-filenames` 作为可选。

### 4.6 依赖检测在 Finder 启动时会误报

`hasCmd` 用 `exec.LookPath`（`config.go:269-272`）。从 Finder/`open` 启动的进程 PATH 只有
`/usr/bin:/bin:/usr/sbin:/sbin`，brew 的 `yt-dlp`、`ffmpeg` 全部找不到 → 误报「缺少依赖」。
建议启动时把 `/opt/homebrew/bin`、`/usr/local/bin` 追加到查找路径（或 `brew --prefix` 探测）。

### 4.7 没有 yt-dlp 版本新鲜度检查

`yt-dlp --version` 都没调用过。**站点失效 90% 靠升级 yt-dlp 解决**，而用户不会主动点「更新」。
建议：启动时 `yt-dlp --version`（毫秒级）解析日期，超过 30–45 天在菜单上显示黄色提示，
并提供「一键升级」（brew 装的就 `brew upgrade yt-dlp`，官方脚本装的才 `-U`，现在只能提示用户自己敲）。

### 4.8 进度解析：yt-dlp 的「估算大小」永远匹配不上（已对照 yt-dlp 源码确认）

`reSize`（`download.go:66`）要求 `~` 后面**紧接数字**：

```go
reSize = regexp.MustCompile(`of\s+~?([\d.]+)\s*([KMGiB]*B?)`)
```

而 yt-dlp（2026.10.26，`downloader/common.py:396`）在只有估算大小时使用的模板是
`of ~%(_total_bytes_estimate_str)s`，其中该字段由 `f'{format_bytes(...):>10s}'` **右对齐补空格**生成，
真实输出形如 `of ~   1.23MiB`（`~` 后有空格）。因此：

- 任何只有 `filesize_approx` 的流（YouTube DASH/HLS 极常见）都匹配失败；
- `totalLock` 永远为 false，于是走 `download.go:445-473` 的 fallback：`已下载/总大小` 只显示 `—`，
  `download.go:410-444` 的「按字节算平滑速度 + 自算 ETA」整段失效；
- 现有测试 `download_test.go` 的假数据只有精确大小 `of 1.23MiB`，掩盖了这个问题。

**修法**：`of\s+~?\s*([\d.]+)`（并补一条带 `~` 的测试用例）。这条改完后，
「大小/速度/ETA 显示不稳定」的体感问题会好一大半。

### 4.9 `reInfo` 正则永远不匹配（死分支）

`reInfo = \`\[info\]\s+Downloading item (\d+) of (\d+)\``（`download.go:70`），
但 yt-dlp 实际输出前缀是 **`[download] Downloading item N of M`**（`YoutubeDL.py:2163`）。
该分支唯一动作是重复设置 `StatusDownloading`，属于死代码；播放列表进度本来也没被真正利用（见 4.4）。

### 4.10 中止竞态：已完成的任务可能被标成「已中止」，摘要统计偏低

- `runYtdlp` 结束时先判 `d.ctx.Err() != nil` 再返回（`download.go:372-374`）：
  若子进程恰好成功退出、而用户同一瞬间按了中止，任务会被判成 `context.Canceled` → 标 Aborted，
  且不会 `removeFromRetry`。
- `q` 中止时 UI 立刻把 Pending/Downloading 标 Aborted（`main.go:446-457`），但 channel 里可能还有
  已投递的 `taskUpdateMsg`；汇总页仍会处理它（`main.go:250-259` 不判断状态），把 Aborted 覆盖回
  Downloading，而 `countResults`（`main.go:829-843`）不统计这两种状态 → 摘要数字对不上。
  根因仍是 `m.tasks` 与 `d.tasks` 双份状态（§4.2）。

### 4.11 显式空 `download_dir` 会原样传给 yt-dlp

`clampConfig` 不校验 `DownloadDir`（`config.go:67-89`）。若用户手工把 JSON 写成
`"download_dir": ""`，`ytdlpArgs` 会生成 `-P ""`（`config.go:317`）；`retryFile` 有空值兜底但
参数侧没有。加一行空值回退到 `~/Downloads` 即可。

### 4.12 没有失败日志，用户无法报障

子进程 stderr 只用于正则解析，**没有落盘**。用户遇到「某站点失败」时既看不到原因（见 3.1），
也没有日志可提交。建议：失败时把该任务的完整输出写到
`~/.local/state/queen/logs/<任务>.log`，汇总页提供「复制诊断信息」快捷键。
这是「支持全网站点」最关键的运营基础设施——没有它，站点支持只能靠你一个人猜。

---

## 5. 下载性能专项

先给结论：**Go 侧几乎没有性能问题**（4 进程 × 1 解析 goroutine，CPU 占比很低），
真正拖慢速度的是**传给 yt-dlp 的参数**和**进度管线的设计**。

### 5.1 现有参数逐条体检

| 参数 | 现状 | 评价 / 建议 |
| --- | --- | --- |
| `--http-chunk-size 10M`（`config.go:319`） | 全局开启 | ❗**反优化**。yt-dlp 帮助明确写这是 *experimental*、**默认关闭**；强制分块会：①每个 10MB 重新发 range 请求，高延迟线路更慢；②部分站点不支持 Range → `416`；③破坏一次性签名 URL 的下载。**建议删掉**，仅在特定被限速站点做白名单。 |
| `--buffer-size 16K`（`config.go:321`） | 16K | 基本无效：yt-dlp 默认 `--resize-buffer` 会从初始值自动扩张。真要提高吞吐应留默认或 1M，**不是重点**。 |
| `--concurrent-fragments 16`（`config.go:304-311`） | 4 任务×16 | 最多 64 条并发连接，家用带宽/磁盘容易过载；且并非所有站点都吃分片并发。建议**全局连接预算**（例如 24）+ 按站点分配（YouTube/HLS 16，其他 4–8），并对同域名任务做串行化（见 5.3）。 |
| 缺 `--throttled-rate 100K` | — | ❗**最该加的单项**。很多站点（尤其 YouTube 系）前几 MB 满速、随后限速到几十 KB/s，这个参数会触发自动重新提取，实测能把吞吐拉回满速。 |
| 缺 `--retry-sleep` | — | 429/503 立即重试只会继续被拒；建议 `--retry-sleep 3`（或指数退避表达式）。 |
| `--merge-output-format mp4` + `bestvideo*+bestaudio`（`config.go:314-315`） | 强制 mp4 | 可能选到 VP9/AV1 + Opus，remux 进 mp4 后 **QuickTime 播不了音频**，或不兼容时静默变成 mkv。建议加格式排序：兼容优先 `-S "vcodec:h264,acodec:aac,ext:mp4:m4a"`，或格式选择器分「画质优先 / 兼容优先」两档。 |
| `--embed-metadata --embed-thumbnail`（`config.go:331-333`） | **默认开** | 每个文件多一次网络请求 + 一次 ffmpeg pass，且部分站点缩略图失败会留下警告。建议默认**关**（或仅在「单文件、非音频」时智能开启）。 |
| `--extract-audio --audio-format mp3`（`config.go:328-330`） | 音频模式必转 mp3 | 强制有损二次转码，CPU 占用高、耗时。建议默认 `m4a`/`opus` **直接 copy**，mp3 作为选项；兼容性由用户在设置里选。 |
| `--no-overwrites` | 开 | 合理；但配合无 `--download-archive`，播放列表重跑会重新提取。建议加 archive。 |
| 缺 `--ignore-config` | — | 用户全局 `~/.config/yt-dlp/config` 可能覆盖 `-o`/`-f`，导致 Queen 行为不可预测。建议显式隔离。 |

### 5.2 进度管线：从「正则」升级到「结构化」

目前 `--newline --progress` + 6 条正则（`download.go:63-75`）解析人类可读文本，已经堆了
「Unknown B/s」「HH:MM:SS」「估算大小」等多层容错补丁，仍然会在以下场景出错：
进度行与警告交错、`[download] Destination` 与进度同帧、播放列表 item 切换、聚合器输出格式变化。
建议改用 yt-dlp 的**结构化输出**：

```
--progress-template "download:QDL|%(progress.downloaded_bytes)s|%(progress.total_bytes)s|%(progress.speed)s|%(progress.eta)s|%(progress.status)s"
```

用 `strings.Cut` 解析固定分隔符，**不再依赖人类可读格式**；错误走独立 stderr 管道。
这样还能顺手拿到阶段信息（`postprocessor:` / `download:`），解决「100% 后卡在合并阶段」的体感问题
（现在是靠 spinner 硬撑）。

### 5.3 并发模型：从「进程数」升级到「连接预算」

当前并发 = 任务数 ≤16，每个任务再 ×分片数，没有全局/按域上限。建议：

1. **全局连接预算**（如 `budget = 24`），每个任务启动前按分片数申请额度；
2. **同域名串行化**（可开关）：避免 4 个任务同时打同一站点触发 429/Cloudflare 挑战；
3. **节流进度消息**：现在每行进度都 `updateTask` → 加锁 → 拷贝 Task → 发 channel → UI 全量重绘。
   16 并发时是「每行一条消息 × 每帧全量 View()」，属于无谓浪费。建议 downloader 内按
   **100ms 合并**成一个批次消息，UI 侧 `View()` 自然降频。

### 5.4 感知性能（用户觉得快）

- 输入链接后到第一次进度出现，中间是 `yt-dlp` 的提取阶段（3–15 秒，YouTube 更长），当前界面什么都不显示。
  应利用现有输出识别阶段：`[youtube] Extracting URL` → 显示「解析中」；`[download] Destination`
  → 显示文件名；`[Merger]/[ExtractAudio]` → 显示「合并/转码中」。
- 批量开始前用 `--flat-playlist -J --skip-download` **并发预取**元数据：能提前知道条数、
  总大小、预估时长，进度条不再是从 0 开始的黑盒。
- 汇总页给「打开文件夹 / 重试失败 / 复制失败清单」三个动作，减少用户找文件的时间。

---

## 6. 站点兼容性专项（含 rule34video.com）

### 6.1 先讲事实：yt-dlp 的能力边界

本机实测 `yt-dlp 2026.10.26`（brew 安装，Python 实现，自带 curl_cffi）：

- `--list-extractors` 输出 **1755 行**（约 1700+ 个提取器），覆盖 YouTube / Bilibili / X /
  TikTok / Instagram / Douyin / Vimeo / Twitch / SoundCloud 等主流站点；
- **`Rule34Video` 提取器原生存在**（同类还有 PornHub、XHamster、SpankBang 等）——
  也就是说 **rule34video.com 不需要 Queen 写任何代码就能下**；
- `--list-impersonate-targets` 可用：Chrome-133 / Chrome-136 / Safari-18.4 / Chrome-Android 等
  （基于 curl_cffi），可用于过 Cloudflare 类防护。

**所以「支持全网所有主流站点」的正确姿势不是自己写站点适配，而是：**

```
Queen 的价值 = yt-dlp 的广度 × (不挡路 + 自动诊断降级 + 及时升级)
```

### 6.2 Queen 现在会「挡路」的地方

| 问题 | 后果 | 建议 |
| --- | --- | --- |
| cookie 自动探测**只对 X/Twitter 生效**（`config.go:281-291`、`config.go:255-267`） | 其他需要登录/年龄验证的站点默认匿名，失败后用户不知道该怎么办 | 域名 → 策略表；并支持「失败后自动带 cookie 重试一次」 |
| 没有 `--cookies`（cookies.txt）支持 | 部分站点（尤其 Safari 取不到 cookie 时、或多账号场景）无解 | 增加 cookies.txt 导入 |
| 没有 `--impersonate` | Cloudflare 保护的站点直接 403 | 按站点档案开启（注意 yt-dlp 官方警告：全局 impersonate 会**降低速度**，必须按需开） |
| 没有 `--extractor-args` 透传 | YouTube 新版 PO token / 部分站点的 client 选择无法调整 | 设置里加「高级：extractor args」文本框 |
| 没有 `--force-generic-extractor` 兜底 | 提取器失效但页面是标准播放器的站点本可救回 | 特定错误类型下自动二次尝试 |
| 没有 `--user-agent` / `--referer` / `--add-header` | 部分需要 Referer 的 CDN 会 403 | 站点档案字段 |
| 没有 `--proxy` UI | 区域限制站点无解 | 配置项 + 环境变量说明 |
| 错误文本被覆盖成 `exit status 1`（§3.1） | **用户完全无法定位原因**，兼容性问题全部沉底 | 见 3.1 |
| `--http-chunk-size 10M` 全局 | 破坏签名 URL/Range 站点（§5.1） | 删除 |
| 合并容器强 mp4 + 缩略图默认开 | 部分站点合并/嵌缩略图失败 | 站点档案里可关 |

### 6.3 rule34video.com 专项

- **状态**：yt-dlp 原生支持，Queen 理论可直接下；需要实际 URL 做一次 `--simulate` 验证
  （我没有真实链接，也不建议在仓库里放成人站点 URL）。
- **预期需要处理的点**：
  1. 年龄门/登录 → cookie（现在只能全局指定浏览器，不能按站点）
  2. 可能 Cloudflare → `--impersonate chrome`
  3. 缩略图/元数据嵌入 → 部分文件失败，建议该站点档案 `embed=off`
  4. **隐私**：通知会把 URL 和错误推到锁屏/通知中心（`main.go:256-259`），这类站点场景下
     很尴尬。建议加「隐私模式」：通知只显示「1 个任务失败」，不显示 URL/标题。
- **建议做成「站点档案（site profile）」**：一个内置 TOML/JSON 表 + 用户可覆盖的
  `~/.config/queen/sites.json`：

```json
{
  "rule34video.com": { "cookies": "auto", "impersonate": "chrome", "embed": false, "privacy": true },
  "bilibili.com":    { "cookies": "auto", "extractor_args": "bilibili:..." },
  "youtube.com":     { "cookies": "auto", "extractor_args": "youtube:player_client=default", "fragments": 16 }
}
```

### 6.4 必须承认的边界：DRM

「全网所有主流**流媒体**网站」这句话里有一类做不到：**Netflix / Disney+ / Prime Video /
Apple TV+ / HBO Max / Spotify** 等是 Widevine/FairPlay DRM，yt-dlp **明确不支持**（技术 + 法律）。
另外部分站点只是图片画廊（gallery-dl 的领域）。
建议在 README/设置页里**明确写清**「支持 yt-dlp 支持的全部公开视频站点；不支持 DRM 流媒体」，
避免用户预期管理出问题——这比含糊承诺「全网」更专业。

---

## 7. TUI 样式与交互专项

### 7.1 实测画面（80×24，发布版二进制）

主菜单：

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  👑 Queen v1.4.0  kesonglab💗 · 由 yt-dlp 强力驱动 · 采用 Mole 风格
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

▶ [1] 粘贴链接下载  每行一个链接，Ctrl+D 开始
  [2] 从剪贴板读取  读取剪贴板中的链接
  ...
  [q] 退出  再见

↑↓ 选择 · Enter 确认 · q 退出
```

设置页（**注意冒号完全不对齐、最后一行悬空冒号**）：

```
▶ 播放列表模式 : 开
  音频模式 (mp3) : 关
  下载字幕 : 关
  ...
  Cookie 浏览器 : 自动检测
  每链接重试次数 : 2 次
  ...
  保存并返回 :
```

下载页（**每任务状态行实际 84 列，在 80 列终端被裁掉尾部 ETA**）：

```
  ⣾  example.com/video-a
      ██████████████░░░░░░░░░░  60.0% 74.1MiB     | 速度 36.2MiB/s   | 剩余 0:00
```

更新日志页（**直接渲染原始 Markdown**）：

```
# 更新日志
## v1.4.0 (2026-10-05)
### 新增
- 设置页可切换 yt-dlp 格式选择器与合并容器（`mp4` / `mkv` / `webm`）。
```

### 7.2 测量结果（脚本按东亚宽度计算显示列）

| 元素 | 实测 | 问题 |
| --- | --- | --- |
| 任务状态行 | **84 列**（终端 80 列） | 尾部 ETA 被静默截断（实测显示 `剩余 0:00` 而非 `0:00:00`）；60/70 列终端丢得更多 |
| 设置页冒号列 | 12–19 列之间（12 行落在 6 个不同列） | 没有标签列宽栅格，冒号参差 |
| 菜单描述列 | 13–21 列之间（8 行 8 个不同起点） | 描述起点随标签长度漂移 |
| 输入框 | 80 列（恰好占满） | 无溢出问题；但固定 8 行，占掉 1/3 屏幕 |
| 分隔线 | `maxInt(width, 50)` | 终端 <50 列时溢出换行 |
| 更新日志可见行 | 计算 `height-6`，实际总行数 29 > 24 | 顶部 banner 被顶出屏幕（实测帧丢失 👑 行） |

### 7.3 视觉设计问题清单

1. **没有布局栅格**：菜单/设置都用「行字符串拼接」，标签列宽不固定，所以冒号、描述、数值列
   全部参差。这是「看起来不精致」的最主要来源，改造成本很低（一个固定 label 宽度的 table 组件）。
2. **固定宽度字段不响应终端**：24 格进度条 + 11+11+8 固定字段，在 <84 列时必然丢信息。
   应做成断点：`len>=100` 全字段、`80–99` 收窄 bar、`<80` 两行式（第一行 URL+百分比，第二行速度/ETA）。
3. **颜色是硬编码 hex**（`main.go:82-99`）：`#00ff87` 在浅色终端上对比度极差；没有
   `AdaptiveColor`（lipgloss 自带 `HasDarkBackground`）、没有主题切换、没有 `NO_COLOR` 语义化。
4. **原始 Markdown**：Changelog 页把 `#`/`###`/反引号直接显示；用 glamour 或 20 行轻量渲染即可。
5. **信息密度低**：banner 占 3 行且每页重复；设置页 12 行无分组无说明；汇总页只有两个数字，
   大片空白。
6. **多任务共用同一个 spinner 实例**（`main.go:753`）：4 个任务同时转，帧完全同步，像装饰不像状态。
7. **没有滚动**：任务多时只能截断成「还有 N 个任务」（`main.go:705-720`），无法回看。
8. **没有 help 覆盖层 / 没有鼠标**；每页 footer 是一句长文案，窄终端会被截断。
9. **无状态栏**：看不到当前并发数、排队数、yt-dlp 版本等关键信息。
10. **汇总页没有明细**：只有「成功 3 失败 0」（实测），失败的是哪几个只能去翻 txt 文件。

### 7.4 视觉改造建议（按投入产出排序）

| 优先级 | 改造 | 产出 |
| --- | --- | --- |
| P0 | **主题 token 化**：语义色 `accent/success/warn/danger/dim/border/surface` + `AdaptiveColor`（深/浅自动） + `--no-color` 支持 | 一处改动，全站统一；解决浅色终端 |
| P0 | **栅格布局组件**：`layout{header, content, footer}` + `table`（label/desc/value 三列定宽）+ 响应式断点 | 解决冒号不对齐、84 列裁切 |
| P1 | **滚动任务列表**：`bubbles/viewport`，支持 j/k/PageUp/PageDown，按状态过滤（全部/下载中/失败） | 多任务可用性 |
| P1 | **状态栏**：并发 x/y · 排队 n · yt-dlp 版本/新鲜度 · 快捷键提示 | 信息密度、可发现性 |
| P1 | **汇总页升级**：结果表格（图标/站点/文件名/大小/耗时/原因）+ `r` 重试失败、`o` 打开目录、`c` 复制诊断 | 闭环动作 |
| P2 | **help 覆盖层**（`?`）、**每任务独立 spinner/阶段标签**（解析中/下载中/合并中） | 专业感 |
| P2 | **Changelog Markdown 渲染**、**输入框随内容自适应高度**、**宽屏限宽居中（max 100 列）** | 精致感 |
| P2 | **View 快照测试**（80×24 / 120×40 golden） | 防样式回归 |

---

## 8. 架构与工程化

### 8.1 代码结构建议

`main.go` 973 行 = 状态机 + 7 个视图 + i18n 胶水 + 依赖检测 + 主函数，已经到临界点。
下一版加「队列/历史/站点档案/主题」会瞬间破 2k 行。建议：

```
internal/
  config/     Config、迁移、原子写、版本号
  ytdlp/      argv 构建（纯函数、可 golden 测试）、进度解析（独立、可 fuzz）
  download/   worker 池、连接预算、批次 ID、事件流
  tui/        model、state、views、theme、components(table/statusbar/list)
  i18n/       翻译表 + parity 测试
```

同时统一「单一事实来源」：UI 不再自己维护 `m.tasks`，只持有 downloader 的快照。

### 8.2 测试（当前 14 个测试，方向对但覆盖偏窄）

已有：X URL 判定、cookie 参数、retry 参数、clamp、retry 文件并发、URL 解析、cycleChoice、
设置行、下载管线、中止语义、多流重置、UTF-8 截断、bar 边界。
缺口（按价值排序）：

1. **`ytdlpArgs` 全量 golden 测试**（参数是性能/兼容的核心，现在只测了 3 个 flag）
2. **进度解析 golden 测试**：把真实 yt-dlp 输出（含警告交错、播放列表、live、无总大小、
   **`of ~` 估算大小**）做成 testdata，替换正则/模板时才有回归保护。
   现有假数据只覆盖了「精确大小」一种格式，恰好掩盖了 4.9 的缺陷。
3. **`View()` 快照测试**：主题/布局改动必须有回归保护
4. **i18n parity 测试**：现在 83/83 对齐，但没有测试锁住（且菜单项用位置式字面量，非常易错）
5. **changelog 双语一致性检查**（脚本对比版本段落）

CI 建议：加 `golangci-lint`、覆盖率门槛（例如 ≥60%）、`go test -race` 已在（保留）；
release 建议用 goreleaser 统一产物命名 + 自动 changelog。

### 8.3 发布 / 跨平台

- **未签名未公证**：README 已经要教用户「右键打开 / 隐私与安全性里允许」，这是 macOS 用户流失的大头。
  v2 前建议 `codesign` + `notarytool` + Homebrew tap（`brew install kesonglab/tap/queen`）。
- **平台绑定**：`syscall.Setpgid`、`pbpaste`、`open`、`osascript`、brew 路径全部是 darwin 专用。
  若要支持 Linux/Windows（这类工具在 Linux 上需求更大），需要 build tags 抽象
  `platform/{spawn,kill,clipboard,notify,open,installDeps}.go`。
- README 说 `make build`，但默认产出 `vdev`（§3.5）；`make install` 的 `go install ./...` 与
  `-ldflags` 组合也没验证。

### 8.4 安全 / 隐私

- `--cookies-from-browser` 会读取浏览器 cookie 库（macOS Chrome 还会弹 Keychain 授权）。
  建议首次使用时明确提示用途，并在设置页显示「仅对哪些站点使用」；`--cookies` 文件支持
  可以让高级用户避免全库读取。
- 通知内容含 URL + 错误文本（可能含标题）→ 锁屏泄露；建议隐私模式（§6.3）。
- `notify` 的 AppleScript 转义问题（§3.7）。
- 下载目录 `MkdirAll` + `open` 属于预期行为，可接受；可选加「下载完成校验文件大小」。

### 8.5 文档一致性小问题

- README 键位表写「`q` / `Esc` 返回 / 退出」，实际汇总页 `q` 是返回菜单、`Ctrl+C` 无效；
- README 的 `Go 1.26` 徽章、`go.mod` 的 `go 1.26.6`、CI 的 `go-version-file` 三者要一致；
- 失败链接文件名中英混杂（§3.6）；
- Changelog 双语需人工同步，建议脚本校验（§8.2）。

---

## 9. 下一版本方向建议（三档可选，推荐 A+B+C 组合）

### A. v1.4.1 热修（0.5–1 天，建议先发）

| 项 | 验收标准 |
| --- | --- |
| 失败原因保留真实错误文本 | 失败行/通知/汇总显示 yt-dlp 原始 `ERROR:` 文本 |
| Toast 全局渲染 + ticker 过期 | 任何页面都能看到提示，4s 后自动消失 |
| 批量进度区分成功/失败 | 失败单独计数/着色，不再把失败算作完成 |
| 配置原子的写 + 错误提示 + 损坏备份 | 只读目录/损坏 JSON 有明确提示 |
| 版本号统一 | `make build` 不再显示 `vdev`；`v1.4.0` 只来自 tag |
| 失败文件名改英文 | `failed_links.txt` |
| 通知参数化转义 | 含引号/反斜杠的文本能正常通知 |
| 修正 `of ~` 估算大小解析 + 删死正则 | YouTube 等只有估算大小的流能显示大小/速度/ETA 与平滑值 |
| 删 `--http-chunk-size` | 默认参数不再破坏 Range 站点 |

### B. v1.5.0 性能 + 兼容（1–2 周，核心版本）

1. yt-dlp 参数重做（§5.1 表）：去 http-chunk-size、加 `--throttled-rate`/`--retry-sleep`/
   `--ignore-config`/`--trim-filenames`、格式选择「画质/兼容」两档、音频 copy 优先、embed 默认关
2. 结构化进度管线（`--progress-template`）+ 独立 stderr + 阶段标签（解析/下载/合并）
3. 连接预算 + 同域串行化 + 进度合并节流（100ms）
4. 站点档案表（内置 + 用户覆盖）：cookie / impersonate / extractor-args / embed / 隐私
5. 错误分类器：auth / DRM / geo / Cloudflare / unsupported → 给出「怎么做」+ 自动重试策略
6. 失败日志落盘 + 汇总页「复制诊断」
7. yt-dlp 新鲜度检查 + 一键升级（brew 感知）
8. playlist 两阶段队列（预取条目、每项一任务、子目录）
9. `--download-archive` 可选项

**验收**：同一批 20 个混合站点链接（含 YouTube/X/Bilibili/一个 Cloudflare 站点/一个需登录站点），
成功率与 yt-dlp 命令行一致；被限速的 YouTube 视频吞吐提升；任意失败都能看到可操作原因。

### C. v1.6.0 TUI 重做（1–2 周）

主题 token 化 → 栅格布局 + 响应式断点 → 滚动列表 + 状态栏 → 汇总页动作 → help 覆盖层 →
Markdown changelog → View golden 测试（§7.4 全表）。

**验收**：80×24 / 100×30 / 140×40 三种尺寸下无裁切、无错位、无溢出；浅色终端可读；
`NO_COLOR=1` 仍可用；所有页面都有明确快捷键提示。

### D. v2.0.0（可选，按需）

队列持久化（关机恢复）、下载历史、每任务暂停/取消、多平台（Linux 优先）、
签名公证 + Homebrew tap、goreleaser。

### E. 不建议做的

- ❌ 自己写站点 extractor（1700+ 站点的维护是无底洞）——要做的是**跟随 yt-dlp 版本 + 用户反馈回归**；
- ❌ 承诺 DRM 流媒体；
- ❌ 默认集成 aria2c（cookie/分片/校验兼容性会引入新问题，做「高级选项」即可）；
- ❌ 自己实现 yt-dlp 自更新（brew 管理的必须走 brew）；
- ❌ 为了「少依赖」不用 `bubbles/viewport`、`progress`、`help`——这些能省几百行自研代码。

---

## 10. 附录 A：参数前后对照（可直接落地）

| 现在 | 建议 |
| --- | --- |
| `--http-chunk-size 10M` | 删除（或站点白名单开启） |
| `--buffer-size 16K` | 删除（默认已 `--resize-buffer`） |
| `--concurrent-fragments 16/8/4` | 全局连接预算 + 按站点（YouTube 16 / 默认 4–8） |
| — | `--throttled-rate 100K` |
| — | `--retry-sleep 3` |
| — | `--ignore-config` |
| — | `--trim-filenames 200` |
| — | `--color never`（保证解析稳定） |
| `--merge-output-format mp4` + `bestvideo*+bestaudio` | 加 `-S` 兼容排序，或分「画质/兼容」两档 |
| `--embed-metadata --embed-thumbnail`（默认开） | 默认关，或「仅单文件/视频」智能开 |
| `--extract-audio --audio-format mp3` | 默认 `m4a`/`opus` copy，mp3 可选 |
| `--progress` + 正则 | `--progress-template` 结构化字段 |
| 无 | `--download-archive`（可选）、`--live-from-start`（直播可选） |
| 无 | `--cookies FILE`、`--impersonate`（站点档案） |

## 附录 B：本次评审的复现方式

```bash
# 1) 发布版二进制
curl -fsSL -o queen.zip https://github.com/kesonglab/queen/releases/latest/download/queen-darwin-arm64.zip
# 2) 在 80×24 伪终端里跑，PATH 前置一个只打印固定进度行的假 yt-dlp
# 3) 用 pyte 解析 ANSI 得到真实屏幕，测量显示列宽 / 截断情况
```

实测发现的问题均可在 min 尺寸终端上直接复现：任务状态行 84 列被裁、设置页冒号不对齐、
Changelog 原始 Markdown、失败原因显示 `exit status 1`、汇总页无明细。

---

*本文档由代码评审 + 二进制实跑生成，可作为 v1.5/v1.6 的输入；如不需要可随时删除。*
