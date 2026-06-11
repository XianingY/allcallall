package usergrpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/usergrpc/userpb"
)

type ClientAuthenticator struct {
	client  userpb.UserServiceClient
	timeout time.Duration
}

func NewClientAuthenticator(client userpb.UserServiceClient, timeout time.Duration) *ClientAuthenticator {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &ClientAuthenticator{client: client, timeout: timeout}
}

func DialClientAuthenticator(ctx context.Context, addr string, timeout time.Duration) (*ClientAuthenticator, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, nil, errors.New("user service grpc addr is required")
	}
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return NewClientAuthenticator(userpb.NewUserServiceClient(conn), timeout), conn.Close, nil
}

func (c *ClientAuthenticator) ValidateAccessToken(ctx context.Context, token string) (*auth.Claims, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("user grpc client is not initialized")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("access token is required")
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := structpb.NewStruct(map[string]any{"access_token": token})
	if err != nil {
		return nil, err
	}
	resp, err := c.client.ValidateAccessToken(callCtx, req)
	if err != nil {
		return nil, err
	}
	fields := resp.GetFields()
	userID := uint64(fields["user_id"].GetNumberValue())
	email := strings.TrimSpace(fields["email"].GetStringValue())
	if userID == 0 || email == "" {
		return nil, fmt.Errorf("invalid user grpc auth response")
	}
	return &auth.Claims{
		UserID:    userID,
		Email:     email,
		TokenType: auth.TokenTypeAccess,
	}, nil
}
