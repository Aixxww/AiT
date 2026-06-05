# AIT 文档整理报告

> 生成时间：2026-06-05
> 范围：项目 Markdown/TXT/RST/ADOC 文档、文档文件名、根目录报告归档

## 整理结果

1. 全仓库检索旧品牌关键字的常见大小写组合，未发现残留。
2. 文档文件名检索未发现旧品牌命名残留。
3. 根目录历史交易分析报告已归档到 `reports/`：
   - `reports/hunter_live_analysis_20260603.md`
   - `reports/hunter_round2_analysis_20260603.md`
4. 新增猎手 v7 实盘复盘与提示词优化报告：
   - `reports/hunter-v7-live-prompt-review-20260605.md`
5. 已替换 `web/public/icons/ait.svg`，移除旧品牌图形路径，改为 AIT 专属标识。

## 当前文档结构

- 项目入口文档保留在根目录：`README.md`、`README.ja.md`、`CHANGELOG.md`、`CHANGELOG.zh-CN.md`、`SECURITY.md`、`DEPLOY.md`。
- 产品、架构、使用指南保留在 `docs/`。
- 实盘分析、品牌替换检查、策略复盘报告统一保留在 `reports/`。
- 前端公共图标保留在 `web/public/icons/`，其中 `ait.svg` 为 AIT 当前品牌标识。

## 验证命令

```bash
rg -n "<旧品牌关键字大小写组合>" .
git ls-files "<旧品牌文件名大小写组合>"
find . -maxdepth 2 -type f -name "*.md" -print | sort
```

## 结论

AIT 文档品牌命名已完成清理，报告类文档已统一归档，当前文档结构可提交推送。
