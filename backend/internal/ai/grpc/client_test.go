package grpc

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	pb "github.com/example/redcart-copilot/backend/internal/ai/gen/ai/v1"
)

type testAIServer struct {
	pb.UnimplementedAIGenerationServiceServer
	pb.UnimplementedA2UIServiceServer
}

// GenerateSellingPoints 测试服务端实现：返回基于商品名的固定卖点列表。
func (s *testAIServer) GenerateSellingPoints(ctx context.Context, req *pb.GenerateSellingPointsRequest) (*pb.GenerateSellingPointsResponse, error) {
	return &pb.GenerateSellingPointsResponse{
		Points: []string{req.ProductName + " point", "another point"},
	}, nil
}

// GenerateBusinessReview 测试服务端实现：返回固定诊断文案与行动项。
func (s *testAIServer) GenerateBusinessReview(ctx context.Context, req *pb.GenerateBusinessReviewRequest) (*pb.GenerateBusinessReviewResponse, error) {
	return &pb.GenerateBusinessReviewResponse{
		Diagnosis: "test diagnosis",
		NextSteps: []string{"step one", "step two"},
	}, nil
}

// GenerateA2UISurface 测试服务端实现：回显 surfaceId 并返回固定 A2UI 指令。
func (s *testAIServer) GenerateA2UISurface(ctx context.Context, req *pb.GenerateA2UISurfaceRequest) (*pb.GenerateA2UISurfaceResponse, error) {
	return &pb.GenerateA2UISurfaceResponse{
		SurfaceId: req.SurfaceId,
		A2UiJson:  `{"version":"v0.9","createSurface":{"surfaceId":"` + req.SurfaceId + `","catalogId":"test"}}`,
	}, nil
}

// newTestClient 启动内存 bufconn 上的测试 gRPC 服务并返回客户端与清理函数。
func newTestClient(t *testing.T) (*Client, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	pb.RegisterAIGenerationServiceServer(srv, &testAIServer{})
	pb.RegisterA2UIServiceServer(srv, &testAIServer{})

	// 在后台 goroutine 中对外提供 gRPC 服务，服务错误仅记录日志。
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("test server serve error: %v", err)
		}
	}()

	// dialer 把 gRPC 拨号重定向到 bufconn，避免真实网络依赖。
	dialer := func(context.Context, string) (net.Conn, error) { return lis.DialContext(context.Background()) }
	client, err := NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	// cleanup 关闭客户端并停止测试服务。
	cleanup := func() {
		_ = client.Close()
		srv.Stop()
	}
	return client, cleanup
}

// TestClientGenerateSellingPoints 验证卖点生成经 gRPC 往返返回期望结果。
func TestClientGenerateSellingPoints(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()

	result, err := client.GenerateSellingPoints(context.Background(), backendai.SellingPointRequest{
		ProductName: "Widget",
		Audience:    "makers",
		Attributes:  []string{"small"},
		Reviews:     []string{"nice"},
	})
	if err != nil {
		t.Fatalf("GenerateSellingPoints: %v", err)
	}
	if len(result.Points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(result.Points))
	}
	if result.Points[0] != "Widget point" {
		t.Fatalf("unexpected first point: %s", result.Points[0])
	}
}

// TestClientGenerateBusinessReview 验证经营复盘经 gRPC 往返返回期望结果。
func TestClientGenerateBusinessReview(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()

	result, err := client.GenerateBusinessReview(context.Background(), backendai.BusinessReviewRequest{
		WindowDays: 7,
		GMV:        12345,
		RefundRate: 0.02,
	})
	if err != nil {
		t.Fatalf("GenerateBusinessReview: %v", err)
	}
	if result.Diagnosis != "test diagnosis" {
		t.Fatalf("unexpected diagnosis: %s", result.Diagnosis)
	}
	if len(result.NextSteps) != 2 {
		t.Fatalf("expected 2 next steps, got %d", len(result.NextSteps))
	}
}

// TestClientGenerateA2UISurface 验证 A2UI 界面生成经 gRPC 往返返回期望结果。
func TestClientGenerateA2UISurface(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()

	result, err := client.GenerateA2UISurface(context.Background(), backendai.A2UISurfaceRequest{
		SurfaceID:   "test_surface",
		UserIntent:  "show welcome",
		ContextJSON: "{}",
	})
	if err != nil {
		t.Fatalf("GenerateA2UISurface: %v", err)
	}
	if result.SurfaceID != "test_surface" {
		t.Fatalf("unexpected surface id: %s", result.SurfaceID)
	}
	if result.A2UIJSON == "" {
		t.Fatalf("expected non-empty a2ui_json")
	}
}
