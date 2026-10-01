package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const defaultProxyLogLevel = "INFO"

const (
	proxyLogDebug int32 = iota
	proxyLogInfo
	proxyLogWarning
	proxyLogError
)

var activeProxyLogLevel atomic.Int32

func init() {
	activeProxyLogLevel.Store(proxyLogInfo)
}

func parseProxyLogLevel(level string) (int32, bool) {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG":
		return proxyLogDebug, true
	case "INFO":
		return proxyLogInfo, true
	case "WARNING":
		return proxyLogWarning, true
	case "ERROR":
		return proxyLogError, true
	default:
		return 0, false
	}
}

func setActiveProxyLogLevel(level string) bool {
	value, ok := parseProxyLogLevel(level)
	if !ok {
		return false
	}
	activeProxyLogLevel.Store(value)
	return true
}

func activeProxyLogLevelName() string {
	switch activeProxyLogLevel.Load() {
	case proxyLogDebug:
		return "DEBUG"
	case proxyLogWarning:
		return "WARNING"
	case proxyLogError:
		return "ERROR"
	default:
		return "INFO"
	}
}

var errForbiddenDestination = errors.New("destination is not allowed")

func (s *server) authenticateProxyRequest(w http.ResponseWriter, r *http.Request) bool {
	_, authenticated := s.authenticateProxyUserRequest(w, r)
	return authenticated
}

func (s *server) authenticateProxyUserRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	header := r.Header.Get("Proxy-Authorization")
	scheme, encoded, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Basic") {
		writeProxyChallenge(w)
		logProxyEvent("auth_denied", "", r.RemoteAddr, r.Method, requestTarget(r), http.StatusProxyAuthRequired, 0, 0, 0)
		return "", false
	}
	credentials, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		writeProxyChallenge(w)
		logProxyEvent("auth_denied", "", r.RemoteAddr, r.Method, requestTarget(r), http.StatusProxyAuthRequired, 0, 0, 0)
		return "", false
	}
	username, password, ok := strings.Cut(string(credentials), ":")
	if !ok || !s.users.authenticateProxy(username, password) {
		writeProxyChallenge(w)
		logProxyEvent("auth_denied", "", r.RemoteAddr, r.Method, requestTarget(r), http.StatusProxyAuthRequired, 0, 0, 0)
		return "", false
	}
	logProxyEvent("auth_accepted", username, r.RemoteAddr, r.Method, requestTarget(r), http.StatusOK, 0, 0, 0)
	return username, true
}

func writeProxyChallenge(w http.ResponseWriter) {
	w.Header().Set("Proxy-Authenticate", `Basic realm="Homerouter Proxy", charset="UTF-8"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusProxyAuthRequired)
}

func (s *server) handleConnect(w http.ResponseWriter, r *http.Request) {
	username, authenticated := s.authenticateProxyUserRequest(w, r)
	if !authenticated {
		return
	}
	finishActivity := s.activity.begin(username)
	defer finishActivity()
	started := time.Now()
	upstream, err := dialPublicTarget(r.Context(), r.Host, "443")
	if err != nil {
		http.Error(w, "proxy destination is unavailable", http.StatusBadGateway)
		logProxyEvent("connect", username, r.RemoteAddr, r.Method, r.Host, http.StatusBadGateway, 0, 0, time.Since(started))
		return
	}
	client, buffered, err := w.(http.Hijacker).Hijack()
	if err != nil {
		upstream.Close()
		logProxyEvent("connect", username, r.RemoteAddr, r.Method, r.Host, http.StatusInternalServerError, 0, 0, time.Since(started))
		return
	}
	if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		client.Close()
		upstream.Close()
		logProxyEvent("connect", username, r.RemoteAddr, r.Method, r.Host, http.StatusInternalServerError, 0, 0, time.Since(started))
		return
	}
	if err := buffered.Flush(); err != nil {
		client.Close()
		upstream.Close()
		logProxyEvent("connect", username, r.RemoteAddr, r.Method, r.Host, http.StatusInternalServerError, 0, 0, time.Since(started))
		return
	}
	logProxyEvent("connect_open", username, r.RemoteAddr, r.Method, r.Host, http.StatusOK, 0, 0, time.Since(started))

	finished := make(chan struct{}, 2)
	var uploaded atomic.Int64
	var downloaded atomic.Int64
	go func() {
		count, _ := io.Copy(upstream, buffered.Reader)
		uploaded.Store(count)
		closeWrite(upstream)
		finished <- struct{}{}
	}()
	go func() {
		count, _ := io.Copy(buffered, upstream)
		downloaded.Store(count)
		_ = buffered.Flush()
		closeWrite(client)
		finished <- struct{}{}
	}()
	<-finished
	_ = client.Close()
	_ = upstream.Close()
	<-finished
	logProxyEvent("connect", username, r.RemoteAddr, r.Method, r.Host, http.StatusOK, uploaded.Load(), downloaded.Load(), time.Since(started))
}

func (s *server) handleForward(w http.ResponseWriter, r *http.Request) {
	username, authenticated := s.authenticateProxyUserRequest(w, r)
	if !authenticated {
		return
	}
	finishActivity := s.activity.begin(username)
	defer finishActivity()
	started := time.Now()
	if r.URL == nil || !r.URL.IsAbs() || !strings.EqualFold(r.URL.Scheme, "http") || r.URL.Host == "" {
		http.Error(w, "only absolute-form HTTP proxy requests are supported", http.StatusBadRequest)
		logProxyEvent("http", username, r.RemoteAddr, r.Method, requestTarget(r), http.StatusBadRequest, r.ContentLength, 0, time.Since(started))
		return
	}

	request := r.Clone(r.Context())
	request.RequestURI = ""
	request.Header = r.Header.Clone()
	request.Header.Del("Proxy-Authorization")
	removeHopHeaders(request.Header)

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialPublicTarget(ctx, address, "80")
		},
		ResponseHeaderTimeout: 20 * time.Second,
		IdleConnTimeout:       30 * time.Second,
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		logProxyEvent("http", username, r.RemoteAddr, r.Method, r.URL.Host, http.StatusBadGateway, r.ContentLength, 0, time.Since(started))
		return
	}
	defer response.Body.Close()
	copyHeaders(w.Header(), response.Header)
	removeHopHeaders(w.Header())
	w.WriteHeader(response.StatusCode)
	responseBytes, _ := io.Copy(w, response.Body)
	logProxyEvent("http", username, r.RemoteAddr, r.Method, r.URL.Host, response.StatusCode, r.ContentLength, responseBytes, time.Since(started))
}

func requestTarget(r *http.Request) string {
	if r.Method == http.MethodConnect {
		return r.Host
	}
	if r.URL != nil && r.URL.IsAbs() {
		return r.URL.Host
	}
	return r.Host
}

func logProxyEvent(event, username, remote, method, target string, status int, uploaded, downloaded int64, duration time.Duration) {
	level := proxyLogInfo
	if event == "auth_denied" || status >= http.StatusBadRequest && status < http.StatusInternalServerError {
		level = proxyLogWarning
	}
	if event == "auth_accepted" {
		level = proxyLogDebug
	}
	if status >= http.StatusInternalServerError {
		level = proxyLogError
	}
	if level < activeProxyLogLevel.Load() {
		return
	}
	log.Printf("level=%s proxy event=%s user=%q remote=%q method=%q target=%q status=%d uploaded_bytes=%d downloaded_bytes=%d duration_ms=%d",
		logLevelName(level), event, username, remote, method, target, status, uploaded, downloaded, duration.Milliseconds())
}

func logLevelName(level int32) string {
	switch level {
	case proxyLogDebug:
		return "DEBUG"
	case proxyLogWarning:
		return "WARNING"
	case proxyLogError:
		return "ERROR"
	default:
		return "INFO"
	}
}

func removeHopHeaders(header http.Header) {
	for _, value := range header.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			header.Del(strings.TrimSpace(name))
		}
	}
	for _, name := range []string{
		"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding", "Upgrade",
	} {
		header.Del(name)
	}
}

func copyHeaders(destination, source http.Header) {
	for name, values := range source {
		for _, value := range values {
			destination.Add(name, value)
		}
	}
}

func closeWrite(connection net.Conn) {
	if tcp, ok := connection.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
		return
	}
	_ = connection.Close()
}

func dialPublicTarget(ctx context.Context, address, defaultPort string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		if strings.Contains(err.Error(), "missing port in address") {
			host = address
			port = defaultPort
		} else {
			return nil, fmt.Errorf("invalid target address")
		}
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || (portNumber != 80 && portNumber != 443) {
		return nil, errForbiddenDestination
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve target")
	}
	dialer := net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	var lastError error
	for _, candidate := range ips {
		if !isPublicIP(candidate.IP) {
			continue
		}
		connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(candidate.IP.String(), port))
		if err == nil {
			return connection, nil
		}
		lastError = err
	}
	if lastError != nil {
		return nil, lastError
	}
	return nil, errForbiddenDestination
}

func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || !ip.IsGlobalUnicast() {
		return false
	}
	blocked := []string{
		"0.0.0.0/8",
		"100.64.0.0/10",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"192.88.99.0/24",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"240.0.0.0/4",
		"64:ff9b::/96",
		"64:ff9b:1::/48",
		"100::/64",
		"2001::/23",
		"2001:db8::/32",
		"2001:10::/28",
		"2001:20::/28",
		"2002::/16",
		"3fff::/20",
		"5f00::/16",
		"fec0::/10",
	}
	for _, cidr := range blocked {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil && network.Contains(ip) {
			return false
		}
	}
	return true
}
