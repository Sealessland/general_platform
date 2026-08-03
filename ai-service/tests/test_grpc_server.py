import unittest

import grpc

from app.ai.v1 import ai_pb2
from app.ai.v1 import ai_pb2_grpc
from app.grpc_server import build_server


class TestAIGenerationGRPCServer(unittest.TestCase):
    """通过真实 gRPC server（随机端口）做端到端验证：正常路径 + 参数校验映射。"""

    @classmethod
    def setUpClass(cls) -> None:
        """类级准备：用随机端口启动真实 gRPC server，供各用例调用。"""
        # 用临时端口启动 server，避免与本地已运行实例冲突。
        cls.server = build_server()
        cls.port = cls.server.add_insecure_port("[::]:0")
        cls.server.start()
        cls.channel = grpc.insecure_channel(f"localhost:{cls.port}")
        cls.stub = ai_pb2_grpc.AIGenerationServiceStub(cls.channel)
        cls.a2ui_stub = ai_pb2_grpc.A2UIServiceStub(cls.channel)

    @classmethod
    def tearDownClass(cls) -> None:
        """类级清理：关闭 channel 并停止 server。"""
        cls.channel.close()
        cls.server.stop(None)

    def test_generate_selling_points(self) -> None:
        """正常路径：卖点结果应包含人群定向文案。"""
        request = ai_pb2.GenerateSellingPointsRequest(
            product_name="Travel Makeup Organizer",
            audience="dorm users",
            attributes=["portable"],
            reviews=["great"],
        )
        response = self.stub.GenerateSellingPoints(request)
        self.assertIn("Travel Makeup Organizer for dorm users", response.points)

    def test_generate_selling_points_requires_product_name(self) -> None:
        """缺商品名应返回 INVALID_ARGUMENT。"""
        request = ai_pb2.GenerateSellingPointsRequest(product_name="")
        with self.assertRaises(grpc.RpcError) as cm:
            self.stub.GenerateSellingPoints(request)
        self.assertEqual(cm.exception.code(), grpc.StatusCode.INVALID_ARGUMENT)

    def test_generate_business_review(self) -> None:
        """正常路径：复盘应返回诊断结论与至少一条下一步动作。"""
        request = ai_pb2.GenerateBusinessReviewRequest(
            window_days=7,
            gmv=10000,
            refund_rate=0.05,
        )
        response = self.stub.GenerateBusinessReview(request)
        self.assertTrue(response.diagnosis)
        self.assertGreaterEqual(len(response.next_steps), 1)

    def test_generate_business_review_requires_positive_window(self) -> None:
        """窗口天数非正应返回 INVALID_ARGUMENT。"""
        request = ai_pb2.GenerateBusinessReviewRequest(window_days=0)
        with self.assertRaises(grpc.RpcError) as cm:
            self.stub.GenerateBusinessReview(request)
        self.assertEqual(cm.exception.code(), grpc.StatusCode.INVALID_ARGUMENT)

    def test_generate_a2ui_surface(self) -> None:
        """正常路径：A2UI 响应应包含 createSurface 与 updateComponents。"""
        request = ai_pb2.GenerateA2UISurfaceRequest(
            surface_id="test_surface",
            user_intent="show welcome",
            context_json="{}",
        )
        response = self.a2ui_stub.GenerateA2UISurface(request)
        self.assertEqual(response.surface_id, "test_surface")
        self.assertIn("createSurface", response.a2ui_json)
        self.assertIn("updateComponents", response.a2ui_json)

    def test_generate_a2ui_surface_requires_surface_id(self) -> None:
        """缺 surface_id 应返回 INVALID_ARGUMENT。"""
        request = ai_pb2.GenerateA2UISurfaceRequest(
            surface_id="",
            user_intent="show welcome",
        )
        with self.assertRaises(grpc.RpcError) as cm:
            self.a2ui_stub.GenerateA2UISurface(request)
        self.assertEqual(cm.exception.code(), grpc.StatusCode.INVALID_ARGUMENT)


if __name__ == "__main__":
    unittest.main()
