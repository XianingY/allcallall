package usergrpc

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
	"github.com/allcallall/backend/internal/usergrpc/userpb"
)

type Server struct {
	userpb.UnimplementedUserServiceServer
	auth  *auth.Manager
	users *user.Service
}

func NewServer(authManager *auth.Manager, users *user.Service) *Server {
	return &Server{auth: authManager, users: users}
}

func (s *Server) ValidateAccessToken(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	if s == nil || s.auth == nil {
		return nil, status.Error(codes.FailedPrecondition, "user grpc service is not initialized")
	}
	token := strings.TrimSpace(req.GetFields()["access_token"].GetStringValue())
	if token == "" {
		return nil, status.Error(codes.InvalidArgument, "access_token is required")
	}
	claims, err := s.auth.ParseToken(token)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid access token")
	}
	statusValue := models.UserStatusActive
	if s.users != nil {
		u, err := s.users.GetByID(ctx, claims.UserID)
		if err != nil {
			if errors.Is(err, user.ErrNotFound) {
				return nil, status.Error(codes.Unauthenticated, "user not found")
			}
			return nil, status.Error(codes.Internal, "user lookup failed")
		}
		if u.Status == models.UserStatusDeleted {
			return nil, status.Error(codes.PermissionDenied, "user deleted")
		}
		statusValue = u.Status
	}
	return structpb.NewStruct(map[string]any{
		"user_id": float64(claims.UserID),
		"email":   claims.Email,
		"status":  statusValue,
	})
}

func (s *Server) GetUser(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	if s == nil || s.users == nil {
		return nil, status.Error(codes.FailedPrecondition, "user grpc service is not initialized")
	}
	var (
		u   *models.User
		err error
	)
	userID := uint64(req.GetFields()["user_id"].GetNumberValue())
	email := strings.TrimSpace(req.GetFields()["email"].GetStringValue())
	switch {
	case userID > 0:
		u, err = s.users.GetByID(ctx, userID)
	case email != "":
		u, err = s.users.GetByEmail(ctx, email)
	default:
		return nil, status.Error(codes.InvalidArgument, "user_id or email is required")
	}
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Error(codes.Internal, "user lookup failed")
	}
	return structpb.NewStruct(map[string]any{
		"user_id":      float64(u.ID),
		"email":        u.Email,
		"display_name": u.DisplayName,
		"status":       u.Status,
	})
}
