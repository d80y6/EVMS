package main

import (
	"context"
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

type Service struct {
	damv1.UnimplementedCameraServiceServer
	db     *sqlx.DB
	logger *slog.Logger
}

func NewService(dbURL string, logger *slog.Logger) (*Service, error) {
	db, err := sqlx.Connect("postgres", dbURL)
	if err != nil {
		return nil, err
	}
	return &Service{db: db, logger: logger}, nil
}

func (s *Service) ListCameras(ctx context.Context, req *damv1.ListCamerasRequest) (*damv1.ListCamerasResponse, error) {
	var cameras []Camera
	var err error

	if req.SiteId != "" {
		err = s.db.SelectContext(ctx, &cameras, "SELECT id, site_id, name, description, connection_url, substream_url, status, created_at FROM cameras WHERE site_id = $1", req.SiteId)
	} else {
		err = s.db.SelectContext(ctx, &cameras, "SELECT id, site_id, name, description, connection_url, substream_url, status, created_at FROM cameras")
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

func (s *Service) GetCamera(ctx context.Context, req *damv1.GetCameraRequest) (*damv1.Camera, error) {
	var c Camera
	err := s.db.GetContext(ctx, &c, "SELECT id, site_id, name, description, connection_url, substream_url, status, created_at FROM cameras WHERE id = $1", req.Id)
	if err != nil {
		s.logger.Error("Failed to get camera", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.NotFound, "camera not found")
	}
	return s.mapCameraToProto(c), nil
}

func (s *Service) CreateCamera(ctx context.Context, req *damv1.CreateCameraRequest) (*damv1.Camera, error) {
	var id string
	err := s.db.QueryRowContext(ctx, "INSERT INTO cameras (site_id, name, connection_url, substream_url) VALUES ($1, $2, $3, $4) RETURNING id",
		req.SiteId, req.Name, req.ConnectionUrl, req.SubstreamUrl).Scan(&id)
	if err != nil {
		s.logger.Error("Failed to create camera", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to create camera: %v", err)
	}

	return s.GetCamera(ctx, &damv1.GetCameraRequest{Id: id})
}

func (s *Service) UpdateCamera(ctx context.Context, req *damv1.UpdateCameraRequest) (*damv1.Camera, error) {
	_, err := s.db.ExecContext(ctx, "UPDATE cameras SET name = $1, description = $2, connection_url = $3, substream_url = $4, updated_at = NOW() WHERE id = $5",
		req.Name, req.Description, req.ConnectionUrl, req.SubstreamUrl, req.Id)
	if err != nil {
		s.logger.Error("Failed to update camera", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.Internal, "failed to update camera: %v", err)
	}

	return s.GetCamera(ctx, &damv1.GetCameraRequest{Id: req.Id})
}

func (s *Service) DeleteCamera(ctx context.Context, req *damv1.DeleteCameraRequest) (*damv1.DeleteCameraResponse, error) {
	_, err := s.db.ExecContext(ctx, "DELETE FROM cameras WHERE id = $1", req.Id)
	if err != nil {
		s.logger.Error("Failed to delete camera", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.Internal, "failed to delete camera: %v", err)
	}
	return &damv1.DeleteCameraResponse{Success: true}, nil
}

func (s *Service) StreamStatus(ctx context.Context, req *damv1.StreamStatusRequest) (*damv1.StreamStatusResponse, error) {
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

func (s *Service) mapCameraToProto(c Camera) *damv1.Camera {
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

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	common.StartMetricsServer(":2112")

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = "postgres://dam_admin:dam_password@localhost:5432/dam_vms?sslmode=disable"
	}

	service, err := NewService(dbURL, logger)
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer service.db.Close()

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		logger.Error("Failed to listen", "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	damv1.RegisterCameraServiceServer(grpcServer, service)
	reflection.Register(grpcServer)

	logger.Info("Camera Management Service (gRPC) listening", "address", ":50051")

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("Failed to serve gRPC", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down Camera Management Service...")
	grpcServer.GracefulStop()
}
