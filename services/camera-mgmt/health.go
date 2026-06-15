package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

type healthProbeResult int

const (
	probeOffline  healthProbeResult = 0
	probeDegraded healthProbeResult = 1
	probeOnline   healthProbeResult = 2
)

type healthConfig struct {
	tcpTimeout   time.Duration
	rtspTimeout  time.Duration
	onvifTimeout time.Duration
}

func defaultHealthConfig() *healthConfig {
	return &healthConfig{
		tcpTimeout:   getEnvDuration("TCP_PROBE_TIMEOUT", 3*time.Second),
		rtspTimeout:  getEnvDuration("RTSP_PROBE_TIMEOUT", 5*time.Second),
		onvifTimeout: getEnvDuration("ONVIF_PROBE_TIMEOUT", 5*time.Second),
	}
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}
	return d
}

func probeTCP(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func probeRTSP(rawURL string, timeout time.Duration) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Host
	if host == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", host, timeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))
	req := fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\nCSeq: 1\r\n\r\n", rawURL)
	if _, err := conn.Write([]byte(req)); err != nil {
		return false
	}
	resp, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return false
	}
	return strings.Contains(resp, "200 OK") || strings.Contains(resp, "401") || strings.Contains(resp, "301")
}

func probeONVIF(host string, port int, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ch := make(chan bool, 1)
	go func() {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
		if err != nil {
			ch <- false
			return
		}
		conn.Close()
		ch <- true
	}()
	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return false
	}
}
