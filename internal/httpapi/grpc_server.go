package httpapi

import (
	"context"
	"time"

	v1 "qazna.org/api/gen/go/api/proto/qazna/v1"
	"qazna.org/internal/obs"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const serviceName = "qazna-api"

// GRPCServer implements the gRPC services defined in api/proto/qazna/v1.
type GRPCServer struct {
	v1.UnimplementedInfoServiceServer
	v1.UnimplementedHealthServiceServer

	readiness readinessChecker
	version   string
}

// NewGRPCServer creates the gRPC service wrapper.
func NewGRPCServer(r readinessChecker, version string) *GRPCServer {
	return &GRPCServer{
		readiness: r,
		version:   version,
	}
}

// GetInfo returns service metadata.
func (s *GRPCServer) GetInfo(ctx context.Context, _ *v1.InfoRequest) (*v1.InfoResponse, error) {
	return &v1.InfoResponse{
		Name:        serviceName,
		Version:     s.version,
		TimeRfc3339: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// Check evaluates readiness. On failure returns gRPC Unavailable error.
func (s *GRPCServer) Check(ctx context.Context, _ *v1.HealthCheckRequest) (*v1.HealthCheckResponse, error) {
	if err := s.readiness.Check(ctx); err != nil {
		obs.SetReady(false)
		return nil, status.Errorf(codes.Unavailable, "not ready: %v", err)
	}
	obs.SetReady(true)
	return &v1.HealthCheckResponse{
		Status:      "ok",
		Service:     serviceName,
		Version:     s.version,
		TimeRfc3339: time.Now().UTC().Format(time.RFC3339),
	}, nil
}
