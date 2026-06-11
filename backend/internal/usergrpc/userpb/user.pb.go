// Package userpb contains a small hand-written gRPC binding for the user service.
// The canonical contract lives in backend/proto/user/v1/user.proto.
package userpb

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

const UserServiceName = "allcallall.user.v1.UserService"

type UserServiceClient interface {
	ValidateAccessToken(ctx context.Context, in *structpb.Struct, opts ...grpc.CallOption) (*structpb.Struct, error)
	GetUser(ctx context.Context, in *structpb.Struct, opts ...grpc.CallOption) (*structpb.Struct, error)
}

type userServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewUserServiceClient(cc grpc.ClientConnInterface) UserServiceClient {
	return &userServiceClient{cc: cc}
}

func (c *userServiceClient) ValidateAccessToken(ctx context.Context, in *structpb.Struct, opts ...grpc.CallOption) (*structpb.Struct, error) {
	out := new(structpb.Struct)
	err := c.cc.Invoke(ctx, "/"+UserServiceName+"/ValidateAccessToken", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *userServiceClient) GetUser(ctx context.Context, in *structpb.Struct, opts ...grpc.CallOption) (*structpb.Struct, error) {
	out := new(structpb.Struct)
	err := c.cc.Invoke(ctx, "/"+UserServiceName+"/GetUser", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type UserServiceServer interface {
	ValidateAccessToken(context.Context, *structpb.Struct) (*structpb.Struct, error)
	GetUser(context.Context, *structpb.Struct) (*structpb.Struct, error)
}

type UnimplementedUserServiceServer struct{}

func (UnimplementedUserServiceServer) ValidateAccessToken(context.Context, *structpb.Struct) (*structpb.Struct, error) {
	return nil, status.Error(codes.Unimplemented, "method ValidateAccessToken not implemented")
}

func (UnimplementedUserServiceServer) GetUser(context.Context, *structpb.Struct) (*structpb.Struct, error) {
	return nil, status.Error(codes.Unimplemented, "method GetUser not implemented")
}

func RegisterUserServiceServer(s grpc.ServiceRegistrar, srv UserServiceServer) {
	s.RegisterService(&UserService_ServiceDesc, srv)
}

func _UserService_ValidateAccessToken_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(structpb.Struct)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(UserServiceServer).ValidateAccessToken(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/" + UserServiceName + "/ValidateAccessToken",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(UserServiceServer).ValidateAccessToken(ctx, req.(*structpb.Struct))
	}
	return interceptor(ctx, in, info, handler)
}

func _UserService_GetUser_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(structpb.Struct)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(UserServiceServer).GetUser(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/" + UserServiceName + "/GetUser",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(UserServiceServer).GetUser(ctx, req.(*structpb.Struct))
	}
	return interceptor(ctx, in, info, handler)
}

var UserService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: UserServiceName,
	HandlerType: (*UserServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ValidateAccessToken",
			Handler:    _UserService_ValidateAccessToken_Handler,
		},
		{
			MethodName: "GetUser",
			Handler:    _UserService_GetUser_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "proto/user/v1/user.proto",
}
