package grpc

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	pb "github.com/example/redcart-copilot/backend/internal/ai/gen/ai/v1"
)

// Client implements backendai.AIProvider by calling the Python AI service over gRPC.
type Client struct {
	conn   *grpc.ClientConn
	client pb.AIGenerationServiceClient
}

// NewClient dials addr and returns a gRPC-backed AI provider.
// The caller is responsible for calling Close.
// If no dial options are supplied, an insecure transport is used.
func NewClient(addr string, opts ...grpc.DialOption) (*Client, error) {
	if len(opts) == 0 {
		opts = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	}
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial ai grpc service: %w", err)
	}
	return &Client{
		conn:   conn,
		client: pb.NewAIGenerationServiceClient(conn),
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) GenerateSellingPoints(ctx context.Context, req backendai.SellingPointRequest) (*backendai.SellingPointResult, error) {
	resp, err := c.client.GenerateSellingPoints(ctx, &pb.GenerateSellingPointsRequest{
		ProductName: req.ProductName,
		Audience:    req.Audience,
		Attributes:  req.Attributes,
		Reviews:     req.Reviews,
	})
	if err != nil {
		return nil, fmt.Errorf("ai grpc GenerateSellingPoints: %w", err)
	}
	return &backendai.SellingPointResult{Points: resp.Points}, nil
}

func (c *Client) GenerateBusinessReview(ctx context.Context, req backendai.BusinessReviewRequest) (*backendai.BusinessReviewResult, error) {
	resp, err := c.client.GenerateBusinessReview(ctx, &pb.GenerateBusinessReviewRequest{
		WindowDays: int32(req.WindowDays),
		Gmv:        req.GMV,
		RefundRate: req.RefundRate,
	})
	if err != nil {
		return nil, fmt.Errorf("ai grpc GenerateBusinessReview: %w", err)
	}
	return &backendai.BusinessReviewResult{
		Diagnosis: resp.Diagnosis,
		NextSteps: resp.NextSteps,
	}, nil
}
