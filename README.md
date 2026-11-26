# PureAgent

本地运行的图像创作工作台。Go 后端与内嵌 Web UI，无第三方运行依赖。
支持 Seedream 5.5 / AgentPlan 与 OpenAI Images（GPT Image 2.6）。可保存多个模型配置，分别选择接口类型、模型名称、API Key 和 URL。

## 启动

Windows：在 [GitHub Release](https://github.com/UMR731x/PureAgent/releases/latest) 下载 `PureAgent-windows-amd64.zip`，解压后双击 `start.cmd`，然后打开 **http://127.0.0.1:57123**。首次启动需保留终端窗口。从源码克隆的项目需先按下方命令构建。

Linux：下载 Release 中的 `PureAgent-linux-amd64.tar.gz`，解压后在该目录运行 `./bin/pureagent`。

从源码运行（Go 1.24 或更新版本）：

```sh
go run -buildvcs=false .
```

构建并运行：

```sh
go build -buildvcs=false -o bin/pureagent .
./bin/pureagent
```

Windows 构建：

```powershell
go build -buildvcs=false -o bin/PureAgent.exe .
.\start.cmd
```

监听地址固定为 `127.0.0.1`。端口与数据目录可以在启动时指定：

```sh
./bin/pureagent -port 57125 -data ./data
```

默认数据目录是**当前工作目录**下的 `data/`；`start.cmd` 会先进入项目根目录。`-buildvcs=false` 使从不含 Git 元数据的源码副本构建时也能工作。

## GitHub Actions 构建

推送、Pull Request 和手动运行均会触发 [Build PureAgent](https://github.com/UMR731x/PureAgent/actions/workflows/build.yml)。工作流使用 Go 1.27，在 Windows/Linux 上测试真实进程替换、回退及数据保留，并运行 Linux 竞态测试、Go 静态检查和前端 JavaScript 语法检查，然后生成 Windows/Linux amd64 可执行文件。

在一次成功运行的 **Artifacts** 区域下载：

- `PureAgent-windows-amd64`：解压后包含 `start.cmd`、`README.md` 和 `bin/PureAgent.exe`。
- `PureAgent-linux-amd64`：下载后解压其中的 `PureAgent-linux-amd64.tar.gz`，在解压目录运行 `./bin/pureagent`。tar 包保留可执行权限。

需要手动构建时，在 Actions 中选择该工作流并点击 **Run workflow**。CI 使用模拟图片接口测试，不需要模型 API Key。构建包不包含本地配置或会话数据；首次使用请在模型设置中填入自己的密钥。

### 自动发布 Release

在 `.github/releases/` 新增版本文案，例如 `v0.1.1.md`，经人工确认后推送到 `main` 即可。流水线从文件名读取版本号，测试通过后自动构建、创建版本标签并发布 Release；正文完全使用该 Markdown 文件，不生成或改写文案。无需人工上传构建包。

修改最新版本的文案后推送到 `main`，流水线仅更新已发布版本的正文，保留原有下载文件。代码变更需要发布新版本时，应新增更高版本号的文案文件。没有文案变更的普通推送只产生 Actions 开发构建；在 `main` 手动运行工作流也可发布最新文案对应版本。

### 客户端自动更新

左下角“检查并更新”会读取本仓库最新正式 Release，下载匹配当前系统的客户端并校验下载完整性。已有生成任务会先完成，随后自动替换客户端、重启并恢复页面；新版本正常启动后删除旧客户端和临时更新文件，启动失败则恢复原版本并显示错误。

更新只替换运行中的可执行文件，保留原工作目录、端口和 `-data` 参数；配置、会话 JSON、上传原图和生成结果不会被迁移或删除。自动重启后的服务在后台运行，浏览器中的当前任务和未提交输入会保留。请勿在更新过程中移动安装目录。

没有更新入口的旧客户端需要先手动替换一次 `bin` 中对应文件，保留 `data`。源码开发构建显示 `dev`，不会自行替换源码运行产生的临时程序；一键更新适用于 Release 客户端。

## 使用

1. 右上角“模型设置”中选择接口类型并保存配置。旧配置仍按 Seedream 处理。同一接口类型的已有密钥留空时保持原值；切换接口类型需填写对应密钥，界面不会回传已保存的密钥。
2. 输入描述，选择模型与生成参数。点击“生成”，或按 `Ctrl/⌘ + Enter`。
3. 参考图可通过附件按钮、拖入或粘贴上传；输入框上方的顺序对应“图 1、图 2”等编号。
4. 点击生成结果查看原图；“编辑”把选中图片作为参考图，再输入本轮修改要求。每次编辑都保存新结果。
5. “局部编辑”支持点选与框选。Seedream 将 `<point>` / `<bbox>` 坐标插入输入框；OpenAI 为第一张参考图创建独立的透明 PNG 标记，并填入可修改的编辑指令。原图始终保留原始字节。
6. “更多生成选项”按接口显示能力：Seedream 提供背景、PNG/JPEG、水印与图层拆分；OpenAI 提供背景、PNG/JPEG/WebP、质量与复杂度。Seedream 拆分只使用一张 PNG/JPEG，可填写拆分要求，也可留空让模型识别。
7. 拆分结果逐层保存。打开文字图层可查看、复制 OCR 文本，以及位置和文字样式；各图层也可继续交给模型编辑。

任务运行在 Go 后端，刷新页面、关闭页面或切换任务不会取消生成。点击“停止”会取消本地等待与后续调用，上游可能仍在处理或已经计费。停止程序后，未完成记录会在下次启动标记为中断，不自动重新调用。

## Seedream 5.5 参数

| 选项 | 行为 |
| --- | --- |
| 生成数量 | 默认 1。5.5 每次请求生成一张；多张任务按指定数量逐次调用，不发送 `n`、组图或流式参数。发生错误时停止后续调用，保留已完成结果。 |
| 自动分辨率 | 省略 `size`。生成场景服务端默认 2K；拆分场景由接口按输入图自动适配。 |
| 分辨率档位 | 1K、1.5K、2K。 |
| 自动比例 | 不人为固定宽高比，模型结合描述和参考图决定。 |
| 指定比例 | 换算成对应档位的明确宽高；分辨率为自动时使用接口默认的 2K 像素预算。 |
| 参考图 | 最多 10 张，每张最多 30 MiB，原始文件发送为 Data URL。支持 PNG、JPEG、WebP、BMP、TIFF、GIF、HEIC、HEIF；部分格式的浏览器预览取决于浏览器支持。 |
| 背景 | 自动、不透明、透明。透明背景使用 PNG。拆分请求不发送 `background`。 |
| 图层拆分 | 最大输出 2–100 张，含底图；默认采用 API 的 30 张。最大数影响接口预扣额度，实际输出数由模型决定。 |
| OCR | 拆分时可启用。只对模型识别为规整文字的图层返回，字体与样式是模型推断结果。 |

只提交当前输入框内的提示词、选定的参考图与生成选项。不设置系统提示词，不偷偷拼接历史对话。继续修改时选择需要编辑的结果；若需要结合多张图，可以将它们依次加入参考图。

Seedream 的自动比例和分辨率使用 API 的真实语义，不发送该生成接口不支持的 `size: "auto"`。本地页面显示的是任务进度，不是模型的流式生成。

## OpenAI GPT Image 2.6 参数

在模型设置中选择 **OpenAI Images · GPT Image 2.6**，填入 `gpt-image-2.6-flare` 或 `gpt-image-2.6-sunburst`、对应 API Key 和 `https://api.openai.com/v1`。也可填写兼容服务的模型别名、基础 URL 或完整 `/images/generations`、`/images/edits` 地址。自定义路径前缀和查询参数会保留；服务需实现对应的 Images 协议。

| 选项 | 行为 |
| --- | --- |
| 生成与编辑 | 纯文本使用 JSON `/images/generations`；有参考图时使用 multipart `/images/edits`，通过 `image[]` 上传原始文件。读取返回的 `b64_json` 并保存为原图。 |
| 生成数量 | 默认 1，多张任务逐张调用；发生错误时保留已完成结果。 |
| 尺寸 | 默认 `auto`；提供常用横竖比例与自定义宽高。宽高须为 16 的倍数、单边不超过 3840 像素、比例在 1:3 至 3:1、总像素为 655,360–8,294,400。 |
| 参考图 | 最多 16 张 PNG、JPEG 或 WebP，每张小于 50 MiB。 |
| 局部编辑 | 点选或框选生成透明 PNG `markup`，独立于原图发送，仅作用于第一张参考图；可以移除或重新选区。底图和标记图均不超过 16,777,216 像素。 |
| 背景与格式 | 自动、不透明、透明；PNG、JPEG、WebP。透明背景使用 PNG 或 WebP。 |
| 质量 | `auto`、`low`、`medium`、`high`、`xhigh`、`max`。 |
| 复杂度 | 模型默认（不传参数）、`low`、`medium`、`high`。 |
| 能力差异 | 不提供 Seedream 的图层拆分、结构化 OCR 和水印开关，不发送这些字段或 `response_format`。 |

本次接入覆盖静态图片生成与编辑。SVG、GIF、流式输出未接入。已使用模拟接口验证请求协议、参考原图与局部标记、批次生成、错误显示和会话保存；尚未使用 OpenAI 真实密钥验证出图效果。

依据：[图像生成指南](https://developers.openai.com/api/docs/guides/image-generation)、[图像提示与标记编辑](https://developers.openai.com/api/docs/guides/image-prompting)、[生成 API](https://developers.openai.com/api/reference/resources/images/methods/generate)、[编辑 API](https://developers.openai.com/api/reference/resources/images/methods/edit)。

## 文件保存

```text
data/
  config.json                   # 本地模型配置（包含 API Key）
  sessions/
    <session-id>/
      session.json              # 文本、参数、状态、用量、错误、图层元数据
      sources/
        <asset-id>.png          # 上传的原图或模型返回的原图
        <asset-id>.jpeg
```

`session.json` 中的 `assets[].path` 相对于该会话目录；`reference_ids`、OpenAI 局部编辑的 `markup_id` 和 `outputs[].asset_id` 引用同一 JSON 内的附件记录。JSON 不包含图片 Base64 或密钥。图层的坐标、顺序和 OCR 样式一并保存。

图片按原始字节保存，发给 API 和下载到本地时都不缩放、不重新编码。复制 PNG 时直接复制原图；其他浏览器支持的图片格式会按原始像素尺寸转换成剪贴板支持的 PNG。复制失败时界面显示原因，原图下载不受影响。

模型错误保留消息、错误码、HTTP 状态与 Request ID；多张任务已成功的图片继续可用。配置和会话 JSON 先写入临时文件再替换，避免写入被中断时截断已有 JSON。

`data/` 和 `bin/` 已加入 `.gitignore`。分享源码时不要附带个人配置或会话目录。应用只监听本机端口；配置密钥是本地明文文件，应按凭据保管。

## 检查

```sh
go test -buildvcs=false -race -timeout 60s ./internal/...
go vet -buildvcs=false ./...
```

测试覆盖原图上传→API 参考图→生成→原图下载、JSON 路径引用、无系统提示词、Seedream 5.5 与 OpenAI 2.6 参数构造、OpenAI 多参考图和独立标记上传、批次部分成功、错误信息与密钥隐藏、配置更新保留密钥、重启中断恢复及本地请求来源检查。测试使用本机模拟 API，不消耗模型额度。

开发时另已通过提供的 AgentPlan 接口验证：自动分辨率生成、框选指令编辑、透明 PNG、图层拆分和富文本 OCR。实测会话保存在本地 `data/sessions/`，不属于源码。

接口依据：[Seedream 5.5 教程](https://docs.volcengine.com/docs/ark/seedream-5-5?lang=zh)、[图片生成 API](https://docs.volcengine.com/docs/ark/image-generation-api?lang=zh)、[交互编辑指南](https://docs.volcengine.com/docs/ark/seedream-5-0-pro-editing-guide?lang=zh)、[AgentPlan 接入视觉模型](https://ark.volcengine.com/region:cn-beijing/docs/agent-plan-enterprise-visual-models)。

## 图标资源

图标以分层画布、风景画面、编辑选区和生成光芒表现图像创作功能，与界面的青绿色配色保持一致。原始 PNG 和多尺寸 ICO 位于 `assets/`；网页使用 `web/icon.png`、`web/favicon.ico`。Windows amd64 构建自动嵌入仓库中的 `resources_windows_amd64.syso`，无需额外工具。

更换图标后，可使用 `rsrc` v0.10.2 重新生成 Windows 资源：

```sh
rsrc -ico assets/pureagent.ico -o resources_windows_amd64.syso -arch amd64
```
