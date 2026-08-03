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
	conn       *grpc.ClientConn
	client     pb.AIGenerationServiceClient
	a2uiClient pb.A2UIServiceClient
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
		conn:       conn,
		client:     pb.NewAIGenerationServiceClient(conn),
		a2uiClient: pb.NewA2UIServiceClient(conn),
	}, nil
}

// Close 关闭底层 gRPC 连接。
func (c *Client) Close() error {
	return c.conn.Close()
}

// GenerateSellingPoints 将领域请求转换为 proto 请求，调用 Python AI 服务
// 的 gRPC 接口生成卖点，并将结果映射回领域类型。
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

// GenerateBusinessReview 调用 gRPC 接口生成经营复盘，注意领域侧 GMV
// 与 proto 的 Gmv 字段为同一含义（金额）。
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

// GenerateA2UISurface 调用 gRPC 接口生成 A2UI 界面指令（走独立的
// A2UIService），并返回多行 NDJSON 字符串。
func (c *Client) GenerateA2UISurface(ctx context.Context, req backendai.A2UISurfaceRequest) (*backendai.A2UISurfaceResult, error) {
	resp, err := c.a2uiClient.GenerateA2UISurface(ctx, &pb.GenerateA2UISurfaceRequest{
		SurfaceId:   req.SurfaceID,
		UserIntent:  req.UserIntent,
		ContextJson: req.ContextJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("ai grpc GenerateA2UISurface: %w", err)
	}
	return &backendai.A2UISurfaceResult{
		SurfaceID: resp.SurfaceId,
		A2UIJSON:  resp.A2UiJson,
	}, nil
}
