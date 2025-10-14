package httpapi

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	v1 "qazna.org/api/gen/go/api/proto/qazna/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

func startBufGRPC(t *testing.T, srv *GRPCServer) (*grpc.ClientConn, func()) {
	t.Helper()

	listener := bufconn.Listen(bufSize)
	server := grpc.NewServer()
	v1.RegisterInfoServiceServer(server, srv)
	v1.RegisterHealthServiceServer(server, srv)

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("grpc serve error: %v", err)
		}
	}()

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return listener.Dial()
	}
	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufnet: %v", err)
	}

	cleanup := func() {
		server.GracefulStop()
		_ = conn.Close()
		_ = listener.Close()
	}
	return conn, cleanup
}

func TestGRPCServer_InfoAndHealth(t *testing.T) {
	srv := NewGRPCServer(ReadyProbe{}, "1.2.3", nil)
	conn, cleanup := startBufGRPC(t, srv)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	infoResp, err := v1.NewInfoServiceClient(conn).GetInfo(ctx, &v1.InfoRequest{})
	if err != nil {
		t.Fatalf("GetInfo error: %v", err)
	}
	if infoResp.GetName() != serviceName || infoResp.GetVersion() != "1.2.3" {
		t.Fatalf("unexpected info response: %+v", infoResp)
	}
	if _, err := time.Parse(time.RFC3339, infoResp.GetTimeRfc3339()); err != nil {
		t.Fatalf("invalid time format: %v", err)
	}

	healthResp, err := v1.NewHealthServiceClient(conn).Check(ctx, &v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if healthResp.GetStatus() != "ok" {
		t.Fatalf("unexpected status: %s", healthResp.GetStatus())
	}
}

type failingReadiness struct{}

func (f failingReadiness) Check(context.Context) error { return errors.New("boom") }

func TestGRPCServer_HealthFailure(t *testing.T) {
	srv := NewGRPCServer(failingReadiness{}, "1.0.0", nil)
	conn, cleanup := startBufGRPC(t, srv)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := v1.NewHealthServiceClient(conn).Check(ctx, &v1.HealthCheckRequest{})
	if err == nil {
		t.Fatal("expected health check error")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unavailable {
		t.Fatalf("unexpected status: %v", err)
	}
}
