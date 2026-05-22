package damv1

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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

type CameraServiceServer interface {
	ListCameras(context.Context, *ListCamerasRequest) (*ListCamerasResponse, error)
	mustEmbedUnimplementedCameraServiceServer()
}

type UnimplementedCameraServiceServer struct{}

func (UnimplementedCameraServiceServer) ListCameras(context.Context, *ListCamerasRequest) (*ListCamerasResponse, error) {
	return nil, nil
}
func (UnimplementedCameraServiceServer) mustEmbedUnimplementedCameraServiceServer() {}

func RegisterCameraServiceServer(s grpc.ServiceRegistrar, srv CameraServiceServer) {
	s.RegisterService(&CameraService_ServiceDesc, srv)
}

var CameraService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "dam_v1.CameraService",
	HandlerType: (*CameraServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ListCameras",
			Handler:    nil, // Simplified for this environment
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "api/proto/v1/camera.proto",
}
