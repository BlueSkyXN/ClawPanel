# ClawPanel Lite v0.1.0

发布时间：2026-03-10

## 首个 Linux 可用版

- 新增 `ClawPanel Lite` 发行形态，安装后直接内置 OpenClaw `2026.2.26`
- Lite 默认托管运行时，不支持切换到外部 OpenClaw
- 新增 `clawlite-openclaw` 专用命令，避免和用户本机已有 `openclaw` 冲突

## 开箱即用体验

- 安装完成后，用户只需配置 API Key 与通道凭据即可开始使用
- 默认内置并托管常用插件：`telegram`、`qq`、`qqbot`、`feishu`、`dingtalk`、`wecom`
- 所有预置通道默认关闭，但已内置可配置，配置完成即可直接启用

## Lite 面板与更新

- 新增 Lite 专用安装脚本：`scripts/install-lite.sh`
- 新增 Lite 专用卸载脚本：`scripts/uninstall-lite.sh`
- Lite 面板已接入独立版本线更新能力，更新时仅跟随 Lite 版本
- Lite 安装与更新默认优先走 GitHub Release，超时或失败时自动回退到加速服务器

## 运行时与通道修复

- 修复 Lite 内嵌 OpenClaw / 网关启动失败问题
- 修复 Lite 预置插件启用态被错误覆盖的问题
- 修复 Telegram、QQBot、飞书、钉钉、企业微信在 Lite 中的基础可用性问题
- 企业微信智能机器人调整为更简单的 `botId + secret` 配置方式

## 发布说明

- Lite 当前版本号：`0.1.0`
- Lite 当前主包：`clawpanel-lite-core-v0.1.0-linux-amd64.tar.gz`
- QQ NapCat 仍维持独立 Bundle 方案，未并入 Lite 主包
