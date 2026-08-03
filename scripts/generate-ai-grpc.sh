#!/usr/bin/env bash
set -euo pipefail

# 从 api/proto/ai/v1/ai.proto 重新生成 Go 与 Python 的 gRPC 桩代码，并修复 Python 包导入路径。
REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$REPO_ROOT"

mkdir -p backend/internal/ai/gen ai-service/app/ai
# 优先使用仓库内 backend/.bin 下的 protoc 插件，避免依赖全局 Go 环境
export PATH="${REPO_ROOT}/backend/.bin:${PATH}"

if ! command -v protoc-gen-go >/dev/null 2>&1; then
  echo "protoc-gen-go not found in backend/.bin; install with:" >&2
  echo "  GOBIN=\${REPO_ROOT}/backend/.bin go install google.golang.org/protobuf/cmd/protoc-gen-go@latest" >&2
  exit 1
fi
if ! command -v protoc-gen-go-grpc >/dev/null 2>&1; then
  echo "protoc-gen-go-grpc not found in backend/.bin; install with:" >&2
  echo "  GOBIN=\${REPO_ROOT}/backend/.bin go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest" >&2
  exit 1
fi

# 生成 Go 桩：source_relative 保持 proto 目录结构，输出到 backend/internal/ai/gen
echo "Generating Go stubs..."
protoc \
  --proto_path=api/proto \
  --go_out=backend/internal/ai/gen --go_opt=paths=source_relative \
  --go-grpc_out=backend/internal/ai/gen --go-grpc_opt=paths=source_relative \
  ai/v1/ai.proto

# 生成 Python 桩：优先用 ai-service/.venv 的解释器，缺失时回退到系统 python3
echo "Generating Python stubs..."
PYTHON="${REPO_ROOT}/ai-service/.venv/bin/python"
if [[ ! -x "${PYTHON}" ]]; then
  PYTHON="python3"
fi
"${PYTHON}" -m grpc_tools.protoc \
  --proto_path=api/proto \
  --python_out=ai-service/app \
  --grpc_python_out=ai-service/app \
  ai/v1/ai.proto

# Make the generated code importable as app.ai.v1 from the repo root / ai-service directory.
touch ai-service/app/__init__.py
touch ai-service/app/ai/__init__.py
touch ai-service/app/ai/v1/__init__.py

# The Python gRPC plugin hard-codes the import using the proto package name.
# Patch it so it resolves through the app package when running from ai-service/.
sed -i 's/^from ai\.v1 import ai_pb2 as ai_dot_v1_dot_ai__pb2$/from app.ai.v1 import ai_pb2 as ai_dot_v1_dot_ai__pb2/' ai-service/app/ai/v1/ai_pb2_grpc.py

echo "Done."
