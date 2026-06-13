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

func (s *testAIServer) GenerateSellingPoints(ctx context.Context, req *pb.GenerateSellingPointsRequest) (*pb.GenerateSellingPointsResponse, error) {
	return &pb.GenerateSellingPointsResponse{
		Points: []string{req.ProductName + " point", "another point"},
	}, nil
}

func (s *testAIServer) GenerateBusinessReview(ctx context.Context, req *pb.GenerateBusinessReviewRequest) (*pb.GenerateBusinessReviewResponse, error) {
	return &pb.GenerateBusinessReviewResponse{
		Diagnosis: "test diagnosis",
		NextSteps: []string{"step one", "step two"},
	}, nil
}

func (s *testAIServer) GenerateA2UISurface(ctx context.Context, req *pb.GenerateA2UISurfaceRequest) (*pb.GenerateA2UISurfaceResponse, error) {
	return &pb.GenerateA2UISurfaceResponse{
		SurfaceId: req.SurfaceId,
		A2UiJson:  `{"version":"v0.9","createSurface":{"surfaceId":"` + req.SurfaceId + `","catalogId":"test"}}`,
	}, nil
}

func newTestClient(t *testing.T) (*Client, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	pb.RegisterAIGenerationServiceServer(srv, &testAIServer{})
	pb.RegisterA2UIServiceServer(srv, &testAIServer{})

	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("test server serve error: %v", err)
		}
	}()

	dialer := func(context.Context, string) (net.Conn, error) { return lis.DialContext(context.Background()) }
	client, err := NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	cleanup := func() {
		_ = client.Close()
		srv.Stop()
	}
	return client, cleanup
}

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
