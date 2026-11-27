# PureAgent

Windows 本地图像创作工作台。Go 后端与 [MyGo](https://mygo.run/) 桌面窗口，使用系统 WebView2 显示内嵌界面，保留原有布局、配色和交互。
界面和 Go 后端通过 MyGo 进程内资源通道通信，不启动 HTTP 服务器、不监听本地端口。模型调用、额度查询和 GitHub 更新仍使用相应服务的在线 API。
支持 Seedream 5.5 / AgentPlan 与 OpenAI Images（GPT Image 2.6）。可保存多个模型配置，分别选择接口类型、模型名称、API Key 和 URL。

## 启动

在 [GitHub Release](https://github.com/UMR731x/PureAgent/releases/latest) 下载 `PureAgent-windows-amd64.zip`，解压后双击 `bin/PureAgent.exe`，直接打开独立桌面窗口，不显示控制台。需要 Microsoft Edge WebView2 运行时；Windows 10 / 11 通常已安装，缺少时可从 [微软官网](https://developer.microsoft.com/microsoft-edge/webview2) 安装。

通知区域显示软件图标，单击打开工作台；右键菜单提供“打开工作台”“打开数据目录”“退出 PureAgent”，保留原有样式。关闭窗口会隐藏到托盘，后台任务继续运行；从托盘退出才会结束进程。同一数据目录只运行一个实例，重复启动会唤起已有窗口；不同数据目录可分别运行。不再提供 Linux 客户端。

Windows Release 默认使用安装目录中的 `data`：客户端位于 `bin` 内时使用 `bin` 上一级的 `data`；单文件客户端使用自身所在目录的 `data`。显式 `-data` 参数仍按启动时的工作目录解析。

从源码运行（Windows，Go 1.27.3 或更新版本）：

```sh
go run -buildvcs=false .
```

Windows 构建：

```powershell
go build -buildvcs=false -ldflags "-H=windowsgui" -o bin/PureAgent.exe .
.\start.cmd
```

数据目录可以在启动时指定：

```powershell
.\bin\PureAgent.exe -data .\data
```

默认数据目录是**当前工作目录**下的 `data/`；`start.cmd` 会先进入项目根目录。`-buildvcs=false` 使从不含 Git 元数据的源码副本构建时也能工作。

旧启动参数 `-port` 仅保留升级兼容，用于识别旧界面偏好的存储位置，不再打开端口。


### 客户端更新

左下角“检查并更新”会读取本仓库最新正式 Release，下载匹配当前系统的客户端并校验下载完整性。已有生成任务会先完成，随后自动替换客户端、重启并恢复页面；新版本正常启动后删除旧客户端和临时更新文件，启动失败则恢复原版本并显示错误。

更新只替换运行中的可执行文件，保留原工作目录和实际使用的数据目录；配置、会话 JSON、上传原图和生成结果不会被迁移或删除。桌面客户端自动重启后恢复托盘、窗口中的当前任务和未提交输入。从使用本机端口的桌面版升级时，会将旧页面的主题、模型选择和更新草稿转入内置页面。请勿在更新过程中移动安装目录。

从浏览器版首次升级时，旧浏览器中的未提交输入和外观偏好不会自动转入桌面窗口；已保存的任务、原图和模型配置直接沿用原有 `data`。

没有更新入口的旧客户端需要先手动替换一次 `bin` 中对应文件，保留 `data`。源码开发构建显示 `dev`，不会自行替换源码运行产生的临时程序；一键更新适用于 Release 客户端。

## 使用

右上角的外观按钮提供“日间模式”“夜间模式”和“跟随系统”。首次使用保持日间配色，选择保存在桌面窗口的 WebView2 用户配置中；“跟随系统”会实时响应系统明暗变化。

1. 右上角“模型设置”中选择接口类型并保存配置。旧配置仍按 Seedream 处理。同一接口类型的已有密钥留空时保持原值；切换接口类型需填写对应密钥，界面不会回传已保存的密钥。
2. 输入描述，选择模型与生成参数。点击“生成”，或按 `Ctrl/⌘ + Enter`。
3. 参考图可通过附件按钮、拖入或粘贴上传；输入框上方的顺序对应“图 1、图 2”等编号。
4. 每轮结束后，其输出图片自动加入输入框附件；继续输入修改要求即可连续编辑。也可移除不需要的图片，或点击某张结果的“编辑”指定参考图。打开历史任务时带入最近一轮的输出；新任务不继承其他任务的图片。每次编辑都保存新结果。
5. “局部编辑”支持点选与框选。Seedream 将 `<point>` / `<bbox>` 坐标插入输入框；OpenAI 为第一张参考图创建独立的透明 PNG 标记，并填入可修改的编辑指令。原图始终保留原始字节。
6. “更多生成选项”按接口显示能力：Seedream 提供背景、PNG/JPEG、水印与图层拆分；OpenAI 提供背景、PNG/JPEG/WebP、质量与复杂度。Seedream 拆分只使用一张 PNG/JPEG，可填写拆分要求，也可留空让模型识别。
7. 拆分结果逐层保存。打开文字图层可查看、复制 OCR 文本，以及位置和文字样式；各图层也可继续交给模型编辑。
8. 历史记录中可点击会话旁的删除图标，或进入“管理”勾选多个会话、使用“全选”后删除。确认框会列出所选会话；删除会永久清理对应目录中的记录、上传图片与生成图片。正在生成的任务需先停止后再删除，全选会保留这些任务。删除当前会话后回到新任务，其他会话和模型配置保持不变。

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

只提交当前输入框内的提示词、附件中的参考图与生成选项。不设置系统提示词，不拼接历史对话文本。连续编辑自动带入上一轮输出的原图，用户手动移除后不会被轮询重新添加；部分成功的任务仍带入已完成的图片。上一轮没有输出时不回退到更早的图片。若拆分结果超过模型参考图上限，需要从附件中移除多余图片后再生成。

Seedream 的自动比例和分辨率使用 API 的真实语义，不发送该生成接口不支持的 `size: "auto"`。本地页面显示的是任务进度，不是模型的流式生成。

## OpenAI GPT Image 2.6 参数

在模型设置中选择 **OpenAI Images · GPT Image 2.6**，填入 `gpt-image-2.6-flare` 或 `gpt-image-2.6-sunburst`、对应 API Key 和 `https://api.openai.com/v1`。也可填写兼容服务的模型别名、基础 URL 或完整 `/images/generations`、`/images/edits` 地址。自定义路径前缀和查询参数会保留；服务需实现对应的 Images 协议。

| 选项 | 行为 |
| --- | --- |
| 生成与编辑 | 纯文本使用 JSON `/images/generations`；有参考图时使用 multipart `/images/edits`，通过 `image[]` 上传原始文件。读取返回的 `b64_json` 并保存为原图。 |
| 生成数量 | 默认 1，多张任务逐张调用；发生错误时保留已完成结果。 |
| 静态图尺寸 | 默认 `auto`；提供常用横竖比例与自定义宽高。宽高须为 16 的倍数、单边不超过 3840 像素、比例在 1:3 至 3:1、总像素为 655,360–8,294,400。 |
| 参考图 | 最多 16 张 PNG、JPEG 或 WebP，每张小于 50 MiB。 |
| 局部编辑 | 点选或框选生成透明 PNG `markup`，独立于原图发送，仅作用于第一张参考图；可以移除或重新选区。底图和标记图均不超过 16,777,216 像素。 |
| 背景与格式 | 自动、不透明、透明；PNG、JPEG、WebP、GIF。透明背景使用 PNG 或 WebP。 |
| GIF 动图 | 在更多生成选项中选择 GIF，支持文字生成动画或用静态参考图引导动画。帧数为 2–16，默认 9；每帧时长至少 20 ms，默认 200 ms，由 API 按 10 ms 取整。动画数量沿用生成数量选项。 |
| GIF 尺寸 | 默认 `auto`，也可选择 512 × 512 等预设或自定义宽高；宽高为 16 的正整数倍。具体尺寸与总帧像素预算由模型校验，不套用静态图的最小像素限制。 |
| 质量 | `auto`、`low`、`medium`、`high`、`xhigh`、`max`。 |
| 复杂度 | 模型默认（不传参数）、`low`、`medium`、`high`。 |
| 能力差异 | 不提供 Seedream 的图层拆分、结构化 OCR 和水印开关，不发送这些字段或 `response_format`。 |

GIF 需要账户具备对应模型的 GIF 预览权限。结果在工作台和查看器中直接播放，保存和下载均保留完整动画；复制按钮复制首帧为 PNG。OpenAI 参考图仍需为 PNG、JPEG 或 WebP，因此 GIF 输出不会自动作为下一轮附件；可点击“使用此指令”恢复提示词、静态参考图和动画参数。

本次接入覆盖静态图片生成与编辑、GIF 动画生成。SVG、流式输出未接入。已使用模拟接口验证请求协议、参考原图与局部标记、批次生成、GIF 原始字节保存与下载、错误显示和会话保存；尚未使用 OpenAI 真实密钥验证出图效果。

依据：[图像生成指南](https://developers.openai.com/api/docs/guides/image-generation)、[图像提示与标记编辑](https://developers.openai.com/api/docs/guides/image-prompting)、[生成 API](https://developers.openai.com/api/reference/resources/images/methods/generate)、[编辑 API](https://developers.openai.com/api/reference/resources/images/methods/edit)。

## 用量与额度

右上角“额度”位于“模型设置”旁边，查询输入框当前选中的模型。“刷新”重新访问官方接口；查询凭据按模型保存在本地 `config.json` 的 `usage` 中，浏览器仅收到是否已保存的标记，模型设置的普通编辑不会覆盖这些凭据。

- **Seedream / AgentPlan**：填写火山引擎 Access Key 与 Secret Key；企业版还需填写 Seat ID。生图 API Key 不能替代 AK/SK。显示适用于图片的近一天、近一月 AFP 已用量、剩余量、总额和重置时间，以及当前模型的本月用量明细。AFP 是套餐或席位的共享额度，不能视为该模型独享或直接换算成图片数；套餐内、套餐外和不同计量单位分别统计。模型明细可能延迟半天至一天。
- **OpenAI**：填写组织管理员的 Admin API Key，按模型名称查询本月组织内所有项目的图片处理量和请求次数。普通生图 Key 无法查询组织用量；官方没有余额查询 API，因此剩余额度显示说明并链接到官方账单页面。

两类查询均访问对应官方域名，不会将管理凭据发给自定义生图 URL。第三方中转服务没有统一的额度协议，其实际余额需到该服务的控制台查看。查询失败会显示错误；某项数据缺失不会被替换为 0，也不影响生图。已使用模拟接口验证个人版、企业席位、分页、模型筛选和错误显示；真实账户查询仍需对应管理凭据验证。

接口依据：[AgentPlan 套餐额度](https://docs.volcengine.com/docs/ark/get-afp-usage-api?lang=zh)、[模型用量](https://docs.volcengine.com/docs/ark/get-usage-details-api?lang=zh)、[企业席位额度](https://docs.volcengine.com/docs/ark/get-seat-afp-usage-api?lang=zh)、[企业席位用量](https://docs.volcengine.com/docs/ark/get-seat-usage-details-api?lang=zh)、[OpenAI Images Usage](https://developers.openai.com/api/reference/resources/admin/subresources/organization/subresources/usage/methods/images)。

## 文件保存

```text
data/
  config.json                   # 本地模型配置（包含生图与额度查询凭据）
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

`data/` 和 `bin/` 已加入 `.gitignore`。分享源码时不要附带个人配置或会话目录。应用通过内置页面操作本地数据；配置密钥是本地明文文件，应按凭据保管。

## 检查

```sh
go test -buildvcs=false -timeout 60s ./internal/...
go vet -buildvcs=false ./...
```

测试覆盖原图上传→API 参考图→生成→原图下载、JSON 路径引用、无系统提示词、Seedream 5.5 与 OpenAI 2.6 参数构造、OpenAI 多参考图和独立标记上传、批次部分成功、错误信息与密钥隐藏、配置更新保留密钥、重启中断恢复及本地请求来源检查。测试使用本机模拟 API，不消耗模型额度。

开发时另已通过提供的 AgentPlan 接口验证：自动分辨率生成、框选指令编辑、透明 PNG、图层拆分和富文本 OCR。实测会话保存在本地 `data/sessions/`，不属于源码。

接口依据：[Seedream 5.5 教程](https://docs.volcengine.com/docs/ark/seedream-5-5?lang=zh)、[图片生成 API](https://docs.volcengine.com/docs/ark/image-generation-api?lang=zh)、[交互编辑指南](https://docs.volcengine.com/docs/ark/seedream-5-0-pro-editing-guide?lang=zh)、[AgentPlan 接入视觉模型](https://ark.volcengine.com/region:cn-beijing/docs/agent-plan-enterprise-visual-models)。


