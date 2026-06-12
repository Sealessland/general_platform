FROM python:3.11-slim
WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

ENV PYTHONPATH=/app
ENV AI_GRPC_HOST=0.0.0.0
ENV AI_GRPC_PORT=50051

EXPOSE 50051

CMD ["python", "-m", "app.grpc_server"]
