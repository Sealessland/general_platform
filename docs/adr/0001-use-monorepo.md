# ADR 0001：采用 Monorepo

## 状态

已接受

## 背景

项目同时包含后端、前端、AI 服务、接口契约、迁移和工程协作资产。面试场景下需要让评审快速看到全链路。

## 决策

采用 monorepo，统一维护 `backend/`、`frontend/`、`ai-service/`、`docs/`、`.github/` 与 `scripts/`。

## 影响

- 跨模块契约可以放在同一仓库下统一查看
- CI 可以在一套工作流里检查后端、前端、AI 服务、OpenAPI 与 Docker
- 模块边界必须靠文档明确，否则容易耦合
