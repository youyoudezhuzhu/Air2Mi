# Air2Mi

Air2Mi 在局域网中接收 AirPlay 2 音频，并通过妙播输出到兼容音箱。管理端用于发现设备、配置接收器和查看播放状态。

| 平台 | 管理端 | 公开测试内核 |
| --- | --- | --- |
| 飞牛 fnOS / Linux x64 | [fnOS 管理端、打包与安装说明](https://github.com/youyoudezhuzhu/Air2Mi/tree/fnpack) | Linux 单文件运行包 |
| Android ARM64 | [Android 管理端与构建说明](https://github.com/youyoudezhuzhu/Air2Mi/tree/android) | 可在设置中导入的组件 ZIP |

内核和应用分别发布。下载页提供平台要求、离线校验命令和外部校准文件示例。[内核 0.3.11-beta.14 公开测试版](https://github.com/youyoudezhuzhu/Air2Mi/tree/engine-0.3.11-beta.14-public-hardened)

## 客户端内核更新

飞牛通过签名更新 ZIP 安装内核，Android 通过组件 ZIP 独立更新。两端从本库 Pages 的 `/Air2Mi/updates/engine-stable.json` 读取已签名目录。

在仓库设置中选择 GitHub Actions 作为 Pages 来源，配置 `AIR2MI_UPDATE_KEY_B64`（现有客户端更新私钥 32 字节种子的 Base64）。维护者可手动运行 Publish Engine updates，选择本库已发布的二进制标签，或在发布正式 Engine Release 时附加公开 Linux .run、对应的 .manifest.json 和 Android -components.zip。发布任务核对公钥和摘要后部署目录与下载文件。
