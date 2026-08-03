# 基础镜像与工作目录；启动命令以 -m app.grpc_server 运行，依赖 /app 在 sys.path 中。
FROM python:3.11-slim
WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

# 使 app 包可从任意目录导入（app.grpc_server 顶层 import 依赖此路径）。
ENV PYTHONPATH=/app
ENV AI_GRPC_HOST=0.0.0.0
ENV AI_GRPC_PORT=50051

EXPOSE 50051

CMD ["python", "-m", "app.grpc_server"]
