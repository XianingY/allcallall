package usergrpc

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/usergrpc/userpb"
)

func TestClientAuthenticatorValidatesTokenOverGRPC(t *testing.T) {
	manager, err := auth.NewManager(auth.Config{Secret: "grpc-secret", Issuer: "test"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := manager.GenerateAccessToken(42, "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	userpb.RegisterUserServiceServer(server, NewServer(manager, nil))
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithInsecure(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	validator := NewClientAuthenticator(userpb.NewUserServiceClient(conn), time.Second)
	claims, err := validator.ValidateAccessToken(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 42 || claims.Email != "owner@example.com" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}
