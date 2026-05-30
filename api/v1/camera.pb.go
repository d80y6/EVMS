package damv1

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// NOTE: This file is a manual representation of the generated gRPC code
// because protoc is not available in the current environment.
// In a full development environment, this would be generated from camera.proto.

type Camera struct {
	Id            string
	SiteId        string
	Name          string
	Description   string
	ConnectionUrl string
	SubstreamUrl  string
	Status        string
	CreatedAt     *timestamppb.Timestamp
}

type ListCamerasRequest struct {
	SiteId string
}

type ListCamerasResponse struct {
	Cameras []*Camera
}

type CreateCameraRequest struct {
	SiteId        string
	Name          string
	ConnectionUrl string
	SubstreamUrl  string
}

type GetCameraRequest struct {
	Id string
}

type UpdateCameraRequest struct {
	Id            string
	Name          string
	Description   string
	ConnectionUrl string
	SubstreamUrl  string
}

type DeleteCameraRequest struct {
	Id string
}

type DeleteCameraResponse struct {
	Success bool
}

type StreamStatusRequest struct {
	CameraId string
}

type StreamStatusResponse struct {
	Status  string
	Bitrate float64
	Fps     float64
}

type CameraServiceServer interface {
	ListCameras(context.Context, *ListCamerasRequest) (*ListCamerasResponse, error)
	GetCamera(context.Context, *GetCameraRequest) (*Camera, error)
	CreateCamera(context.Context, *CreateCameraRequest) (*Camera, error)
	UpdateCamera(context.Context, *UpdateCameraRequest) (*Camera, error)
	DeleteCamera(context.Context, *DeleteCameraRequest) (*DeleteCameraResponse, error)
	StreamStatus(context.Context, *StreamStatusRequest) (*StreamStatusResponse, error)
	mustEmbedUnimplementedCameraServiceServer()
}

type UnimplementedCameraServiceServer struct{}

func (UnimplementedCameraServiceServer) ListCameras(context.Context, *ListCamerasRequest) (*ListCamerasResponse, error) {
	return nil, nil
}
func (UnimplementedCameraServiceServer) GetCamera(context.Context, *GetCameraRequest) (*Camera, error) {
	return nil, nil
}
func (UnimplementedCameraServiceServer) CreateCamera(context.Context, *CreateCameraRequest) (*Camera, error) {
	return nil, nil
}
func (UnimplementedCameraServiceServer) UpdateCamera(context.Context, *UpdateCameraRequest) (*Camera, error) {
	return nil, nil
}
func (UnimplementedCameraServiceServer) DeleteCamera(context.Context, *DeleteCameraRequest) (*DeleteCameraResponse, error) {
	return nil, nil
}
func (UnimplementedCameraServiceServer) StreamStatus(context.Context, *StreamStatusRequest) (*StreamStatusResponse, error) {
	return nil, nil
}
func (UnimplementedCameraServiceServer) mustEmbedUnimplementedCameraServiceServer() {}

func RegisterCameraServiceServer(s grpc.ServiceRegistrar, srv CameraServiceServer) {
	s.RegisterService(&CameraService_ServiceDesc, srv)
}

func _CameraService_ListCameras_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListCamerasRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CameraServiceServer).ListCameras(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/dam_v1.CameraService/ListCameras",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CameraServiceServer).ListCameras(ctx, req.(*ListCamerasRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _CameraService_GetCamera_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetCameraRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CameraServiceServer).GetCamera(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/dam_v1.CameraService/GetCamera",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CameraServiceServer).GetCamera(ctx, req.(*GetCameraRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _CameraService_CreateCamera_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateCameraRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CameraServiceServer).CreateCamera(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/dam_v1.CameraService/CreateCamera",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CameraServiceServer).CreateCamera(ctx, req.(*CreateCameraRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _CameraService_UpdateCamera_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateCameraRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CameraServiceServer).UpdateCamera(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/dam_v1.CameraService/UpdateCamera",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CameraServiceServer).UpdateCamera(ctx, req.(*UpdateCameraRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _CameraService_DeleteCamera_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteCameraRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CameraServiceServer).DeleteCamera(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/dam_v1.CameraService/DeleteCamera",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CameraServiceServer).DeleteCamera(ctx, req.(*DeleteCameraRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _CameraService_StreamStatus_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StreamStatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CameraServiceServer).StreamStatus(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/dam_v1.CameraService/StreamStatus",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CameraServiceServer).StreamStatus(ctx, req.(*StreamStatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var CameraService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "dam_v1.CameraService",
	HandlerType: (*CameraServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ListCameras",
			Handler:    _CameraService_ListCameras_Handler,
		},
		{
			MethodName: "GetCamera",
			Handler:    _CameraService_GetCamera_Handler,
		},
		{
			MethodName: "CreateCamera",
			Handler:    _CameraService_CreateCamera_Handler,
		},
		{
			MethodName: "UpdateCamera",
			Handler:    _CameraService_UpdateCamera_Handler,
		},
		{
			MethodName: "DeleteCamera",
			Handler:    _CameraService_DeleteCamera_Handler,
		},
		{
			MethodName: "StreamStatus",
			Handler:    _CameraService_StreamStatus_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "api/proto/v1/camera.proto",
}

// Client types for gRPC gateway access

type CameraServiceClient interface {
	ListCameras(ctx context.Context, in *ListCamerasRequest, opts ...grpc.CallOption) (*ListCamerasResponse, error)
	GetCamera(ctx context.Context, in *GetCameraRequest, opts ...grpc.CallOption) (*Camera, error)
}

type cameraServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewCameraServiceClient(cc grpc.ClientConnInterface) CameraServiceClient {
	return &cameraServiceClient{cc}
}

func (c *cameraServiceClient) ListCameras(ctx context.Context, in *ListCamerasRequest, opts ...grpc.CallOption) (*ListCamerasResponse, error) {
	out := new(ListCamerasResponse)
	err := c.cc.Invoke(ctx, "/dam_v1.CameraService/ListCameras", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *cameraServiceClient) GetCamera(ctx context.Context, in *GetCameraRequest, opts ...grpc.CallOption) (*Camera, error) {
	out := new(Camera)
	err := c.cc.Invoke(ctx, "/dam_v1.CameraService/GetCamera", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}
