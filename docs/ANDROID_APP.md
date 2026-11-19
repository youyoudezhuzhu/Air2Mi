# Air2Mi Android 1.1.0

2026-11-13：新增仅本地使用的内置 Engine 测试变体，普通版仍不含内核。
状态栏适配已编译并通过回归；Linux 和 Android 真机安全校验结果、APK 及
构建方式见 [内置测试版报告](ANDROID_BUNDLED_TEST_20261113.md)。

Android 原生宿主位于 `android/`，与飞牛管理端共同使用仓库根目录的 `web/`。原生宿主负责本机生命周期和平台适配。

## 安装和内核导入

APK 不包含 Engine、AirPlay 接收器、MiPlay Sender、River 或 AAC 原生库，只带管理界面、宿主和通用 Python 运行时。安装后在“设置 → 内核安装与更新”手动选择独立的 Android ARM64 组件 ZIP。请使用组件提供方公布的 SHA256 校验本地文件。内核版本通过真实构建清单/API 返回，与 App 1.1.0 分开。

导入支持旧 `engine-source.zip` 和最新加固 `engine-native.zip` 格式；验证平台、ARM64 ELF 头、最低 API、API v1 和三个实际组件 SHA256。自带清单的 SHA256 只能检查完整性，不能代替发布者签名；请只导入可信来源。公开仓库和 APK 都没有内核。

已加载的原生库不会被覆盖：更新包先进入独立 pending 目录，下次进程启动重新校验后原子切换。界面会明确要求完全停止应用并重新打开，而不是把 Activity 重建称为内核更新。文件替换失败会恢复旧文件；旧组件备份和 `engine-data` 分开，安装/更新从不删除用户配置、身份、音箱及关联。新原生库启动失败后仍需用户重启进程再导入可信旧包，不能在已加载原生库的进程中热回滚。

当前 Engine 只有私有本地测试包；没有生产在线更新源。工作流 Artifact 会过期，不能当长期更新服务。App 不自动下载 Engine，不保存用户 CDK，也不自行判断激活权益。

## 与飞牛版同步的功能

| 功能 | Android 实现 |
|---|---|
| 概览、River、真实输出状态 | 相同 Vue 页面；读取 Engine 状态，不自行仲裁主源 |
| 接收器新增、改名、启停、删除 | Host 自动生成标识并检查端口；配置经 Engine API 保存 |
| 音箱与接收器一对一 | UI 和 Host 均验证已有关联，停用不释放关联 |
| 局域网搜索 | 调用已导入 Engine 自带发现函数，绑定 Android 局域网 IPv4；界面有进行状态 |
| 自动添加、重复扫描和地址变更 | 经 Engine API 合并，保留名称、关联及用户设置，稳定身份优先，不按型号判断设备 |
| 测试音、可选音量、手动停止、最长五分钟 | 固定本地 WAV + 原 Engine 文件会话；先拒绝活动投放，暂时停止日用 profile，结束后恢复；浏览器不能提供文件路径 |
| Plus 状态、激活、恢复、清除、商品 | 与飞牛相同页面和 Engine API；保留错误码与 Retry-After；商品缓存 60 秒并支持 ETag |
| 展示设备码 | Native Host 先过滤为 AVD1，再交给 WebView；错误和导出同样过滤，不把真实码交给前端后再隐藏 |
| 日志和诊断 | 本机日志搜索/等级过滤、脱敏、通过 Android 文件选择器导出诊断 |
| 内核安装 | Android 文件选择器和组件导入；不使用 Linux .run/FPK 安装逻辑 |

不虚构曲名、封面、播放进度或实时音量 API。iPhone 独立音量和多音箱同步仍由 Engine 处理。当前独立实时 Web 音量和多目标关联等飞牛端未提供的能力也没有额外添加空壳控件。

## 后台运行

* 用户点击启动后，独立的 `EngineService` 使用 `mediaPlayback|connectedDevice` 前台服务和常驻通知。关闭页面、锁屏或划走最近任务不会主动停止内核。
* CPU WakeLock、Wi-Fi Lock 和组播锁随内核运行持有；停止、启动失败和服务销毁时清理。网络回调和 NSD 注册有独立所有权及清理，不随 WebView 销毁。
* 记录用户期望状态；系统回收后 `START_STICKY` 可重新初始化已安装内核。显式停止会清除恢复标记，通知栏停止按钮也停止内核。
* 设置页显示真实通知/服务/电池优化状态，并提供系统设置入口；不擅自申请电池豁免或开机常驻。

Android 和厂商仍可能限制后台网络/进程，不能保证系统强行停止后自行复活。长时间待机应允许通知，并按系统设置取消电池优化；用户强行停止后需手动重新打开。参见官方 [前台服务类型](https://developer.android.com/develop/background-work/services/fgs/service-types)、[后台启动限制](https://developer.android.com/develop/background-work/services/fgs/restrictions-bg-start) 和 [Doze](https://developer.android.com/training/monitoring-device-state/doze-standby)。

## 构建

需要 Node.js 20.19+、JDK 17/21、Android SDK 35、Gradle 8.6。最低 Android API 29，当前目标 SDK 34，仅 ARM64。

```sh
npm --prefix web ci
npm --prefix web run check
npm --prefix web test -- --run
npm --prefix web run build
python3 -m unittest discover -s tests/android -v
gradle -p android :app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest
python3 tools/check_android_apk.py android/app/build/outputs/apk/debug/app-debug.apk
```

主 APK：`android/app/build/outputs/apk/debug/app-debug.apk`。测试 APK 不作为应用交付。两者均为开发签名；正式发布需保留固定签名证书和递增 versionCode（本版 11000）。

## 实际验证与边界

* Vue/TypeScript 检查、18 项 Web 测试、7 项 Android JVM 测试及管理适配测试已执行；详见 [本轮报告](ANDROID_TEST_REPORT_20261112.md)。
* Android 16 ARM64 真机已独立导入 beta.9 加固组件，实际通过 API、AAC、自带接收器加载、接收器自动编号/端口、商品查询及 AVD1 过滤。
* 实际关闭 Activity 后等待 60 秒，内核仍运行，Android ActivityManager 确认前台服务；通知停止动作生效。
* 没有借用/重新绑定用户 CDK，也没有据此宣称真实激活成功。
* 真 iPhone 连续投放、异机型/长时待机、多音箱听音、测试音实听和厂商省电策略仍需用户实机验收。自动测试不把模拟控制面当作真实音箱同步证据。

## 状态栏适配源码更新（2026-11-13，未编译）

状态栏和系统导航栏改为透明，使用深色图标，原生窗口底色与 Fluent 页面一致。安全区域统一由原生容器处理，保护状态栏、侧边刘海、手势/三键导航及键盘区域；不在 Web CSS 再叠加一次系统边距。Android 10 使用兼容布局标志与 adjustResize，Android 11 及以上使用 WindowInsets/IME。

本轮只修改并静态检查源码，未运行 Gradle、前端打包、JVM 测试或 APK 安装。新增的 Insets 回归用例需下轮允许构建后执行，不能计为已通过。上节 1.1.0 的构建/实机结果属于此前交付包，不代表此项已上机验证。参考 [Android 官方 edge-to-edge 指南](https://developer.android.com/develop/ui/views/layout/edge-to-edge)。

`Air2Mi Engine Test`（包名 `org.air2mi.engine.testhost`）是内核仓库的独立诊断宿主，曾用于加载、编码及启停验证；它不是正式 Air2Mi 的依赖。正常使用只需 `org.air2mi.app` 及其手动导入的内核，可卸载测试宿主。本轮未操作设备上的应用。
