# 交付前验证

在交付结构性改动、文档改动或 agent 路由改动前，执行这个流程。

## 命令

```bash
bash scripts/validate-workspace.sh
```

工作区整理建议：

```bash
rtk bash scripts/git-worktree.sh list
rtk python3 scripts/update-branch-status.py
```

项目本地 Codex hook 会在交付前自动跑 quick handoff gate；需要手动复核时也可以直接执行：

```bash
python3 .codex/hooks/redcart_project_hook.py --mode quick
```

跨后端、前端、AI service、CI 或 PostgreSQL 集成路径的改动，使用完整门禁：

```bash
python3 .codex/hooks/redcart_project_hook.py --mode full
```

## CHANGELOG 约束

- 每个 commit 都应同步更新 `CHANGELOG.md`，记录本次提交带来的用户可见变化、工程行为变化、验证门禁变化或文档资产变化。
- 不把多次提交的变化留到发布前一次性补写；`CHANGELOG.md` 应反映提交粒度的演进记录。
- 如果某个提交确实只有机械整理且没有可记录变化，必须在 PR 描述或交付说明中明确说明不更新 `CHANGELOG.md` 的原因。

## Git 约束

- 当前任务默认应在独立 `git worktree` 中完成；只有非常短的小修正才允许在主工作区直接处理。
- 提交前先确认当前工作区只包含本次要交付的差异；试验性改动、临时调试和异常基准结果应留在独立分支或带说明的 stash 中。
- 性能对照、中间件接入或高风险排查如果出现 QPS、正确性或运行门禁异常，不提交到当前主开发分支。
- 提交信息应能直接说明本次行为变化和原因；不要把多个不相关试验揉成一个 commit。

## 这个检查会验证什么

- 顶层关键入口文件是否存在
- 核心文档是否存在
- 工作流文档是否写明完成证据
- agent 路由文件是否指向规范文档
- `ci/` 目录下的 CI/CD 说明和基础脚本是否齐全
- AI 工作流是否有必要留档，且留档位置能从文档索引找到
- 项目本地 Codex hook 配置、脚本和自测是否存在且可运行

## 完成证据

- 验证命令退出码为 0
- 项目 Codex hook quick gate 通过；高风险或跨层改动已经跑 full gate 或对应 CI 脚本
- 当前任务对应的 worktree 已经过整理，主工作区没有混入这条工作线的临时改动
- 主工作区 `BRANCH_STATUS.local.md` 已更新，能反映当前 worktree 和更改大纲状态
- 当前提交已经同步更新 `CHANGELOG.md`，或已经明确说明本次不更新的原因
- 当前提交差异已经与工作区试验代码隔离，未混入验证异常或未完成的临时改动
- 如果有刻意保留的缺口，已经在最近的相关文档中说明
- AI 参与设计、实现、测试、审查或验收时，已经记录到 `../../AI_WORKFLOW.md`、ADR、风险归档或 PR 描述
- 本地提交信息已经基于暂存差异复核，能说明具体行为变化，未使用 `risk gaps`、`improve backend`、`update files` 这类空泛表述
