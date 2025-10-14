package httpapi

import (
	"context"
	"errors"
	"strings"
	"time"

	v1 "qazna.org/api/gen/go/api/proto/qazna/v1"
	"qazna.org/internal/auth"
	"qazna.org/internal/obs"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const serviceName = "qazna-api"

// GRPCServer implements the gRPC services defined in api/proto/qazna/v1.
type GRPCServer struct {
	v1.UnimplementedInfoServiceServer
	v1.UnimplementedHealthServiceServer

	readiness readinessChecker
	version   string
	auth      *auth.Service
}

const bearerMetadataPrefix = "Bearer "

// NewGRPCServer creates the gRPC service wrapper.
func NewGRPCServer(r readinessChecker, version string, authSvc *auth.Service) *GRPCServer {
	return &GRPCServer{
		readiness: r,
		version:   version,
		auth:      authSvc,
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

// UnaryAuthInterceptor authenticates incoming gRPC requests and enriches context.
func UnaryAuthInterceptor(authSvc *auth.Service) grpc.UnaryServerInterceptor {
	if authSvc == nil || !authSvc.SupportsTokens() {
		return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			return handler(ctx, req)
		}
	}

	publicMethods := map[string]struct{}{
		"/qazna.v1.HealthService/Check": {},
		"/qazna.v1.InfoService/GetInfo": {},
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if _, ok := publicMethods[info.FullMethod]; ok {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		token, err := tokenFromMetadata(md)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}
		principal, err := authSvc.AuthenticateToken(ctx, token)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidToken) {
				return nil, status.Error(codes.Unauthenticated, "invalid token")
			}
			return nil, status.Error(codes.Internal, "authentication error")
		}

		ctx = auth.ContextWithPrincipal(ctx, principal)
		ctx = auth.ContextWithToken(ctx, token)
		return handler(ctx, req)
	}
}

func tokenFromMetadata(md metadata.MD) (string, error) {
	values := md.Get("authorization")
	if len(values) == 0 {
		return "", errors.New("missing bearer token")
	}
	header := strings.TrimSpace(values[0])
	if header == "" {
		return "", errors.New("missing bearer token")
	}
	if !strings.HasPrefix(strings.ToLower(header), strings.ToLower(bearerMetadataPrefix)) {
		return "", errors.New("invalid authorization scheme")
	}
	token := strings.TrimSpace(header[len(bearerMetadataPrefix):])
	if token == "" {
		return "", errors.New("missing bearer token")
	}
	return token, nil
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
