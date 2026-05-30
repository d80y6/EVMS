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
		DBURL:           getEnv("DB_URL", "postgres://dam_admin:dam_password@localhost:5432/dam_vms?sslmode=disable"),
		GRPCPort:        getEnv("GRPC_PORT", ":50051"),
		MetricsPort:     getEnv("METRICS_PORT", ":2112"),
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
	CreatedAt     time.Time `db:"created_at"`
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
			"SELECT id, site_id, name, description, connection_url, substream_url, status, created_at FROM cameras WHERE site_id = $1", 
			req.SiteId)
	} else {
		err = s.db.SelectContext(ctx, &cameras, 
			"SELECT id, site_id, name, description, connection_url, substream_url, status, created_at FROM cameras")
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
		"SELECT id, site_id, name, description, connection_url, substream_url, status, created_at FROM cameras WHERE id = $1", 
		req.Id)
	if err != nil {
		s.logger.Error("Failed to get camera", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.NotFound, "camera not found")
	}
	return s.mapCameraToProto(c), nil
}

// CreateCamera creates a new camera record
func (s *CameraService) CreateCamera(ctx context.Context, req *damv1.CreateCameraRequest) (*damv1.Camera, error) {
	var id string
	err := s.db.QueryRowContext(ctx, 
		"INSERT INTO cameras (site_id, name, connection_url, substream_url) VALUES ($1, $2, $3, $4) RETURNING id",
		req.SiteId, req.Name, req.ConnectionUrl, req.SubstreamUrl).Scan(&id)
	if err != nil {
		s.logger.Error("Failed to create camera", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to create camera: %v", err)
	}

	return s.GetCamera(ctx, &damv1.GetCameraRequest{Id: id})
}

// UpdateCamera updates an existing camera record
func (s *CameraService) UpdateCamera(ctx context.Context, req *damv1.UpdateCameraRequest) (*damv1.Camera, error) {
	_, err := s.db.ExecContext(ctx, 
		"UPDATE cameras SET name = $1, description = $2, connection_url = $3, substream_url = $4, updated_at = NOW() WHERE id = $5",
		req.Name, req.Description, req.ConnectionUrl, req.SubstreamUrl, req.Id)
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
	// In a real system, we might query a cache or NATS for real-time metrics.
	// For now, we fetch the basic status from the DB.
	var statusStr string
	err := s.db.GetContext(ctx, &statusStr, "SELECT status FROM cameras WHERE id = $1", req.CameraId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "camera not found")
	}

	return &damv1.StreamStatusResponse{
		Status:  statusStr,
		Bitrate: 2500.0, // Mocked for enterprise foundation
		Fps:     30.0,
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

// getEnv retrieves environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	config := DefaultCameraConfig()

	// Start metrics server
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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down Camera Management Service...")

	ctx, cancel := context.WithTimeout(context.Background(), config.GracefulTimeout)
	defer cancel()

	if err := service.Shutdown(ctx); err != nil {
		logger.Error("Error during shutdown", "error", err)
	}
}
