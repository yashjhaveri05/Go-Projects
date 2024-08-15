package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"time"
)

type Server interface {
	Address() string
	IsAlive() bool
	Serve(rw http.ResponseWriter, req *http.Request)
	IncrementConnection()
	DecrementConnection()
	Connections() int
	UpdateResponseTime(duration time.Duration)
	AverageResponseTime() time.Duration
}

type simpleServer struct {
	addr             string
	proxy            *httputil.ReverseProxy
	connections      int
	totalResponseTime time.Duration
	requests         int
	mutex            sync.Mutex
}

func newSimpleServer(addr string) *simpleServer {
	serveUrl, err := url.Parse(addr)
	handleErr(err)

	return &simpleServer{
		addr:  addr,
		proxy: httputil.NewSingleHostReverseProxy(serveUrl),
	}
}

func handleErr(err error) {
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
}

type loadBalancer struct {
	port    string
	servers []Server
}

func newLoadBalancer(port string, servers []Server) *loadBalancer {
	return &loadBalancer{
		port:    port,
		servers: servers,
	}
}

func (s *simpleServer) Address() string {
	return s.addr
}

func (s *simpleServer) IsAlive() bool {
	// Check if the server is alive by making a simple GET request
	timeout := 2 * time.Second
	client := http.Client{
		Timeout: timeout,
	}

	resp, err := client.Get(s.addr)
	if err != nil || resp.StatusCode != http.StatusOK {
		return false
	}
	return true
}

func (s *simpleServer) Serve(rw http.ResponseWriter, req *http.Request) {
	// Increment the connection count when a request is served
	s.IncrementConnection()
	defer s.DecrementConnection()

	start := time.Now()
	s.proxy.ServeHTTP(rw, req)
	duration := time.Since(start)

	// Update the average response time
	s.UpdateResponseTime(duration)
}

func (s *simpleServer) IncrementConnection() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.connections++
}

func (s *simpleServer) DecrementConnection() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.connections--
}

func (s *simpleServer) Connections() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.connections
}

func (s *simpleServer) UpdateResponseTime(duration time.Duration) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.requests++
	s.totalResponseTime += duration
}

func (s *simpleServer) AverageResponseTime() time.Duration {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.requests == 0 {
		return 0
	}
	return s.totalResponseTime / time.Duration(s.requests)
}

func (lb *loadBalancer) pickServer() Server {
	var selectedServer Server
	minResponseTime := time.Duration(^uint64(0) >> 1) // Initialize to max duration

	for _, server := range lb.servers {
		if server.IsAlive() {
			responseTime := server.AverageResponseTime()
			if responseTime < minResponseTime {
				minResponseTime = responseTime
				selectedServer = server
			}
		}
	}

	return selectedServer
}

func (lb *loadBalancer) serveProxy(rw http.ResponseWriter, req *http.Request) {
	targetServer := lb.pickServer()
	if targetServer == nil {
		http.Error(rw, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	log.Printf("Redirecting request to server: %s", targetServer.Address())
	targetServer.Serve(rw, req)
}

func main() {
	servers := []Server{
		newSimpleServer("https://www.facebook.com"),
		newSimpleServer("http://www.bing.com"),
		newSimpleServer("http://www.duckduckgo.com"),
	}

	lb := newLoadBalancer("8000", servers)
	handleRedirect := func(rw http.ResponseWriter, req *http.Request) {
		lb.serveProxy(rw, req)
	}
	http.HandleFunc("/", handleRedirect)

	log.Printf("Load Balancer serving at localhost:%s", lb.port)
	err := http.ListenAndServe(":"+lb.port, nil)
	handleErr(err)
}
