# 文档索引

这是仓库的文档入口，优先从这里定位信息，而不是在多个 agent 文件之间来回查找。

## 优先阅读

- 项目总览：`../README.md`
- 架构边界：`architecture.md`
- 完成检查清单：`checklists/agent-native-completion.md`

## 工作流文档

- 新增或修改功能：`workflows/add-feature.md`
- 新增集成或适配器：`workflows/add-integration.md`
- 排查故障：`workflows/debug.md`
- 交付前验证：`workflows/validate.md`

## Agent 路由

- 仓库级说明：`../AGENTS.md`
- 本地 agent 注册：`../.agents/README.md`
- Codex 技能入口：`../.codex/skills/agent-native-shop/SKILL.md`

面向 agent 的文件只负责路由，不重复维护业务事实；稳定事实统一落到 `docs/`。
