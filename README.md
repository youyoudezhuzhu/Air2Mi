# Air2Mi

Air2Mi 跨平台管理应用。第一阶段开发飞牛 fnOS 原生 FPK：管理独立 Air2Mi Engine，通过真实 API 展示接收器、妙播音箱和 River 状态。

- `main`：基础结构与项目说明。
- `fnpack`：fnOS Host、Vue 3 Web UI、FPK 打包及验证。

Engine 由独立的内核项目开发和编译。本仓库不复制或维护 AirPlay/MiPlay/River 源码，只引用固定构建产物。App 与 Engine 分别版本化。

Windows 和 Android 宿主以后独立规划，本阶段不创建空壳工程。
