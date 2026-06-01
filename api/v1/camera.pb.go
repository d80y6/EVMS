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
	PtzProtocol   string
	RetentionDays    int32
	PrerecordSeconds int32
	OnvifData        string
	Config           string
	CreatedAt        *timestamppb.Timestamp
}

type Site struct {
	Id        string
	Name      string
	Location  string
	CreatedAt *timestamppb.Timestamp
}

type ListCamerasRequest struct {
	SiteId string
}

type ListCamerasResponse struct {
	Cameras []*Camera
}

type CreateCameraRequest struct {
	SiteId           string
	Name             string
	ConnectionUrl    string
	SubstreamUrl     string
	PtzProtocol      string
	RetentionDays    int32
	PrerecordSeconds int32
}

type GetCameraRequest struct {
	Id string
}

type UpdateCameraRequest struct {
	Id               string
	Name             string
	Description      string
	ConnectionUrl    string
	SubstreamUrl     string
	PtzProtocol      string
	RetentionDays    int32
	PrerecordSeconds int32
	Config           string
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

type ListSitesRequest struct{}

type ListSitesResponse struct {
	Sites []*Site
}

type CreateSiteRequest struct {
	Name     string
	Location string
}

type UpdateSiteRequest struct {
	Id       string
	Name     string
	Location string
}

type DeleteSiteRequest struct {
	Id string
}

type DeleteSiteResponse struct {
	Success bool
}

type SmartSearchRequest struct {
	CameraId      string
	ObjectType    string
	MinConfidence float64
	StartTime     string
	EndTime       string
	Limit         int32
	BoundingBox   string
}

type SmartSearchResult struct {
	Id          string
	CameraId    string
	EventTime   string
	ObjectType  string
	Confidence  float64
	BoundingBox string
	TrackId     string
	Thumbnail   string
}

type SmartSearchResponse struct {
	Results []*SmartSearchResult
	Total   int32
}

// CameraServiceServer
type CameraServiceServer interface {
	ListCameras(context.Context, *ListCamerasRequest) (*ListCamerasResponse, error)
	GetCamera(context.Context, *GetCameraRequest) (*Camera, error)
	CreateCamera(context.Context, *CreateCameraRequest) (*Camera, error)
	UpdateCamera(context.Context, *UpdateCameraRequest) (*Camera, error)
	DeleteCamera(context.Context, *DeleteCameraRequest) (*DeleteCameraResponse, error)
	StreamStatus(context.Context, *StreamStatusRequest) (*StreamStatusResponse, error)
	ListSites(context.Context, *ListSitesRequest) (*ListSitesResponse, error)
	CreateSite(context.Context, *CreateSiteRequest) (*Site, error)
	UpdateSite(context.Context, *UpdateSiteRequest) (*Site, error)
	DeleteSite(context.Context, *DeleteSiteRequest) (*DeleteSiteResponse, error)
	SmartSearch(context.Context, *SmartSearchRequest) (*SmartSearchResponse, error)
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
func (UnimplementedCameraServiceServer) ListSites(context.Context, *ListSitesRequest) (*ListSitesResponse, error) {
	return nil, nil
}
func (UnimplementedCameraServiceServer) CreateSite(context.Context, *CreateSiteRequest) (*Site, error) {
	return nil, nil
}
func (UnimplementedCameraServiceServer) UpdateSite(context.Context, *UpdateSiteRequest) (*Site, error) {
	return nil, nil
}
func (UnimplementedCameraServiceServer) DeleteSite(context.Context, *DeleteSiteRequest) (*DeleteSiteResponse, error) {
	return nil, nil
}
func (UnimplementedCameraServiceServer) SmartSearch(context.Context, *SmartSearchRequest) (*SmartSearchResponse, error) {
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
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/dam_v1.CameraService/ListCameras"}
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
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/dam_v1.CameraService/GetCamera"}
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
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/dam_v1.CameraService/CreateCamera"}
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
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/dam_v1.CameraService/UpdateCamera"}
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
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/dam_v1.CameraService/DeleteCamera"}
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
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/dam_v1.CameraService/StreamStatus"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CameraServiceServer).StreamStatus(ctx, req.(*StreamStatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _CameraService_ListSites_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListSitesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CameraServiceServer).ListSites(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/dam_v1.CameraService/ListSites"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CameraServiceServer).ListSites(ctx, req.(*ListSitesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _CameraService_CreateSite_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateSiteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CameraServiceServer).CreateSite(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/dam_v1.CameraService/CreateSite"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CameraServiceServer).CreateSite(ctx, req.(*CreateSiteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _CameraService_UpdateSite_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateSiteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CameraServiceServer).UpdateSite(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/dam_v1.CameraService/UpdateSite"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CameraServiceServer).UpdateSite(ctx, req.(*UpdateSiteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _CameraService_DeleteSite_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteSiteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CameraServiceServer).DeleteSite(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/dam_v1.CameraService/DeleteSite"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CameraServiceServer).DeleteSite(ctx, req.(*DeleteSiteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _CameraService_SmartSearch_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SmartSearchRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CameraServiceServer).SmartSearch(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/dam_v1.CameraService/SmartSearch"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CameraServiceServer).SmartSearch(ctx, req.(*SmartSearchRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var CameraService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "dam_v1.CameraService",
	HandlerType: (*CameraServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "ListCameras", Handler: _CameraService_ListCameras_Handler},
		{MethodName: "GetCamera", Handler: _CameraService_GetCamera_Handler},
		{MethodName: "CreateCamera", Handler: _CameraService_CreateCamera_Handler},
		{MethodName: "UpdateCamera", Handler: _CameraService_UpdateCamera_Handler},
		{MethodName: "DeleteCamera", Handler: _CameraService_DeleteCamera_Handler},
		{MethodName: "StreamStatus", Handler: _CameraService_StreamStatus_Handler},
		{MethodName: "ListSites", Handler: _CameraService_ListSites_Handler},
		{MethodName: "CreateSite", Handler: _CameraService_CreateSite_Handler},
		{MethodName: "UpdateSite", Handler: _CameraService_UpdateSite_Handler},
		{MethodName: "DeleteSite", Handler: _CameraService_DeleteSite_Handler},
		{MethodName: "SmartSearch", Handler: _CameraService_SmartSearch_Handler},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "api/proto/v1/camera.proto",
}

type CameraServiceClient interface {
	ListCameras(ctx context.Context, in *ListCamerasRequest, opts ...grpc.CallOption) (*ListCamerasResponse, error)
	GetCamera(ctx context.Context, in *GetCameraRequest, opts ...grpc.CallOption) (*Camera, error)
	ListSites(ctx context.Context, in *ListSitesRequest, opts ...grpc.CallOption) (*ListSitesResponse, error)
	CreateSite(ctx context.Context, in *CreateSiteRequest, opts ...grpc.CallOption) (*Site, error)
	UpdateCamera(ctx context.Context, in *UpdateCameraRequest, opts ...grpc.CallOption) (*Camera, error)
	SmartSearch(ctx context.Context, in *SmartSearchRequest, opts ...grpc.CallOption) (*SmartSearchResponse, error)
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

func (c *cameraServiceClient) ListSites(ctx context.Context, in *ListSitesRequest, opts ...grpc.CallOption) (*ListSitesResponse, error) {
	out := new(ListSitesResponse)
	err := c.cc.Invoke(ctx, "/dam_v1.CameraService/ListSites", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *cameraServiceClient) CreateSite(ctx context.Context, in *CreateSiteRequest, opts ...grpc.CallOption) (*Site, error) {
	out := new(Site)
	err := c.cc.Invoke(ctx, "/dam_v1.CameraService/CreateSite", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *cameraServiceClient) UpdateCamera(ctx context.Context, in *UpdateCameraRequest, opts ...grpc.CallOption) (*Camera, error) {
	out := new(Camera)
	err := c.cc.Invoke(ctx, "/dam_v1.CameraService/UpdateCamera", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *cameraServiceClient) SmartSearch(ctx context.Context, in *SmartSearchRequest, opts ...grpc.CallOption) (*SmartSearchResponse, error) {
	out := new(SmartSearchResponse)
	err := c.cc.Invoke(ctx, "/dam_v1.CameraService/SmartSearch", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}
