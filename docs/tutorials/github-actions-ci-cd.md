# 初学者教程：GitHub Actions CI/CD 与性能数据

仓库把“检查代码”“测性能”“发布镜像”分成三条流水线，便于初学者定位失败原因。

## CI：每个 PR 的质量门禁

`.github/workflows/ci.yml` 在面向 `main` 的 PR 和 `main` push 上运行，编排以下可复用 workflow：

- Workspace/OpenAPI：检查结构、文档和接口契约；
- Backend：Go 格式、vet、测试、覆盖率、PostgreSQL/Redis/Kafka 集成与 benchmark；
- Frontend、AI Service：各自测试与构建；
- Security、Docker：密钥扫描和三个镜像的可构建性。

本地复现最小门禁：

```bash
rtk bash scripts/validate-workspace.sh
rtk bash scripts/check-openapi.sh
```

后端完整门禁需要真实 PostgreSQL、Redis、Kafka，GitHub Runner 会自动创建这些 service containers。

## Benchmark：读取 Runner 性能

`.github/workflows/benchmark.yml` 支持手动运行，也会在 `main` push 后运行。GitHub 页面操作路径是 Actions → Benchmark → Run workflow。

CLI 触发和查看：

```bash
gh workflow run benchmark.yml --ref your-branch
gh run list --workflow benchmark.yml --branch your-branch --limit 1
gh run watch RUN_ID --exit-status
```

运行结果同时出现在 Job Summary 和 `benchmark-results-RUN_ID` artifact。表格包含 QPS、`ns/op`、`B/op` 与 `allocs/op`。手动分支运行不会改 README；只有 `main` push 才自动提交最新基线。

性能数据只能比较同类 Runner、相同依赖版本和相同 `-benchtime`。共享的 `ubuntu-latest` 有噪声，适合发现数量级退化，不应当作生产容量承诺。

## CD：发布而不是假装部署

`.github/workflows/release.yml` 在推送 `vX.Y.Z` tag 或手动触发时，把 backend、frontend、ai-service 构建并发布到 GHCR：

```text
ghcr.io/<owner>/redcart-backend
ghcr.io/<owner>/redcart-frontend
ghcr.io/<owner>/redcart-ai-service
```

workflow 使用最小的 `contents: read`、`packages: write` 权限，生成 SHA/语义版本标签、SBOM 和 provenance，并使用 GitHub Actions 构建缓存。

当前仓库没有指定 Kubernetes、云主机或生产凭据，因此 CD 的明确边界是“生成可部署的不可变镜像”。未来选定部署平台后，应新增独立 deploy job/environment、审批规则、健康检查和回滚步骤，而不是把服务器密钥写进仓库。

## 常见失败定位

- service container 不健康：先看 PostgreSQL、Redis、Kafka 初始化日志；
- 覆盖率失败：下载 `backend-ci-artifacts`，查看 `backend-coverage-summary.txt`；
- benchmark 缺项：下载 benchmark artifact，确认测试未被环境变量跳过；
- 镜像发布 403：检查仓库 Actions 的 package write 权限与组织策略。
