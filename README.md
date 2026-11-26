# PureAgent

本地运行的图像创作工作台。Go 后端与内嵌 Web UI，无第三方运行依赖。
当前接入 Seedream 5.5 / AgentPlan 图片生成协议；多个配置可使用不同的模型名称、API Key 和完整接口 URL。

## 启动

Windows：下载下方 Actions 构建产物，解压后双击 `start.cmd`，然后打开 **http://127.0.0.1:57123**。保留终端窗口，关闭窗口即停止服务。从源码克隆的项目需先按下方命令构建。

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

推送、Pull Request 和手动运行均会触发 [Build PureAgent](https://github.com/UMR731x/PureAgent/actions/workflows/build.yml)。工作流使用 Go 1.27，运行竞态测试、Go 静态检查和前端 JavaScript 语法检查，然后生成 Windows/Linux amd64 可执行文件。

在一次成功运行的 **Artifacts** 区域下载：

- `PureAgent-windows-amd64`：解压后包含 `start.cmd`、`README.md` 和 `bin/PureAgent.exe`。
- `PureAgent-linux-amd64`：下载后解压其中的 `PureAgent-linux-amd64.tar.gz`，在解压目录运行 `./bin/pureagent`。tar 包保留可执行权限。

需要手动构建时，在 Actions 中选择该工作流并点击 **Run workflow**。CI 使用模拟图片接口测试，不需要模型 API Key。构建包不包含本地配置或会话数据；首次使用请在模型设置中填入自己的密钥。

## 使用

1. 右上角“模型设置”中保存配置。已有密钥留空时保持原值，界面不会回传已保存的密钥。
2. 输入描述，选择模型与生成参数。点击“生成”，或按 `Ctrl/⌘ + Enter`。
3. 参考图可通过附件按钮、拖入或粘贴上传；输入框上方的顺序对应“图 1、图 2”等编号。
4. 点击生成结果查看原图；“编辑”把选中图片作为参考图，再输入本轮修改要求。每次编辑都保存新结果。
5. “局部编辑”支持点选与框选，将官方 `<point>` / `<bbox>` 坐标插入输入框；可以直接修改指令。图片本身不会被画框、裁切或重新编码。
6. “更多生成选项”可选择透明背景、PNG/JPEG、水印与图层拆分。拆分只使用一张 PNG/JPEG，可填写拆分要求，也可留空让模型识别。
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

自动比例和分辨率使用 API 的真实语义，不发送生成接口不支持的 `size: "auto"`。本地页面显示的是任务进度，不是模型的流式生成。

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

`session.json` 中的 `assets[].path` 相对于该会话目录；`reference_ids` 和 `outputs[].asset_id` 引用同一 JSON 内的附件记录。JSON 不包含图片 Base64 或密钥。图层的坐标、顺序和 OCR 样式一并保存。

图片按原始字节保存，发给 API 和下载到本地时都不缩放、不重新编码。复制 PNG 时直接复制原图；其他浏览器支持的图片格式会按原始像素尺寸转换成剪贴板支持的 PNG。复制失败时界面显示原因，原图下载不受影响。

模型错误保留消息、错误码、HTTP 状态与 Request ID；多张任务已成功的图片继续可用。配置和会话 JSON 先写入临时文件再替换，避免写入被中断时截断已有 JSON。

`data/` 和 `bin/` 已加入 `.gitignore`。分享源码时不要附带个人配置或会话目录。应用只监听本机端口；配置密钥是本地明文文件，应按凭据保管。

## 检查

```sh
go test -buildvcs=false -race ./internal/studio
go vet -buildvcs=false ./...
```

测试覆盖原图上传→API 参考图→生成→原图下载、JSON 路径引用、无系统提示词、5.5 参数构造、批次部分成功、错误信息与密钥隐藏、配置更新保留密钥、重启中断恢复及本地请求来源检查。测试使用本机模拟 API，不消耗模型额度。

开发时另已通过提供的 AgentPlan 接口验证：自动分辨率生成、框选指令编辑、透明 PNG、图层拆分和富文本 OCR。实测会话保存在本地 `data/sessions/`，不属于源码。

接口依据：[Seedream 5.5 教程](https://docs.volcengine.com/docs/ark/seedream-5-5?lang=zh)、[图片生成 API](https://docs.volcengine.com/docs/ark/image-generation-api?lang=zh)、[交互编辑指南](https://docs.volcengine.com/docs/ark/seedream-5-0-pro-editing-guide?lang=zh)、[AgentPlan 接入视觉模型](https://ark.volcengine.com/region:cn-beijing/docs/agent-plan-enterprise-visual-models)。
