#!/usr/bin/env bash
# AI service CI 主入口：语法编译检查、单元测试、prompt 契约检查。
set -euo pipefail

# 定位仓库根目录并切换到 ai-service，保证后续命令以服务根目录为基准
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR/ai-service"

# 依赖安装：测试会导入 grpcio 生成代码，CI 与本地门禁都从服务内 requirements 固化依赖。
python -m venv .venv
".venv/bin/python" -m pip install -r requirements.txt

# 编译检查：不实际执行，仅验证 app 与 tests 的语法
".venv/bin/python" -m compileall app tests
# 单元测试：自动发现 tests 目录下的用例
".venv/bin/python" -m unittest discover -s tests -v
# prompt 检查：校验 prompts/ 中的模板是否包含约定占位符
".venv/bin/python" app/check_prompts.py
