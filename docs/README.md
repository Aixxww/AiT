# 📚 AiT Documentation Center / 文档中心

Welcome to the AiT documentation! This page helps you find the right documentation quickly.

欢迎来到 AiT 文档中心！本页面帮助您快速找到所需文档。

---

## 🚀 Getting Started / 快速开始

**New to AiT? Start here!**

| Document | Description | 描述 |
|----------|-------------|------|
| [Main README](../README.md) | Project overview, features, quick start | 项目概述、功能、快速入门 |
| [Getting Started Index (EN)](getting-started/README.md) | All deployment options | 所有部署选项 |
| [Getting Started Index (中文)](getting-started/README.zh-CN.md) | 所有部署选项 | All deployment options |
| [Custom API (EN)](getting-started/custom-api.en.md) | Connect custom AI API providers | 连接自定义 AI API |
| [Custom API (中文)](getting-started/custom-api.md) | 连接自定义 AI API 提供商 | Custom AI provider guide |

**Quick Links:**
- 📖 See all options → [Getting Started](getting-started/README.md) / [快速开始](getting-started/README.zh-CN.md)
- 🤖 Custom AI model? → [Custom API (EN)](getting-started/custom-api.en.md) / [自定义 API](getting-started/custom-api.md)

---

## 📘 User Guides / 使用指南

**Learn how to use AiT effectively**

| Document | Description | 描述 |
|----------|-------------|------|
| [User Guides Index (EN)](guides/README.md) | All usage guides and tips | 所有使用指南和技巧 |
| [User Guides Index (中文)](guides/README.zh-CN.md) | 所有使用指南和技巧 | All usage guides and tips |
| [FAQ (English)](guides/faq.en.md) | Frequently asked questions | 常见问题解答 |
| [FAQ (中文)](guides/faq.zh-CN.md) | 常见问题解答 | Frequently asked questions |
| Troubleshooting *(coming soon)* | Common issues and solutions | 故障排查 |
| Configuration Guide *(coming soon)* | Advanced configuration options | 高级配置选项 |
| Trading Strategies *(coming soon)* | AI trading strategy examples | AI 交易策略示例 |

---

## 👥 Community & Contributing / 社区与贡献

**Join the community and contribute!**

| Document | Description | 描述 |
|----------|-------------|------|
| [Security Policy](../SECURITY.md) | Report security vulnerabilities | 报告安全漏洞 |
| [Git Workflow](Git工作流规范.md) | Repository workflow and Git conventions | Git 工作流规范 |

**Get Involved:**
- 💬 [Telegram Community](https://t.me/ait_dev_community)
- 🐦 [Twitter @ait_official](https://x.com/ait_official)
- 🐛 [Report Issues](https://github.com/Aixxww/AiT/issues)

---

## 🏗️ Architecture & Development / 架构与开发

**For developers who want to understand the internals**

| Document | Description | 描述 |
|----------|-------------|------|
| [Architecture Overview (EN)](architecture/README.md) | System architecture, modules, and design | 系统架构、模块和设计 |
| [Architecture Overview (中文)](architecture/README.zh-CN.md) | 系统架构、模块和设计 | System architecture overview |
| API Reference *(coming soon)* | HTTP API documentation | HTTP API 文档 |
| Database Schema *(coming soon)* | SQLite database structure | SQLite 数据库结构 |
| Testing Guide *(coming soon)* | How to write tests | 如何编写测试 |

---

## 🎯 Hunter 选币模块 / Hunter Coin Selection

**智能选币系统 — 基于资金流向、持仓异动、多空比的双向信号**

| Document | Description | 描述 |
|----------|-------------|------|
| [Hunter v7 Signal Router 架构](architecture/HUNTER_V7_SIGNAL_ROUTER.zh-CN.md) | SnapshotStore-based v7 signal router and JSON protocol | SnapshotStore 热快照、多形态信号路由、AIT JSON 标签协议 |
| [Hunter v7 全链路信号召回与策略胜率改造方案](hunter-v7-fullchain-signal-strategy-optimization-plan-20260611.md) | Full-chain plan for signal recall, execution readiness, tier/prompt consistency, and trading guard feedback | 数据源、Hunter v7、策略分层、Prompt、执行风控与归因闭环的系统改造方案 |
| [Hunter v7 实时链路校准实施方案](hunter-v7-realtime-calibration-implementation-plan-20260609.md) | Binance live-data validation, REST pacing, OI notional, amplitude/detail calibration | Binance 实时数据验证、REST 限速、OI 名义值、大波动入口校准 |
| [Hunter v7 架构与标签语义治理](hunter-v7-architecture-tag-taxonomy-20260609.md) | Tag catalog, LLM action semantics, and minimal-code architecture cleanup | 标签 catalog、LLM 行为语义和最小代码量架构治理 |
| [Hunter v7 漏斗优化整改报告](hunter_v7_整改报告.md) | Signal records, mover audit, watch state, displacement setup, and prompt gating implementation | 信号归因、大波动审计、Watch 升级、位移 setup 与提示词分层落地 |
| [Sniffer Gate 2 优化方案](sniffer-optimization-plan.md) | Flexible compression scoring plan for Sniffer Gate 2 | Sniffer Gate 2 弹性压缩评分优化方案 |

---

## 🧾 Reviews & Reports / 复盘报告

**Recent engineering and live-trading reviews**

| Document | Description | 描述 |
|----------|-------------|------|
| [2026-06-07 Session Review](reports/ait-session-review-20260607.md) | Hunter v7 candidate visibility, live open failure root causes, risk geometry fixes, and dashboard performance changes | Hunter v7 候选可见性、实盘开仓失败根因、风控几何修复与看板性能优化 |
| [2026-06-08 Hunter v7 / VVV Live Monitor](hunter-v7-vvv-live-monitor-2026-06-08.md) | VVV live-trading review for Hunter v7 open rate, win-rate regressions, signal tags, and LLM execution quality | VVV 实盘复盘：Hunter v7 开仓率、胜率回归、信号标签和 LLM 执行质量 |

---

## 🗺️ Roadmap / 路线图

**AiT's strategic development plan and market expansion**

| Document | Description | 描述 |
|----------|-------------|------|
| [Roadmap (EN)](roadmap/README.md) | Short-term and long-term roadmap, feature timeline | 短期和长期路线图、功能时间表 |
| [Roadmap (中文)](roadmap/README.zh-CN.md) | 短期和长期路线图、功能时间表 | Strategic development plan |

**Roadmap Highlights:**
- 📈 **Short-term (Q2 2026)**: Hunter 双向选币、AI500 增强、更多交易所集成
- 🚀 **Mid-term (Q3-Q4 2026)**: 策略回测引擎、多 AI 模型集成决策、强化学习
- 🌍 **Long-term (2027)**: 全市场扩展（股票、期货、期权、外汇）、企业级功能

---

## 📄 Legal & Policies / 法律与政策

| Document | Description | 描述 |
|----------|-------------|------|
| [License (AGPL-3.0)](../LICENSE) | Open source license | 开源许可证 |
| [Changelog (EN)](../CHANGELOG.md) | Version history and updates | 版本历史和更新 |
| [Changelog (中文)](../CHANGELOG.zh-CN.md) | 版本历史和更新 | Version history and updates |
| [Security Policy](../SECURITY.md) | Vulnerability disclosure | 漏洞披露政策 |
| [Git Workflow](Git工作流规范.md) | Repository workflow and conventions | Git 工作流规范 |

---

## 🔍 Quick Navigation / 快速导航

**Find what you need fast:**

### I want to...
- 🚀 **Get started quickly** → [Getting Started](getting-started/README.md) / [快速开始](getting-started/README.zh-CN.md)
- 🐛 **Report a bug** → [GitHub Issues](https://github.com/Aixxww/AiT/issues/new)
- 💡 **Suggest a feature** → [Feature Request](https://github.com/Aixxww/AiT/issues/new?template=feature_request.md)
- 🔒 **Report security issue** → [Security Policy](../SECURITY.md)
- 🤝 **Contribute code** → [Git Workflow](Git工作流规范.md)
- 💬 **Ask questions** → [Telegram Community](https://t.me/ait_dev_community)
- 🔍 **Hunter module** → [Hunter v7 Signal Router](architecture/HUNTER_V7_SIGNAL_ROUTER.zh-CN.md)

### I'm looking for...
- 🏗️ **System architecture** → [Architecture (EN)](architecture/README.md) / [架构文档](architecture/README.zh-CN.md)
- 🗺️ **Product roadmap** → [Roadmap (EN)](roadmap/README.md) / [路线图](roadmap/README.zh-CN.md)
- 📊 **API documentation** → Coming soon
- 🧪 **Testing guide** → Coming soon
- 🔧 **Configuration examples** → [Custom API (EN)](getting-started/custom-api.en.md) / [自定义 API](getting-started/custom-api.md)

---

## 📚 Documentation Status

| Category | Status | Last Updated |
|----------|--------|--------------|
| Getting Started | ✅ Complete | 2026-05-23 |
| User Guides | ✅ Complete | 2026-05-23 |
| Community | ✅ Complete | 2026-05-23 |
| Architecture | ✅ Complete | 2026-05-23 |
| Roadmap | ✅ Complete | 2026-05-23 |
| Hunter Docs | ✅ Complete | 2026-06-09 |
| API Reference | 📋 Planned | - |

**Legend:**
- ✅ Complete - Documentation is ready
- 🚧 In Progress - Being written
- 📋 Planned - On the roadmap
- ⚠️ Outdated - Needs update

---

## 🆘 Need Help?

**Can't find what you're looking for?**

1. **Search GitHub Issues** - Someone might have asked already
2. **Join Telegram** - [AiT Developer Community](https://t.me/ait_dev_community)
3. **Ask on Twitter** - Mention [@ait_official](https://x.com/ait_official)
4. **Create an Issue** - [New Issue](https://github.com/Aixxww/AiT/issues/new)

---

## 🤝 Contributing to Documentation

Found an error or want to improve the docs?

1. **Small fixes** - Click "Edit" on GitHub and submit PR
2. **New documentation** - Create an issue first to discuss
3. **Translations** - Follow the repository workflow in [Git Workflow](Git工作流规范.md)

**Documentation Contributors:**
- All documentation follows [Markdown Guide](https://www.markdownguide.org/)
- Use clear, concise language
- Include code examples where helpful
- Add screenshots for UI-related docs

---

**Last Updated:** 2026-06-08
**Maintained by:** [AiT Community](https://github.com/Aixxww/AiT)
