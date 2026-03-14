# ClawPanel Lite v0.1.8

发布时间：2026-03-12

## Lite runtime 依赖 hotfix

- 修复 Lite 包漏打 OpenClaw 运行时依赖的问题
- 补齐 `tslog` 等 production 依赖，解决 `dist/entry.js` 文件存在但无法导入的问题
- 统一收紧 Lite 的 launcher、CLI 与网关工作目录到内嵌 OpenClaw app 目录

## 打包链增强

- Lite 打包阶段新增 runtime smoke test
- 构建时直接校验 `import("./dist/entry.js")`
- 若运行时依赖缺失则直接中断打包，避免继续发布坏包

## 说明

- Lite 继续固定内嵌 OpenClaw `2026.2.26`
- 本版本用于修复 macOS 真机实测中暴露的 Lite runtime 启动失败问题
