package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dam-vms/dam/api/v1"
	"github.com/dam-vms/dam/pkg/common"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CameraConfig holds configuration for the camera management service
type CameraConfig struct {
	DBURL           string
	GRPCPort        string
	MetricsPort     string
	GracefulTimeout time.Duration
}

// DefaultCameraConfig returns default configuration values
func DefaultCameraConfig() *CameraConfig {
	return &CameraConfig{
		DBURL:           common.GetEnv("DB_URL", "postgres://dam_admin:dam_password@localhost:5432/dam_vms?sslmode=disable"),
		GRPCPort:        common.GetEnv("GRPC_PORT", ":50051"),
		MetricsPort:     common.GetEnv("METRICS_ADDR", ":2112"),
		GracefulTimeout: 30 * time.Second,
	}
}

// Camera represents a camera entity in the database
type Camera struct {
	ID            string    `db:"id"`
	SiteID        string    `db:"site_id"`
	Name          string    `db:"name"`
	Description   string    `db:"description"`
	ConnectionURL string    `db:"connection_url"`
	SubstreamURL  string    `db:"substream_url"`
	Status        string    `db:"status"`
	PtzProtocol   string    `db:"ptz_protocol"`
	RetentionDays int       `db:"retention_days"`
	OnvifData     string    `db:"onvif_data"`
	Config        string    `db:"config"`
	CreatedAt     time.Time `db:"created_at"`
}

// Site represents a site entity in the database
type Site struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	Location  string    `db:"location"`
	CreatedAt time.Time `db:"created_at"`
}

// AiEvent represents a row from the ai_events table
type AiEvent struct {
	ID          string  `db:"id"`
	CameraID    string  `db:"camera_id"`
	EventTime   string  `db:"event_time"`
	ObjectType  string  `db:"object_type"`
	Confidence  float64 `db:"confidence"`
	BoundingBox string  `db:"bounding_box"`
	TrackID     string  `db:"track_id"`
	Thumbnail   string  `db:"thumbnail"`
}

// CameraService handles camera management operations
type CameraService struct {
	damv1.UnimplementedCameraServiceServer
	config *CameraConfig
	db     *sqlx.DB
	logger *slog.Logger
	server *grpc.Server
}

// NewCameraService creates a new camera service instance
func NewCameraService(config *CameraConfig, logger *slog.Logger) (*CameraService, error) {
	if config.DBURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	db, err := sqlx.Connect("postgres", config.DBURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &CameraService{
		config: config,
		db:     db,
		logger: logger,
	}, nil
}

// Start initializes and starts the gRPC server
func (s *CameraService) Start() error {
	lis, err := net.Listen("tcp", s.config.GRPCPort)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.config.GRPCPort, err)
	}

	s.server = grpc.NewServer()
	damv1.RegisterCameraServiceServer(s.server, s)
	reflection.Register(s.server)

	s.logger.Info("Camera Management Service (gRPC) started",
		"address", s.config.GRPCPort,
		"metrics_address", s.config.MetricsPort)

	go func() {
		if err := s.server.Serve(lis); err != nil {
			s.logger.Error("Failed to serve gRPC", "error", err)
		}
	}()

	return nil
}

// ListCameras returns a list of cameras, optionally filtered by site ID
func (s *CameraService) ListCameras(ctx context.Context, req *damv1.ListCamerasRequest) (*damv1.ListCamerasResponse, error) {
	var cameras []Camera
	var err error

	if req.SiteId != "" {
		err = s.db.SelectContext(ctx, &cameras,
			"SELECT id, site_id, name, description, connection_url, substream_url, status, ptz_protocol, retention_days, COALESCE(onvif_data, '') AS onvif_data, COALESCE(config, '') AS config, created_at FROM cameras WHERE site_id = $1",
			req.SiteId)
	} else {
		err = s.db.SelectContext(ctx, &cameras,
			"SELECT id, site_id, name, description, connection_url, substream_url, status, ptz_protocol, retention_days, COALESCE(onvif_data, '') AS onvif_data, COALESCE(config, '') AS config, created_at FROM cameras")
	}

	if err != nil {
		s.logger.Error("Failed to list cameras", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to list cameras: %v", err)
	}

	resp := &damv1.ListCamerasResponse{
		Cameras: make([]*damv1.Camera, len(cameras)),
	}

	for i, c := range cameras {
		resp.Cameras[i] = s.mapCameraToProto(c)
	}

	return resp, nil
}

// GetCamera returns a single camera by ID
func (s *CameraService) GetCamera(ctx context.Context, req *damv1.GetCameraRequest) (*damv1.Camera, error) {
	var c Camera
	err := s.db.GetContext(ctx, &c,
		"SELECT id, site_id, name, description, connection_url, substream_url, status, ptz_protocol, retention_days, COALESCE(onvif_data, '') AS onvif_data, COALESCE(config, '') AS config, created_at FROM cameras WHERE id = $1",
		req.Id)
	if err != nil {
		s.logger.Error("Failed to get camera", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.NotFound, "camera not found")
	}
	return s.mapCameraToProto(c), nil
}

// CreateCamera creates a new camera record
func (s *CameraService) CreateCamera(ctx context.Context, req *damv1.CreateCameraRequest) (*damv1.Camera, error) {
	ptzProtocol := req.PtzProtocol
	if ptzProtocol == "" {
		ptzProtocol = "NONE"
	}
	retentionDays := req.RetentionDays
	if retentionDays == 0 {
		retentionDays = 30
	}

	var id string
	err := s.db.QueryRowContext(ctx,
		"INSERT INTO cameras (site_id, name, connection_url, substream_url, ptz_protocol, retention_days) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id",
		req.SiteId, req.Name, req.ConnectionUrl, req.SubstreamUrl, ptzProtocol, retentionDays).Scan(&id)
	if err != nil {
		s.logger.Error("Failed to create camera", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to create camera: %v", err)
	}

	return s.GetCamera(ctx, &damv1.GetCameraRequest{Id: id})
}

// UpdateCamera updates an existing camera record
func (s *CameraService) UpdateCamera(ctx context.Context, req *damv1.UpdateCameraRequest) (*damv1.Camera, error) {
	_, err := s.db.ExecContext(ctx,
		"UPDATE cameras SET name = $1, description = $2, connection_url = $3, substream_url = $4, ptz_protocol = $5, retention_days = $6, config = $7, updated_at = NOW() WHERE id = $8",
		req.Name, req.Description, req.ConnectionUrl, req.SubstreamUrl, req.PtzProtocol, req.RetentionDays, req.Config, req.Id)
	if err != nil {
		s.logger.Error("Failed to update camera", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.Internal, "failed to update camera: %v", err)
	}

	return s.GetCamera(ctx, &damv1.GetCameraRequest{Id: req.Id})
}

// DeleteCamera removes a camera record
func (s *CameraService) DeleteCamera(ctx context.Context, req *damv1.DeleteCameraRequest) (*damv1.DeleteCameraResponse, error) {
	_, err := s.db.ExecContext(ctx, "DELETE FROM cameras WHERE id = $1", req.Id)
	if err != nil {
		s.logger.Error("Failed to delete camera", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.Internal, "failed to delete camera: %v", err)
	}
	return &damv1.DeleteCameraResponse{Success: true}, nil
}

// StreamStatus returns the current stream status for a camera
func (s *CameraService) StreamStatus(ctx context.Context, req *damv1.StreamStatusRequest) (*damv1.StreamStatusResponse, error) {
	var statusStr string
	err := s.db.GetContext(ctx, &statusStr, "SELECT status FROM cameras WHERE id = $1", req.CameraId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "camera not found")
	}

	return &damv1.StreamStatusResponse{
		Status:  statusStr,
		Bitrate: 2500.0,
		Fps:     30.0,
	}, nil
}

// ListSites returns a list of all sites
func (s *CameraService) ListSites(ctx context.Context, req *damv1.ListSitesRequest) (*damv1.ListSitesResponse, error) {
	var sites []Site
	err := s.db.SelectContext(ctx, &sites, "SELECT id, name, location, created_at FROM sites ORDER BY name")
	if err != nil {
		s.logger.Error("Failed to list sites", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to list sites: %v", err)
	}

	resp := &damv1.ListSitesResponse{
		Sites: make([]*damv1.Site, len(sites)),
	}

	for i, site := range sites {
		resp.Sites[i] = &damv1.Site{
			Id:        site.ID,
			Name:      site.Name,
			Location:  site.Location,
			CreatedAt: timestamppb.New(site.CreatedAt),
		}
	}

	return resp, nil
}

// CreateSite creates a new site record
func (s *CameraService) CreateSite(ctx context.Context, req *damv1.CreateSiteRequest) (*damv1.Site, error) {
	var site Site
	err := s.db.QueryRowContext(ctx,
		"INSERT INTO sites (name, location) VALUES ($1, $2) RETURNING id, name, location, created_at",
		req.Name, req.Location).Scan(&site.ID, &site.Name, &site.Location, &site.CreatedAt)
	if err != nil {
		s.logger.Error("Failed to create site", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to create site: %v", err)
	}

	return &damv1.Site{
		Id:        site.ID,
		Name:      site.Name,
		Location:  site.Location,
		CreatedAt: timestamppb.New(site.CreatedAt),
	}, nil
}

// UpdateSite updates an existing site record
func (s *CameraService) UpdateSite(ctx context.Context, req *damv1.UpdateSiteRequest) (*damv1.Site, error) {
	var site Site
	err := s.db.QueryRowContext(ctx,
		"UPDATE sites SET name = $1, location = $2, updated_at = NOW() WHERE id = $3 RETURNING id, name, location, created_at",
		req.Name, req.Location, req.Id).Scan(&site.ID, &site.Name, &site.Location, &site.CreatedAt)
	if err != nil {
		s.logger.Error("Failed to update site", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.Internal, "failed to update site: %v", err)
	}

	return &damv1.Site{
		Id:        site.ID,
		Name:      site.Name,
		Location:  site.Location,
		CreatedAt: timestamppb.New(site.CreatedAt),
	}, nil
}

// DeleteSite removes a site record
func (s *CameraService) DeleteSite(ctx context.Context, req *damv1.DeleteSiteRequest) (*damv1.DeleteSiteResponse, error) {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sites WHERE id = $1", req.Id)
	if err != nil {
		s.logger.Error("Failed to delete site", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.Internal, "failed to delete site: %v", err)
	}
	return &damv1.DeleteSiteResponse{Success: true}, nil
}

// SmartSearch queries the ai_events table with filters
func (s *CameraService) SmartSearch(ctx context.Context, req *damv1.SmartSearchRequest) (*damv1.SmartSearchResponse, error) {
	query := "SELECT id, camera_id, event_time, object_type, confidence, bounding_box, track_id, thumbnail FROM ai_events WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if req.CameraId != "" {
		query += fmt.Sprintf(" AND camera_id = $%d", argIdx)
		args = append(args, req.CameraId)
		argIdx++
	}

	if req.ObjectType != "" {
		query += fmt.Sprintf(" AND object_type = $%d", argIdx)
		args = append(args, req.ObjectType)
		argIdx++
	}

	query += fmt.Sprintf(" AND confidence >= $%d", argIdx)
	args = append(args, req.MinConfidence)
	argIdx++

	if req.StartTime != "" || req.EndTime != "" {
		if req.StartTime != "" && req.EndTime != "" {
			query += fmt.Sprintf(" AND event_time BETWEEN $%d AND $%d", argIdx, argIdx+1)
			args = append(args, req.StartTime, req.EndTime)
			argIdx += 2
		} else if req.StartTime != "" {
			query += fmt.Sprintf(" AND event_time >= $%d", argIdx)
			args = append(args, req.StartTime)
			argIdx++
		} else {
			query += fmt.Sprintf(" AND event_time <= $%d", argIdx)
			args = append(args, req.EndTime)
			argIdx++
		}
	}

	query += " ORDER BY event_time DESC"

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT $%d", argIdx)
	args = append(args, limit)

	var events []AiEvent
	err := s.db.SelectContext(ctx, &events, query, args...)
	if err != nil {
		s.logger.Error("Failed to search events", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to search events: %v", err)
	}

	results := make([]*damv1.SmartSearchResult, len(events))
	for i, e := range events {
		results[i] = &damv1.SmartSearchResult{
			Id:          e.ID,
			CameraId:    e.CameraID,
			EventTime:   e.EventTime,
			ObjectType:  e.ObjectType,
			Confidence:  e.Confidence,
			BoundingBox: e.BoundingBox,
			TrackId:     e.TrackID,
			Thumbnail:   e.Thumbnail,
		}
	}

	return &damv1.SmartSearchResponse{
		Results: results,
		Total:   int32(len(results)),
	}, nil
}

// mapCameraToProto converts a database Camera to a protobuf Camera
func (s *CameraService) mapCameraToProto(c Camera) *damv1.Camera {
	return &damv1.Camera{
		Id:            c.ID,
		SiteId:        c.SiteID,
		Name:          c.Name,
		Description:   c.Description,
		ConnectionUrl: c.ConnectionURL,
		SubstreamUrl:  c.SubstreamURL,
		Status:        c.Status,
		PtzProtocol:   c.PtzProtocol,
		RetentionDays: int32(c.RetentionDays),
		OnvifData:     c.OnvifData,
		Config:        c.Config,
		CreatedAt:     timestamppb.New(c.CreatedAt),
	}
}

// Shutdown gracefully stops the gRPC server
func (s *CameraService) Shutdown(ctx context.Context) error {
	if s.server != nil {
		s.server.GracefulStop()
	}

	if err := s.db.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}

	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultCameraConfig()

	common.StartMetricsServer(config.MetricsPort)

	service, err := NewCameraService(config, logger)
	if err != nil {
		logger.Error("Failed to initialize camera service", "error", err)
		os.Exit(1)
	}

	if err := service.Start(); err != nil {
		logger.Error("Failed to start camera service", "error", err)
		os.Exit(1)
	}

	<-ctx.Done()
	logger.Info("Shutting down Camera Management Service...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), config.GracefulTimeout)
	defer cancel()

	if err := service.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error during shutdown", "error", err)
	}
}
