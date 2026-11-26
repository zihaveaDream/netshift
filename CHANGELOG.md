# Changelog

All notable changes to **NetShift** will be documented in this file.

## [v0.1.1] - 2026-11-26

### Bug Fixes / 问题修复
- **DHCP 切换与网卡状态自动同步**：修复在 Windows 10/11 上由手动静态 IP 切换为自动获取（DHCP）后，因租约获取延迟导致网络适配器下拉框与状态徽章停留在「无 IP」、必须手动点击「刷新数据」的问题。现改为非阻塞后台轮询（不占用 `busy` 锁定按钮，最长 12 秒，受世代号与窗口关闭自动取消保护），获取有效非 APIPA IPv4 后即时同步下拉框、徽章、表单、高级页与系统托盘；同步覆盖预设切换、高级配置应用、快照回退、启用/禁用网卡及修改/还原 MAC 等场景。
- **IPv4 路由表管理与系统命令输出乱码**：修复打包版可执行文件因内嵌 `<activeCodePage>UTF-8</activeCodePage>` 清单导致进程内 `CP_OEMCP` 被覆写为 `65001`，进而把中文 Windows 子进程（`route print -4`、`netsh`、`ipconfig` 等）输出的 CP936/GBK 字节误按 UTF-8 解码的问题（例如路由表直连网关 `在链路上` 显示为 `▯▯▯▯·▯▯`）。新增多候选代码页控制台解码器 `netcfg.DecodeConsole`，优先从注册表 `HKLM\SYSTEM\CurrentControlSet\Control\Nls\CodePage` 读取不受清单覆写的真实 `OEMCP`/`ACP` 并进行严格/宽松两阶段转换。
- **Hosts 文件 ANSI/GBK 注释兼容**：读取系统 `hosts` 文件时若检测到非 UTF-8 编码，自动按系统控制台代码页解码，避免中文注释乱码。

### Docs & Chore / 文档与仓库
- 新增 MIT 许可证（`LICENSE`），在中英文 README 中补充 License 与 Awesome MyGo 徽章。
- 新增 `CHANGELOG.md` 并将版本号升级至 `v0.1.1`。

---

## [v0.1.0] - 2026-11-25

### Initial Release / 首次发布
- 基于 Go 1.27 + `mygo` 原生 GPU 渲染发布首个正式版本。
- 支持网卡启停与信息复制、IPv4 静态/DHCP 与多 IP/多网关/多 DNS/跃点数管理、ARP + ICMP IP 冲突预检与修改前快照一键回退。
- 支持按项目地点管理预设方案与同地点配置继承、内网分流路由与可视化 IPv4 路由表管理、连通性诊断与 TCP 端口探测、254 主机并发 ICMP 群 Ping、CIDR 子网计算器、系统代理与 Hosts 快捷管理、系统托盘快捷切换及操作审计日志。
- 修复 `netsh` `set/add address` 非法 `validate=no` 参数并增强 `netsh` 输出错误检测与 DHCP 幂等切换。
