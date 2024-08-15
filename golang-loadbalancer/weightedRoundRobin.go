package main

import (
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
	Weight() int
}

type simpleServer struct {
	addr   string
	proxy  *httputil.ReverseProxy
	weight int
}

func newSimpleServer(addr string, weight int) *simpleServer {
	serveUrl, err := url.Parse(addr)
	handleErr(err)

	return &simpleServer{
		addr:   addr,
		proxy:  httputil.NewSingleHostReverseProxy(serveUrl),
		weight: weight,
	}
}

func handleErr(err error) {
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
}

type loadBalancer struct {
	port           string
	currentWeight  int
	currentServer  int
	servers        []Server
	weightCounters []int
}

func newLoadBalancer(port string, servers []Server) *loadBalancer {
	// Initialize weight counters for each server
	weightCounters := make([]int, len(servers))
	for i, server := range servers {
		weightCounters[i] = server.Weight()
	}
	return &loadBalancer{
		port:           port,
		currentWeight:  0,
		currentServer:  0,
		servers:        servers,
		weightCounters: weightCounters,
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

func (s *simpleServer) Weight() int {
	return s.weight
}

func (lb *loadBalancer) pickServer() Server {
	for {
		lb.currentServer = (lb.currentServer + 1) % len(lb.servers)
		if lb.currentServer == 0 {
			lb.currentWeight = lb.currentWeight - 1
			if lb.currentWeight <= 0 {
				lb.currentWeight = maxWeight(lb.servers)
				if lb.currentWeight == 0 {
					log.Println("All servers are down")
					return nil
				}
			}
		}

		if lb.weightCounters[lb.currentServer] >= lb.currentWeight && lb.servers[lb.currentServer].IsAlive() {
			return lb.servers[lb.currentServer]
		}
	}
}

func maxWeight(servers []Server) int {
	max := 0
	for _, server := range servers {
		if server.Weight() > max {
			max = server.Weight()
		}
	}
	return max
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
		newSimpleServer("https://www.facebook.com", 5),
		newSimpleServer("http://www.bing.com", 3),
		newSimpleServer("http://www.duckduckgo.com", 1),
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
