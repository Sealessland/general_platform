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
    value = os.environ.get(key)
    return value if value else fallback


class AIGenerationServicer(ai_pb2_grpc.AIGenerationServiceServicer):
    def __init__(self, provider: MockAIProvider | None = None) -> None:
        self.provider = provider or MockAIProvider()

    def GenerateSellingPoints(
        self,
        request: ai_pb2.GenerateSellingPointsRequest,
        context: grpc.ServicerContext,
    ) -> ai_pb2.GenerateSellingPointsResponse:
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
        return ai_pb2.GenerateBusinessReviewResponse(
            diagnosis=result["diagnosis"],
            next_steps=result["next_steps"],
        )


class A2UIServicer(ai_pb2_grpc.A2UIServiceServicer):
    def __init__(self, provider: MockAIProvider | None = None) -> None:
        self.provider = provider or MockAIProvider()

    def GenerateA2UISurface(
        self,
        request: ai_pb2.GenerateA2UISurfaceRequest,
        context: grpc.ServicerContext,
    ) -> ai_pb2.GenerateA2UISurfaceResponse:
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
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    ai_pb2_grpc.add_AIGenerationServiceServicer_to_server(
        AIGenerationServicer(provider), server
    )
    ai_pb2_grpc.add_A2UIServiceServicer_to_server(
        A2UIServicer(provider), server
    )
    return server


def serve() -> None:
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
