package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dam-vms/dam/api/v1"
	"github.com/dam-vms/dam/pkg/common"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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
		DBURL:           os.Getenv("DB_URL"),
		GRPCPort:        common.GetEnv("GRPC_PORT", ":50051"),
		MetricsPort:     common.GetEnv("METRICS_ADDR", ":2112"),
		GracefulTimeout: 30 * time.Second,
	}
}

// Camera represents a camera entity in the database
type Camera struct {
	ID               string    `db:"id"`
	SiteID           string    `db:"site_id"`
	Name             string    `db:"name"`
	Description      string    `db:"description"`
	ConnectionURL    string    `db:"connection_url"`
	SubstreamURL     string    `db:"substream_url"`
	Status           string    `db:"status"`
	PtzProtocol      string    `db:"ptz_protocol"`
	RetentionDays    int       `db:"retention_days"`
	PrerecordSeconds int       `db:"prerecord_seconds"`
	OnvifData        string    `db:"onvif_data"`
	Config           string    `db:"config"`
	CreatedAt        time.Time `db:"created_at"`
}

// Site represents a site entity in the database
type Site struct {
	ID        string    `db:"id"`
	TenantID  string    `db:"tenant_id"`
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
	config    *CameraConfig
	db        *sqlx.DB
	logger    *slog.Logger
	server    *grpc.Server
	healthSrv *http.Server
}

func tenantServerInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if tenant := md.Get("tenant_id"); len(tenant) > 0 && tenant[0] != "" {
			ctx = context.WithValue(ctx, common.TenantKey, tenant[0])
		}
	}
	return handler(ctx, req)
}

// NewCameraService creates a new camera service instance
func NewCameraService(config *CameraConfig, logger *slog.Logger) (*CameraService, error) {
	if config.DBURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	cb := common.NewDBCircuitBreaker("camera-mgmt")
	db, err := common.ConnectDBWithCircuitBreaker(context.Background(), "postgres", config.DBURL, cb)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	migrator := common.NewMigrator(db, common.GetEnv("MIGRATIONS_DIR", "/migrations"), logger)
	if err := migrator.Run(); err != nil {
		return nil, fmt.Errorf("migrations failed: %w", err)
	}

	h := common.NewHealthHandler()
	h.AddDBChecker(db.DB, "postgres")

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.Liveness)
	mux.HandleFunc("/ready", h.Readiness)

	return &CameraService{
		config:    config,
		db:        db,
		logger:    logger,
		healthSrv: &http.Server{Addr: ":8083", Handler: mux},
	}, nil
}

// Start initializes and starts the gRPC server
func (s *CameraService) Start() error {
	lis, err := net.Listen("tcp", s.config.GRPCPort)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.config.GRPCPort, err)
	}

	var opts []grpc.ServerOption
	opts = append(opts, grpc.ChainUnaryInterceptor(tenantServerInterceptor))
	if creds, err := common.GRPCServerTLSCredentials(); err != nil {
		return fmt.Errorf("failed to configure TLS: %w", err)
	} else if creds != nil {
		opts = append(opts, grpc.Creds(creds))
		s.logger.Info("gRPC server configured with TLS")
	}
	s.server = grpc.NewServer(opts...)
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

const camerasSelectCols = "c.id, c.site_id, c.name, c.description, c.connection_url, c.substream_url, c.status, c.ptz_protocol, c.retention_days, COALESCE(c.prerecord_seconds, 0) AS prerecord_seconds, COALESCE(c.onvif_data, '') AS onvif_data, COALESCE(c.config, '') AS config, c.created_at"

func (s *CameraService) tenantFilter(ctx context.Context) (string, string) {
	tenantID := common.TenantFromContext(ctx)
	if tenantID != "" {
		return " JOIN sites s ON c.site_id = s.id AND s.tenant_id = $", tenantID
	}
	return "", ""
}

func (s *CameraService) cameraByIDWithTenant(ctx context.Context, id, tenantID string) (*Camera, error) {
	var c Camera
	var err error
	if tenantID != "" {
		err = s.db.GetContext(ctx, &c,
			"SELECT "+camerasSelectCols+" FROM cameras c JOIN sites s ON c.site_id = s.id AND s.tenant_id = $2 WHERE c.id = $1",
			id, tenantID)
	} else {
		err = s.db.GetContext(ctx, &c,
			"SELECT "+camerasSelectCols+" FROM cameras c WHERE c.id = $1",
			id)
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListCameras returns a list of cameras, optionally filtered by site ID
func (s *CameraService) ListCameras(ctx context.Context, req *damv1.ListCamerasRequest) (*damv1.ListCamerasResponse, error) {
	var cameras []Camera
	var err error

	tenantID := common.TenantFromContext(ctx)

	if req.SiteId != "" {
		if tenantID != "" {
			err = s.db.SelectContext(ctx, &cameras,
				"SELECT "+camerasSelectCols+" FROM cameras c JOIN sites s ON c.site_id = s.id AND s.tenant_id = $2 WHERE c.site_id = $1",
				req.SiteId, tenantID)
		} else {
			err = s.db.SelectContext(ctx, &cameras,
				"SELECT "+camerasSelectCols+" FROM cameras c WHERE c.site_id = $1",
				req.SiteId)
		}
	} else {
		if tenantID != "" {
			err = s.db.SelectContext(ctx, &cameras,
				"SELECT "+camerasSelectCols+" FROM cameras c JOIN sites s ON c.site_id = s.id AND s.tenant_id = $1",
				tenantID)
		} else {
			err = s.db.SelectContext(ctx, &cameras,
				"SELECT "+camerasSelectCols+" FROM cameras c")
		}
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
	tenantID := common.TenantFromContext(ctx)
	c, err := s.cameraByIDWithTenant(ctx, req.Id, tenantID)
	if err != nil {
		s.logger.Error("Failed to get camera", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.NotFound, "camera not found")
	}
	return s.mapCameraToProto(*c), nil
}

// CreateCamera creates a new camera record
func (s *CameraService) CreateCamera(ctx context.Context, req *damv1.CreateCameraRequest) (*damv1.Camera, error) {
	tenantID := common.TenantFromContext(ctx)
	if tenantID != "" {
		var siteTenantID string
		err := s.db.GetContext(ctx, &siteTenantID, "SELECT tenant_id::text FROM sites WHERE id = $1", req.SiteId)
		if err != nil {
			s.logger.Error("Site not found", "site_id", req.SiteId)
			return nil, status.Errorf(codes.NotFound, "site not found")
		}
		if siteTenantID != tenantID {
			s.logger.Warn("cross-tenant camera creation attempt", "tenant", tenantID, "site_tenant", siteTenantID)
			return nil, status.Errorf(codes.PermissionDenied, "cannot create camera in another tenant's site")
		}
	}

	ptzProtocol := req.PtzProtocol
	if ptzProtocol == "" {
		ptzProtocol = "NONE"
	}
	retentionDays := req.RetentionDays
	if retentionDays == 0 {
		retentionDays = 30
	}

	var id string
	prerecordSeconds := req.PrerecordSeconds
	if prerecordSeconds == 0 {
		prerecordSeconds = 5
	}
	err := s.db.QueryRowContext(ctx,
		"INSERT INTO cameras (site_id, name, connection_url, substream_url, ptz_protocol, retention_days, prerecord_seconds) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id",
		req.SiteId, req.Name, req.ConnectionUrl, req.SubstreamUrl, ptzProtocol, retentionDays, prerecordSeconds).Scan(&id)
	if err != nil {
		s.logger.Error("Failed to create camera", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to create camera: %v", err)
	}

	return s.GetCamera(ctx, &damv1.GetCameraRequest{Id: id})
}

// UpdateCamera updates an existing camera record
func (s *CameraService) UpdateCamera(ctx context.Context, req *damv1.UpdateCameraRequest) (*damv1.Camera, error) {
	tenantID := common.TenantFromContext(ctx)
	if tenantID != "" {
		existing, err := s.cameraByIDWithTenant(ctx, req.Id, tenantID)
		if err != nil {
			s.logger.Error("Camera not found for tenant", "id", req.Id, "tenant", tenantID)
			return nil, status.Errorf(codes.NotFound, "camera not found")
		}
		_ = existing
	}

	prerecordSeconds := req.PrerecordSeconds
	if prerecordSeconds == 0 {
		prerecordSeconds = 5
	}
	_, err := s.db.ExecContext(ctx,
		"UPDATE cameras SET name = $1, description = $2, connection_url = $3, substream_url = $4, ptz_protocol = $5, retention_days = $6, prerecord_seconds = $7, config = $8, updated_at = NOW() WHERE id = $9",
		req.Name, req.Description, req.ConnectionUrl, req.SubstreamUrl, req.PtzProtocol, req.RetentionDays, prerecordSeconds, req.Config, req.Id)
	if err != nil {
		s.logger.Error("Failed to update camera", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.Internal, "failed to update camera: %v", err)
	}

	return s.GetCamera(ctx, &damv1.GetCameraRequest{Id: req.Id})
}

// DeleteCamera removes a camera record
func (s *CameraService) DeleteCamera(ctx context.Context, req *damv1.DeleteCameraRequest) (*damv1.DeleteCameraResponse, error) {
	tenantID := common.TenantFromContext(ctx)
	if tenantID != "" {
		existing, err := s.cameraByIDWithTenant(ctx, req.Id, tenantID)
		if err != nil {
			s.logger.Error("Camera not found for tenant", "id", req.Id, "tenant", tenantID)
			return nil, status.Errorf(codes.NotFound, "camera not found")
		}
		_ = existing
	}
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

// ListSites returns a list of all sites for the current tenant
func (s *CameraService) ListSites(ctx context.Context, req *damv1.ListSitesRequest) (*damv1.ListSitesResponse, error) {
	var sites []Site
	tenantID := common.TenantFromContext(ctx)

	var err error
	if tenantID != "" {
		err = s.db.SelectContext(ctx, &sites,
			"SELECT id, tenant_id, name, location, created_at FROM sites WHERE tenant_id = $1 ORDER BY name",
			tenantID)
	} else {
		err = s.db.SelectContext(ctx, &sites,
			"SELECT id, tenant_id, name, location, created_at FROM sites ORDER BY name")
	}
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

// CreateSite creates a new site record for the current tenant
func (s *CameraService) CreateSite(ctx context.Context, req *damv1.CreateSiteRequest) (*damv1.Site, error) {
	tenantID := common.TenantFromContext(ctx)

	var site Site
	var err error
	if tenantID != "" {
		err = s.db.QueryRowContext(ctx,
			"INSERT INTO sites (tenant_id, name, location) VALUES ($1, $2, $3) RETURNING id, tenant_id, name, location, created_at",
			tenantID, req.Name, req.Location).Scan(&site.ID, &site.TenantID, &site.Name, &site.Location, &site.CreatedAt)
	} else {
		err = s.db.QueryRowContext(ctx,
			"INSERT INTO sites (name, location) VALUES ($1, $2) RETURNING id, tenant_id, name, location, created_at",
			req.Name, req.Location).Scan(&site.ID, &site.TenantID, &site.Name, &site.Location, &site.CreatedAt)
	}
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

// UpdateSite updates an existing site record (tenant-scoped)
func (s *CameraService) UpdateSite(ctx context.Context, req *damv1.UpdateSiteRequest) (*damv1.Site, error) {
	tenantID := common.TenantFromContext(ctx)

	var site Site
	var err error
	if tenantID != "" {
		err = s.db.QueryRowContext(ctx,
			"UPDATE sites SET name = $1, location = $2, updated_at = NOW() WHERE id = $3 AND tenant_id = $4 RETURNING id, tenant_id, name, location, created_at",
			req.Name, req.Location, req.Id, tenantID).Scan(&site.ID, &site.TenantID, &site.Name, &site.Location, &site.CreatedAt)
	} else {
		err = s.db.QueryRowContext(ctx,
			"UPDATE sites SET name = $1, location = $2, updated_at = NOW() WHERE id = $3 RETURNING id, tenant_id, name, location, created_at",
			req.Name, req.Location, req.Id).Scan(&site.ID, &site.TenantID, &site.Name, &site.Location, &site.CreatedAt)
	}
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

// DeleteSite removes a site record (tenant-scoped)
func (s *CameraService) DeleteSite(ctx context.Context, req *damv1.DeleteSiteRequest) (*damv1.DeleteSiteResponse, error) {
	tenantID := common.TenantFromContext(ctx)
	var err error
	if tenantID != "" {
		_, err = s.db.ExecContext(ctx, "DELETE FROM sites WHERE id = $1 AND tenant_id = $2", req.Id, tenantID)
	} else {
		_, err = s.db.ExecContext(ctx, "DELETE FROM sites WHERE id = $1", req.Id)
	}
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

	if req.BoundingBox != "" {
		parts := strings.Split(req.BoundingBox, ",")
		if len(parts) == 4 {
			query += fmt.Sprintf(" AND bounding_box @> $%d::jsonb", argIdx)
			args = append(args, fmt.Sprintf(`[%s,%s,%s,%s]`, parts[0], parts[1], parts[2], parts[3]))
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
		Id:               c.ID,
		SiteId:           c.SiteID,
		Name:             c.Name,
		Description:      c.Description,
		ConnectionUrl:    c.ConnectionURL,
		SubstreamUrl:     c.SubstreamURL,
		Status:           c.Status,
		PtzProtocol:      c.PtzProtocol,
		RetentionDays:    int32(c.RetentionDays),
		PrerecordSeconds: int32(c.PrerecordSeconds),
		OnvifData:        c.OnvifData,
		Config:           c.Config,
		CreatedAt:        timestamppb.New(c.CreatedAt),
	}
}

// Shutdown gracefully stops the gRPC server
func (s *CameraService) Shutdown(ctx context.Context) error {
	if s.server != nil {
		s.server.GracefulStop()
	}

	if s.healthSrv != nil {
		if err := s.healthSrv.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown health server: %w", err)
		}
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

	if err := common.InitTelemetry("camera-mgmt"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	config := DefaultCameraConfig()

	common.StartMetricsServer(config.MetricsPort)
	common.StartResourceMonitor(ctx)

	service, err := NewCameraService(config, logger)
	if err != nil {
		logger.Error("Failed to initialize camera service", "error", err)
		os.Exit(1)
	}

	if err := service.Start(); err != nil {
		logger.Error("Failed to start camera service", "error", err)
		os.Exit(1)
	}

	go func() {
		logger.Info("Starting health HTTP server", "addr", ":8083")
		if err := service.healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Health server error", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down Camera Management Service...")

	shutdownCtx, cancel := context.WithTimeout(ctx, config.GracefulTimeout)
	defer cancel()

	if err := service.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error during shutdown", "error", err)
	}
}
