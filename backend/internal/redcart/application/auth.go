package application

import (
	"context"
	"errors"
)

var ErrInvalidToken = errors.New("invalid token")

// TokenPrincipal is the small identity snapshot carried by authentication
// tokens. It deliberately contains no password, phone number, or vendor type.
type TokenPrincipal struct {
	UserID     int64
	Role       string
	Nickname   string
	MerchantID int64
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// TokenManager hides JWT signing and the shared revocation store from the
// application layer. Production uses JWT + Redis; tests may use a local fake.
type TokenManager interface {
	Issue(ctx context.Context, principal TokenPrincipal) (TokenPair, error)
	Authenticate(ctx context.Context, accessToken string) (TokenPrincipal, error)
	Rotate(ctx context.Context, refreshToken string) (TokenPair, TokenPrincipal, error)
	Revoke(ctx context.Context, accessToken string) error
}
