# 贡献说明

这个仓库用于展示内容电商全链路与工程化能力，提交方式按真实团队协作来约束。

## 开发前置

开始实现前先创建或选定一个任务，任务描述至少应包含：

- 背景
- 用户故事
- 验收标准
- 技术方案
- AI 使用记录

## 分支规范

- `main`：稳定分支
- `feature/*`：新功能
- `fix/*`：缺陷修复
- `refactor/*`：不改变外部行为的重构
- `docs/*`：文档改动
- `test/*`：测试补充

## 提交规范

使用 Conventional Commits，例如：

```text
feat: add order state machine
fix: prevent duplicate order submission
refactor: split order usecase into domain service
test: cover refund inventory restoration
docs: translate project documents to chinese
chore: update local validation script
```

提交信息必须描述这次提交实际改变了什么行为或公共面，不能只写抽象结论。尤其修复类提交要写清楚被修的风险类型、受影响模块或可观察结果。

不合格示例：

```text
fix: close risk gaps
fix: improve backend
chore: update files
```

合格示例：

```text
fix: reject get requests for order state changes
fix: isolate ai task access by merchant ownership
fix: reserve postgres inventory atomically on order create
docs: record audited risk fixes and validation evidence
```

提交前先看暂存差异，再写信息：

```bash
git diff --cached --stat
git diff --cached --name-only
```

如果一个提交同时改了多个风险点，提交信息仍要点名主要行为，例如：

```text
fix: enforce method gates, ai ownership, and inventory locks
```

## 提交前检查

- 功能改动是否有对应任务
- API 变更是否更新 `docs/api/openapi.yaml`
- 数据结构变更是否更新 `backend/migrations/`
- AI 参与记录是否补充到 `AI_WORKFLOW.md`
- 相关测试是否补齐
- 以下命令是否通过：

```bash
bash scripts/validate-workspace.sh
bash scripts/check-openapi.sh
```

## Pull Request 要求

- 链接对应任务
- 改动范围聚焦
- 说明验证结果
- 如存在测试缺口，明确写出原因
- 如使用 AI，说明 AI 用于哪一段工作、输出有哪些被采纳或被修正
