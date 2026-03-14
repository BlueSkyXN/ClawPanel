# ClawPanel Lite v0.1.5

发布时间：2026-03-11

## Windows / macOS Lite 脚手架推进

- 新增 `macOS Lite` 安装 / 卸载脚本
- 新增 `Windows Lite` 安装 / 卸载脚本
- 新增 Windows 专用 `clawlite-openclaw.cmd`
- Lite Core 包命名、更新元数据与构建链开始为 `darwin / windows` 预留正式位置

## 构建链调整

- GitHub Actions 中的 Lite 打包链开始按目标平台划分 runner
- 后续将优先在 `macOS runner` / `Windows runner` 上原生准备 runtime 和整包，而不是继续用 Linux 主机硬拼跨平台包

## 当前说明

- Linux Lite 仍是当前正式可用版本
- Windows / macOS Lite 仍处于预览推进阶段
- 当前建议用户继续优先使用 Linux Lite 进行正式安装与生产验证
