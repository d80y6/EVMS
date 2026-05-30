package damv1

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// NOTE: This file is a manual representation of the generated gRPC code
// because protoc is not available in the current environment.
// In a full development environment, this would be generated from ai.proto.

type DetectObjectsRequest struct {
	CameraId  string
	ImageData []byte
}

type DetectObjectsResponse struct {
	Events []*AIEvent
}

type AIEvent struct {
	Id          string
	CameraId    string
	EventTime   *timestamppb.Timestamp
	ObjectType  string
	Confidence  float32
	BoundingBox *BoundingBox
	Embedding   []float32
}

type BoundingBox struct {
	X1 float32
	Y1 float32
	X2 float32
	Y2 float32
}

type StreamEventsRequest struct {
	CameraIds   []string
	ObjectTypes []string
}

type AIService_StreamEventsServer interface {
	Send(*AIEvent) error
	grpc.ServerStream
}

type AIServiceServer interface {
	DetectObjects(context.Context, *DetectObjectsRequest) (*DetectObjectsResponse, error)
	StreamEvents(*StreamEventsRequest, AIService_StreamEventsServer) error
	mustEmbedUnimplementedAIServiceServer()
}

type UnimplementedAIServiceServer struct{}

func (UnimplementedAIServiceServer) DetectObjects(context.Context, *DetectObjectsRequest) (*DetectObjectsResponse, error) {
	return nil, nil
}
func (UnimplementedAIServiceServer) StreamEvents(*StreamEventsRequest, AIService_StreamEventsServer) error {
	return nil
}
func (UnimplementedAIServiceServer) mustEmbedUnimplementedAIServiceServer() {}

func RegisterAIServiceServer(s grpc.ServiceRegistrar, srv AIServiceServer) {
	s.RegisterService(&AIService_ServiceDesc, srv)
}

func _AIService_DetectObjects_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DetectObjectsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AIServiceServer).DetectObjects(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/dam_v1.AIService/DetectObjects",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AIServiceServer).DetectObjects(ctx, req.(*DetectObjectsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AIService_StreamEvents_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(StreamEventsRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(AIServiceServer).StreamEvents(m, &aiServiceStreamEventsServer{stream})
}

type aiServiceStreamEventsServer struct {
	grpc.ServerStream
}

func (x *aiServiceStreamEventsServer) Send(m *AIEvent) error {
	return x.ServerStream.SendMsg(m)
}

var AIService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "dam_v1.AIService",
	HandlerType: (*AIServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "DetectObjects",
			Handler:    _AIService_DetectObjects_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "StreamEvents",
			Handler:       _AIService_StreamEvents_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "api/proto/v1/ai.proto",
}
