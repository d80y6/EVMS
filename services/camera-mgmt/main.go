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
	err := s.db.SelectContext(ctx, &cameras, "SELECT id, name, status, connection_url FROM cameras")
	if err != nil {
		s.logger.Error("Failed to list cameras", "error", err)
		return nil, status.Errorf(codes.Internal, "Internal error")
	}

	resp := &damv1.ListCamerasResponse{
		Cameras: make([]*damv1.Camera, len(cameras)),
	}

	for i, c := range cameras {
		resp.Cameras[i] = &damv1.Camera{
			Id:            c.ID,
			Name:          c.Name,
			Status:        c.Status,
			ConnectionUrl: c.ConnectionURL,
			CreatedAt:     timestamppb.New(c.CreatedAt),
		}
	}

	return resp, nil
}

func (s *Service) CreateCamera(ctx context.Context, req *damv1.CreateCameraRequest) (*damv1.Camera, error) {
	var id string
	err := s.db.QueryRowContext(ctx, "INSERT INTO cameras (site_id, name, connection_url) VALUES ($1, $2, $3) RETURNING id",
		req.SiteId, req.Name, req.ConnectionUrl).Scan(&id)
	if err != nil {
		s.logger.Error("Failed to create camera", "error", err)
		return nil, status.Errorf(codes.Internal, "Internal error")
	}

	return &damv1.Camera{
		Id:            id,
		SiteId:        req.SiteId,
		Name:          req.Name,
		ConnectionUrl: req.ConnectionUrl,
		Status:        "offline",
		CreatedAt:     timestamppb.Now(),
	}, nil
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
