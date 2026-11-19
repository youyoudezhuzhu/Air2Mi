# Engine 能力审查（开发前）

审查原仓库 beta 实际源码，当前本地引用构建固定为 0.3.11-beta.1（API v1）。输出观测与旧音量回报已按用户授权修复；后续测试音和 UI 工作未再修改内核。原仓库代码不进入本仓库；以下列出应用依赖的接口。

| 功能 | 状态 / API / CLI | Host 安全调用与验证 | UI 呈现 / 是否需改内核 |
|---|---|---|---|
| Host 生命周期 | 已实现 initialize/start/stop/restart/status/version/capabilities/doctor；.run host CLI；POST /v1/engine/stop | 仅管理自己 data-dir 及 PID 身份，调用固定二进制；Linux x64 已实机 | 可操作；不改内核 |
| 健康及版本 | GET /v1/health、/v1/config | health ready 只证明管理 API；state.kernel.state 才表示数据面状态 | 分开展示 API 与音频进程，不能假绿 |
| 状态及 River | GET /v1/state、/v1/river；producer/references/paused/generation/output_bytes | producer 是内核选的主源，references 是内核给出的有效输入集合 | 只读；禁止前端仲裁；不增加指标 |
| 接收器 Registry | GET/POST/PATCH/DELETE /v1/inputs | 真正持久化；修改都会 runtime.reload，可能全体短时中断 | 支持增删改启停，保存前明确重载提示 |
| 音箱 Registry | GET/POST/PATCH/DELETE /v1/speakers | 配置记录，不等于在线状态 | 管理名称/IP/型号/启用；不能把记录当在线 |
| Routes | 模型允许多输入、多目标；数据面只取优先路由首个目标，input.options.speaker_id 优先 | 部分实现。多输入同物理输出的数据面去重不可由 App 补做 | 当前编辑只开放真实单目标关联；多关联只读提示，列为内核需求，不改源码 |
| 多路 AirPlay 与独立音量 | 数据面已实现，多台真实音箱测试通过 | AirPlay 端控制音量真实生效 | API 尚未提供独立实时音量操作；Web 不做假滑杆 |
| 配置音量 PATCH | PATCH Speaker.volume 会完整 reload；启动还可能读取音箱实际音量覆盖配置值 | 不适合作为实时音量接口 | 不包装成实时音量控制；只读配置/最近输入报告音量并注明来源 |
| 输出在线/连接状态 | 测试版已提供按音箱的 selected/playback_state/connected/first_audio_pcm 等观测，river 包含选中输入和输出集合 | 已接入真实 API；connected 缺失时为未知，非局域网持续在线检测 | 动画只随真实播放状态；配置关联和未知值独立呈现 |
| mDNS 发现 | /v1/speakers/scan 固定 501；CLI daily --prepare-only 可实际扫描并生成预览 Registry | 复用固定单文件内的既有 CLI；独立预览 data-dir，不启动 AirPlay/MiPlay，不复制协议 | 显示搜索状态，发现结果通过 Engine API 自动新增，重复扫描保留用户命名；不伪造热扫描 API |
| SSE | /v1/events 已实现，但默认 Host 未配置 events_path 时只发心跳 | 原样代理可用；默认不能依赖它传状态 | Host 从真实 API 定频取快照再 SSE 发给 UI，标明来源；不伪造 River 事件 |
| 会话 | POST /v1/sessions 与会话停止接口。已有 file/pcm 会话，只允许单目标 | Host 在空闲时用隔离临时 Engine 的既有 file profile 播放固定测试资源，不接受浏览器路径 | 单音箱测试、音量选择、手动停止及最长五分钟；不做任意文件播放器 |
| doctor | doctor CLI 真实验证 AES、导入和流式 PCM→AAC | 有界子进程、独立缓存；不操作音箱 | 提供诊断；不冒充实机同步验收 |
| 日志 | 未有通用日志 API | Host 读取自己拥有的限定日志，脱敏；不读取私有身份/媒体密钥文件 | 真实日志检索/导出；不用日志替代缺失控制 API |
| Linux 单文件 | x64/ARM64 .run 包含 Python/Rust/FFmpeg；glibc>=2.36+系统 Avahi/D-Bus 仍需要 | x64 日用实机通过，ARM64 编译/QEMU，fnOS ARM64 未实测 | FPK 首版只启用 x64；不额外要求用户装 Python/FFmpeg/Docker |
| 更新/回滚 | 有可信签名校验/二进制回滚辅助；没有生产在线发布源/自动更新 API | Host 独立实现应用层下载/验证/停机/切换/健康/二进制回退；不删除用户树 | 显示源与签名配置真实状态；缺配置时不显示虚假安装更新按钮 |

## 范围与缺口

UI 仅在 Engine 提供真实值时显示动画/状态。配置关联边不是实时媒体连接。不可用接口返回“接口不支持”，不能显示成空设备。任意 Registry 修改都应提示可能重建数据面。

以下不在本次擅自修改原内核：实时音箱控制 API、局域网持续在线检测、配置化的事件流。详见 ENGINE_API_REQUESTS.md。已有多接收器一对一映射是本次实用管理基线。

用户随后明确要求一对一：App 同时限制每输入一个目标和每音箱一个所有者，停用记录仍保留关联；Host 同时校验 inputs 与 routes 并串行化变更。此为管理策略，不改 River 仲裁。远端 FPK 不含 Engine，内核仅本地安装。
