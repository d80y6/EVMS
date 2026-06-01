package common

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func GRPCServerTLSCredentials() (credentials.TransportCredentials, error) {
	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")
	if certFile == "" || keyFile == "" {
		slog.Warn("TLS_CERT_FILE/TLS_KEY_FILE not set, gRPC server will use insecure")
		return nil, nil
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load server cert/key: %w", err)
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	caFile := os.Getenv("TLS_CA_FILE")
	if caFile != "" {
		caCert, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %w", err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert")
		}
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
		tlsCfg.ClientCAs = caPool
	}

	slog.Info("gRPC server configured with TLS")
	return credentials.NewTLS(tlsCfg), nil
}

func GRPCClientTLSCredentials(serverName string) (credentials.TransportCredentials, error) {
	caFile := os.Getenv("TLS_CA_FILE")
	if caFile == "" {
		slog.Warn("TLS_CA_FILE not set, gRPC client will use insecure",
			"server", serverName)
		return insecure.NewCredentials(), nil
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA cert")
	}

	tlsCfg := &tls.Config{
		ServerName: serverName,
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
	}

	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")
	if certFile != "" && keyFile != "" {
		clientCert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{clientCert}
	}

	slog.Info("gRPC client configured with TLS", "server", serverName)
	return credentials.NewTLS(tlsCfg), nil
}
