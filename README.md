# Air2Mi Engine 公开测试版

版本为 `0.3.11-beta.14-public-hardened`。请选择与运行管理端的设备匹配的包。

| 平台 | 文件与用途 |
| --- | --- |
| Linux x86-64，glibc 2.36 或更新 | [下载单文件运行包](https://github.com/youyoudezhuzhu/Air2Mi/raw/refs/tags/engine-0.3.11-beta.14-public-hardened/air2mi-engine-0.3.11-beta.14-public-hardened-linux-x64.run)，可直接在终端运行 |
| Android 10 或更新，ARM64 | [下载内核组件 ZIP](https://github.com/youyoudezhuzhu/Air2Mi/raw/refs/tags/engine-0.3.11-beta.14-public-hardened/air2mi-engine-0.3.11-beta.14-public-hardened-android-arm64-components.zip)，在 Android 管理端设置中选择本地导入 |

## 核验下载

将所需的成品与 [SHA256SUMS](https://github.com/youyoudezhuzhu/Air2Mi/raw/refs/tags/engine-0.3.11-beta.14-public-hardened/SHA256SUMS) 保存到同一目录，在该目录执行下列命令。Linux 的 `doctor` 会离线检查导入、加密依赖和 PCM 转 AAC。

```sh
sha256sum --ignore-missing -c SHA256SUMS
chmod +x air2mi-engine-0.3.11-beta.14-public-hardened-linux-x64.run
./air2mi-engine-0.3.11-beta.14-public-hardened-linux-x64.run doctor
```

Android 导入时会再次核对平台、API 版本和各组件的 SHA-256。若管理端提示需要重启，请重启后再使用。fnOS 管理界面的更新入口另需受信任发布者签名的更新 ZIP。本页的 Linux 文件适用于命令行。

## 设备校准

公开版从空设备表开始，所有音箱的默认校准偏移为 0 ms。通过 Hub 的 `--calibration FILE` 参数可指定自己的 JSON 文件。以下示例把第二台音箱提前 17.25 ms，以补偿它相对参考设备的延迟。请将示例地址替换为自己的设备地址。

```json
{"reference":"192.0.2.10","devices":{"192.0.2.10":0.0,"192.0.2.11":17.25}}
```

包内附第三方组件的许可、声明和编解码器对应源码。Android 包还包含编解码器重链接材料。内核业务模块以编译形式交付。
