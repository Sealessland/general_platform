# Git Worktree 协作

当任务包含并行开发、性能 spike、中间件试验、风险排查或文档/代码需要拆线交付时，默认使用 `git worktree`，不要把多条工作线堆在同一个工作区。

## 目标

- 保持主工作区长期可读、可验证、可随时提交
- 让功能开发、实验分支、文档修订彼此隔离
- 避免 `git stash` 成为默认工作流；stash 只作为短期应急手段

## 默认约定

1. 保留一个干净主工作区，跟随当前稳定开发分支。
2. 每个新任务单独开一个 worktree。
3. 性能 spike、中间件接入、QPS 对照、危险排查必须单独 worktree。
4. 只有验证稳定、范围清楚的差异才能回到可提交分支。
5. 一个 commit 只交付一条工作线，不混功能、实验和无关文档。

## 标准命令

使用仓库提供的 helper：

```bash
rtk bash scripts/git-worktree.sh create feature/my-task
rtk bash scripts/git-worktree.sh list
rtk bash scripts/git-worktree.sh path feature/my-task
rtk bash scripts/git-worktree.sh remove /tmp/agent-native-shop-feature-my-task
```

默认路径规则：

```text
/tmp/<repo-name>-<branch-name-with-slashes-replaced>
```

例如：

```text
/tmp/agent-native-shop-feature-redis-session-spike
```

## 本地状态板

主工作区根目录会自动生成：

```text
BRANCH_STATUS.local.md
```

这个文件由 hook 和 `scripts/git-worktree.sh` 自动刷新，内容包含：

- 当前 worktree 列表
- 每个 worktree 的分支、路径、HEAD、脏/净状态
- 本地 stash
- 由 AI 基于 Git 上下文生成的“更改大纲”

它是本地协作板，不提交进仓库。

## 推荐流程

### 1. 新功能或修复

```bash
rtk bash scripts/git-worktree.sh create feature/orders-cancel-copy
cd /tmp/agent-native-shop-feature-orders-cancel-copy
```

在这个 worktree 内开发、验证、提交。

### 2. 性能或中间件试验

```bash
rtk bash scripts/git-worktree.sh create feature/redis-session-spike
cd /tmp/agent-native-shop-feature-redis-session-spike
```

如果 QPS、正确性或门禁异常：

- 保留 worktree 继续排查
- 不提交到当前主开发分支
- 需要切线时，再开新 worktree 承接稳定结论

### 3. 文档或流程规则修订

```bash
rtk bash scripts/git-worktree.sh create feature/git-workflow-rules
cd /tmp/agent-native-shop-feature-git-workflow-rules
```

文档规则和代码试验分开提交。

## 什么时候可以用 stash

只在这些场景使用：

- 当前 worktree 需要临时让位给更高优先级任务
- 改动还不值得保留成独立分支，但又不能丢
- 需要把脏工作区快速拆成多条 worktree

要求：

- stash 必须命名
- stash 只做短期过渡
- 复原后尽快落到独立 worktree

## 不要这样做

- 不要在长期开发分支的唯一工作区里同时做功能、性能和中间件试验
- 不要用匿名 stash 长期堆现场
- 不要把验证失败或结论不稳定的试验直接提交
- 不要在一个 commit 里混入多个 worktree 的改动
- 不要手工维护 `BRANCH_STATUS.local.md`；它应只由脚本和 hook 自动刷新

## 完成证据

- 当前任务存在独立 worktree，或已经明确说明为什么不需要
- 主工作区保持干净，或脏改动已明确隔离
- 提交前差异只来自当前 worktree 的单一任务线
- `bash scripts/validate-workspace.sh` 通过
