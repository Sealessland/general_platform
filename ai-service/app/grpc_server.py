"""AI 服务的 gRPC 入口：定义各 Servicer 及 server 构建逻辑，负责将 gRPC 请求适配到 provider。

业务校验失败（provider 抛 ValueError）统一映射为 INVALID_ARGUMENT 状态码；
服务名/端口通过环境变量 AI_GRPC_HOST / AI_GRPC_PORT 配置，默认 0.0.0.0:50051。
"""

import logging
import os
from concurrent import futures

import grpc

from app.ai.v1 import ai_pb2
from app.ai.v1 import ai_pb2_grpc
from app.provider import A2UISurfaceRequest, BusinessReviewRequest, MockAIProvider, SellingPointRequest

_DEFAULT_PORT = "50051"
_DEFAULT_HOST = "0.0.0.0"


def _env(key: str, fallback: str) -> str:
    """读取环境变量，未设置或为空时返回默认值。"""
    value = os.environ.get(key)
    return value if value else fallback


class AIGenerationServicer(ai_pb2_grpc.AIGenerationServiceServicer):
    """AIGenerationService 的实现：商品卖点生成与经营复盘。"""

    def __init__(self, provider: MockAIProvider | None = None) -> None:
        """初始化 Servicer，未注入 provider 时使用默认的 MockAIProvider。"""
        self.provider = provider or MockAIProvider()

    def GenerateSellingPoints(
        self,
        request: ai_pb2.GenerateSellingPointsRequest,
        context: grpc.ServicerContext,
    ) -> ai_pb2.GenerateSellingPointsResponse:
        """生成商品卖点：将 protobuf 请求转为领域模型，校验失败时映射为 INVALID_ARGUMENT。"""
        # 将 protobuf 请求转换为内部领域模型；参数校验失败时返回 INVALID_ARGUMENT。
        try:
            points = self.provider.generate_selling_points(
                SellingPointRequest(
                    product_name=request.product_name,
                    audience=request.audience,
                )
            )
        except ValueError as exc:
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(str(exc))
            return ai_pb2.GenerateSellingPointsResponse()
        return ai_pb2.GenerateSellingPointsResponse(points=points)

    def GenerateBusinessReview(
        self,
        request: ai_pb2.GenerateBusinessReviewRequest,
        context: grpc.ServicerContext,
    ) -> ai_pb2.GenerateBusinessReviewResponse:
        """生成经营复盘：校验入参，将诊断结论与下一步动作透出给调用方。"""
        try:
            result = self.provider.generate_business_review(
                BusinessReviewRequest(
                    window_days=request.window_days,
                    gmv=request.gmv,
                    refund_rate=request.refund_rate,
                )
            )
        except ValueError as exc:
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(str(exc))
            return ai_pb2.GenerateBusinessReviewResponse()
        # 复盘结果字段较多，仅向调用方透出诊断结论与下一步动作。
        return ai_pb2.GenerateBusinessReviewResponse(
            diagnosis=result["diagnosis"],
            next_steps=result["next_steps"],
        )


class A2UIServicer(ai_pb2_grpc.A2UIServiceServicer):
    """A2UIService 的实现：按用户意图与上下文生成可渲染的 A2UI 界面 JSON。"""

    def __init__(self, provider: MockAIProvider | None = None) -> None:
        """初始化 Servicer，未注入 provider 时使用默认的 MockAIProvider。"""
        self.provider = provider or MockAIProvider()

    def GenerateA2UISurface(
        self,
        request: ai_pb2.GenerateA2UISurfaceRequest,
        context: grpc.ServicerContext,
    ) -> ai_pb2.GenerateA2UISurfaceResponse:
        """生成 A2UI 界面：将意图与上下文交给 provider，校验失败时返回 INVALID_ARGUMENT。"""
        try:
            result = self.provider.generate_a2ui_surface(
                A2UISurfaceRequest(
                    surface_id=request.surface_id,
                    user_intent=request.user_intent,
                    context_json=request.context_json,
                )
            )
        except ValueError as exc:
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(str(exc))
            return ai_pb2.GenerateA2UISurfaceResponse()
        return ai_pb2.GenerateA2UISurfaceResponse(
            surface_id=result["surface_id"],
            a2ui_json=result["a2ui_json"],
        )


def build_server(provider: MockAIProvider | None = None) -> grpc.Server:
    """构建并注册全部服务的 gRPC server；可通过 provider 参数注入替换实现。"""
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    ai_pb2_grpc.add_AIGenerationServiceServicer_to_server(
        AIGenerationServicer(provider), server
    )
    ai_pb2_grpc.add_A2UIServiceServicer_to_server(
        A2UIServicer(provider), server
    )
    return server


def serve() -> None:
    """从环境变量读取监听地址并启动服务，阻塞直至进程终止。"""
    logging.basicConfig(level=logging.INFO)
    host = _env("AI_GRPC_HOST", _DEFAULT_HOST)
    port = _env("AI_GRPC_PORT", _DEFAULT_PORT)
    address = f"{host}:{port}"

    server = build_server()
    server.add_insecure_port(address)
    server.start()
    logging.info("AI gRPC server listening on %s", address)
    server.wait_for_termination()


if __name__ == "__main__":
    serve()
