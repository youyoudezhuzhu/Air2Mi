# fnOS 开发版安装与使用

当前 App 0.1.2，仅 x64；实际验证平台为 fnOS 1.2.0800 / glibc 2.36。应用通过 fnOS 统一网关和 Unix Socket 提供页面，无独立 LAN 管理端口。仅管理员可访问。普通 Linux 用户不适合直接安装 FPK。

## 安装

**2026-11-04分发规则：公开FPK不含内核。安装App后，需在设置中上传本地受信任签名内核ZIP；Host和UI在没有Engine时也能启动。本机测试可显式从本地文件装入，公开CI不下载或上传内核。以下涉及内核的测试步骤以已完成本地安装为前提。**

下载私有仓库 fnpack 工作流对应提交的 `Air2Mi.fpk` 与 `SHA256SUMS`，核对 SHA256，然后应用中心手动安装，或管理员运行 `appcenter-cli install-fpk Air2Mi.fpk -v 1`。应用中心启动 Air2Mi 后打开图标。服务初次只启动 Host，**不会自动扫描并占用全屋音箱**；在设置启动独立 Engine，然后搜索音箱（自动添加），创建接收器。

应用使用 fnOS 分配的 `TRIM_PKGVAR` 保存 Engine、设备身份、Registry、日志和备份。程序与 Socket 位于 `TRIM_APPDEST`。不接管其他数据目录中的 Engine，尤其不自动迁移已有 NAS 日用进程。并行测试不得复用原 AirPlay 名称、端口或物理音箱；正式替换旧服务应由用户明确切换，避免两个实例争用音箱。

## 管理范围

- 启停/重启本应用独立 Engine，查看真实 API 状态与数据面状态。
- 音箱发现复用 Engine `daily --prepare-only` 生成隔离结果，再通过 Speakers API 自动添加新设备，不直接覆写 Registry。
- 实际 Registry CRUD；关联通过 Input.options.speaker_id 调用真实 API。接收器内部标识和端口自动分配。
- 保存会重载音频服务，页面明确确认。当前编辑限每接收器单目标。
- API 尚未暴露独立实时音量、真实输出在线集合、完整多目标去重。本应用不伪造。
- River 展示只使用 Engine producer/references，不重新仲裁、不混音。
- 实时快照由 Host 每两秒读取真实 API 并 SSE 转发。页面隐藏即断开，恢复时重新连接；不是虚构的内核事件日志。

## 生命周期与数据

关闭网页不停止 Host 或 Engine。应用中心停止通过私有 Unix Socket 指令，验证管理进程身份后有序停止自己拥有的 Engine。升级脚本停旧 Host 后重启；首次启动 Engine 仍由设置页面控制。卸载脚本不删除配置/身份/备份，fnOS 系统卸载界面如提供用户数据删除选项，应自行保留。

## 更新与恢复

生产在线更新源未配置，页面据实提示。离线更新包 ZIP 只含 engine.run / manifest.json / manifest.sig；Host 先验 Ed25519 和 SHA256，再检查 linux/x64/API v1，备份用户数据后停止旧版。原子写版本指针，启动并检查 API；失败只回滚内核文件及版本指针，**不覆盖或删除当前用户配置**。配置迁移失败的人工恢复必须独立处理，备份不自动清理。

信任公钥随 FPK 固定，更新包不能携带替换信任锚。开发私钥未进仓库。更换公钥须通过可信 FPK 升级。当前私有 nonfree Engine 不允许自动公开发布。

## 本地构建

使用 Node 22.23.4、Go 1.27.3、Python 3（仅构建脚本，不是应用运行依赖）：

```sh
npm ci --prefix web
npm --prefix web run check
npm --prefix web test
npm --prefix web run build
go test -race ./hosts/fnos
python3 tools/download_fnpack.py
python3 tools/build_fpk.py --fnpack "$PWD/build/fnpack"
python3 tools/check_fpk.py dist/Air2Mi.fpk
```

fnpack 在独立临时 staging 中执行，避免对源码目录施加 ACL。Engine 二进制只放临时 build；默认 FPK 仅包含管理端，仓库没有第二份内核源码。

## 实机验收

1. 启动 App 后 API 未运行状态应明确；点击启动，应显示 API 已连接和音频数据面真实状态。
2. 添加或发现测试音箱，创建接收器（端口自动分配）。在同局域网 iPhone 中选择它，核对出声。
3. 添加其他一对一映射，检查多选、退出、主源切换与独立 AirPlay 音量。
4. 暂停/恢复、切曲、seek、切换时退出音箱；核对内核原有行为，不以网页状态替代听音。
5. 关闭网页仍播放，重新打开正确显示；应用中心停止才结束本应用服务。
6. 签名错误更新须拒绝，更新失败须保留身份/关联。初版不建议直接在日用数据上试错误迁移。

## 官方规范

按 [Manifest](https://developer.fnnas.com/docs/core-concepts/manifest/)、[统一网关](https://developer.fnnas.com/docs/core-concepts/gateway-registration/)、[fnpack](https://developer.fnnas.com/docs/cli/fnpack/) 实施。不同 fnOS 版本的网关及安全 Header 行为需要实际安装验证，不能只凭静态打包宣称通过。

## 交互修正（2026-11-03）

接收器只需填写名称、关联音箱、启用状态。新增时 Host 根据真实 Registry 与系统监听端口生成唯一 ID 和可用端口；编辑不重建 ID 或改端口。音箱搜索提供不定量进度状态，发现设备通过真实 Speakers API 自动新增；按 ID 或 IP 去重，保留已有设备的用户命名及关联。导入后可在设备详情重命名。仅新增记录触发 Engine 现有配置重载，重复搜索不重载已存在记录。

fnOS Unix 网关可能改写 Host。该受信任传输使用管理员登录态及私有 CSRF nonce 校验操作，不再把内部 Host 与浏览器公开 Origin 强行比较；仍拒绝跨站 Fetch、不合法 Origin、无认证/无nonce请求，开发 TCP 不信任 Forwarded Header 绕过来源检查。

## 当前交互

妙播音箱使用紧凑行布局，发现自动添加，设备详情可重命名。接收器只显示名称、单个关联音箱和启用状态；同一音箱不可由另一接收器占用，停用不释放关联。前后端均验证，保存仍会按真实 Engine 能力重载。

测试音先选音量，再播放；手动停止，最长五分钟。必须取消所有 AirPlay 投放。停止后保留所选设备音量，原接收器配置不变。详见 [测试音](TEST_TONE.md)。
