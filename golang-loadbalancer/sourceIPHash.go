package main

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"
)

type Server interface {
	Address() string
	IsAlive() bool
	Serve(rw http.ResponseWriter, req *http.Request)
}

type simpleServer struct {
	addr  string
	proxy *httputil.ReverseProxy
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
	s.proxy.ServeHTTP(rw, req)
}

func hashIP(ip string) uint32 {
	hash := md5.Sum([]byte(ip))
	return binary.BigEndian.Uint32(hash[:])
}

func (lb *loadBalancer) pickServer(ip string) Server {
	serverIndex := int(hashIP(ip)) % len(lb.servers)
	for !lb.servers[serverIndex].IsAlive() {
		serverIndex = (serverIndex + 1) % len(lb.servers)
	}
	return lb.servers[serverIndex]
}

func (lb *loadBalancer) serveProxy(rw http.ResponseWriter, req *http.Request) {
	ip := req.RemoteAddr
	targetServer := lb.pickServer(ip)
	if targetServer == nil {
		http.Error(rw, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	log.Printf("Redirecting request from IP %s to server: %s", ip, targetServer.Address())
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
