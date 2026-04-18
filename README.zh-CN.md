# hams 🐹

[![CI](https://github.com/zthxxx/hams/actions/workflows/ci.yml/badge.svg)](https://github.com/zthxxx/hams/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/zthxxx/hams)](https://goreportcard.com/report/github.com/zthxxx/hams)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**hams**（仓鼠）是一个面向 macOS 和 Linux 工作站的声明式 IaC 环境管理工具。就像仓鼠喜欢囤东西一样，hams 帮你囤积各种环境配置，以便安全保存和随时恢复。

它包装现有的包管理器（Homebrew、pnpm、npm、apt 等），**自动记录安装操作** 到声明式 YAML 配置文件（"Hamsfile"），实现 **一条命令恢复整个环境**。

## 快速开始

### 安装

```bash
# 一行安装（下载最新 release 二进制）
bash -c "$(curl -fsSL https://github.com/zthxxx/hams/raw/master/scripts/install.sh)"

# 或通过 Homebrew
brew install zthxxx/tap/hams
```

### 使用

```bash
# 安装软件包并自动记录
hams brew install htop

# 通过 pnpm 安装（自动添加 --global）
hams pnpm add serve

# 设置 git 配置并记录（natural git 语法走 `hams git`）
hams git config user.name "Your Name"

# 在新机器上一条命令恢复（如果还没装 brew，加上 --bootstrap）
hams apply --from-repo=your-username/hams-store --tag=macOS
```

### 工作原理

1. **通过 CLI 安装**：`hams brew install git` 执行 `brew install git` **同时** 将 `git` 记录到 `Homebrew.hams.yaml`
2. **同步到 Git**：推送你的 hams-store 仓库，包含所有 `*.hams.yaml` 文件
3. **随处恢复**：`hams apply --bootstrap --from-repo=you/hams-store` 在新机器上重放所有安装（新机器如果还没装 Homebrew 等前置工具，加上 `--bootstrap`）

## 特性

- **15 个内置 Provider，13 个 CLI 入口**：Homebrew、apt、pnpm、npm、uv、goinstall、cargo、VS Code（`hams code`，内部名 `code`）、mas（App Store）、git（`hams git config` + `hams git clone`，内部名 `git-config` + `git-clone`；其余所有 `hams git <verb>` 原样透传到真 `git`）、macOS defaults、duti、Ansible
- **Terraform 风格的状态管理**：跟踪已安装内容，自动重试失败项，检测配置漂移
- **保留注释的 YAML**：你在 Hamsfile 中的注释不会丢失
- **多机器 Profile**：一个 git 仓库，多台机器的配置（macOS、Linux、OpenWrt）
- **LLM 驱动的分类**：通过 Claude/Codex 自动生成标签和描述
- **Provider 插件系统**：通过 Go SDK 扩展自定义 Provider

## 对比

| 工具 | hams 的优势 |
|------|-----------|
| NixOS / nix-darwin | 务实而非严格；使用现有包管理器 |
| brew bundle | 多 Provider 支持；保留注释；有 hooks 和状态跟踪 |
| Ansible | CLI 优先的自动记录；无需先写 playbook |
| chezmoi | 管理软件包，不仅仅是 dotfile |
| Terraform/Pulumi | 主机级别而非云；边安装边记录 |

## 文档

完整文档：[hams.zthxxx.me/docs](https://hams.zthxxx.me/docs)

## 开发

```bash
task setup    # 安装开发工具
task build    # 构建二进制
task test     # 运行测试
task lint     # 运行 linter
task check    # fmt + lint + test
```

参见 [CLAUDE.md](CLAUDE.md) 了解项目架构和开发指南。

## 许可证

MIT
