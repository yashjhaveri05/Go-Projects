// package main

// import (
// 	"fmt"
// 	"net/http"
// 	"net/http/httputil"
// 	"net/url"
// 	"os"
// )

// type Server interface {
// 	Address() string
// 	IsAlive() bool
// 	Serve(rw http.ResponseWriter, req *http.Request)
// }

// type simpleServer struct {
// 	addr string
// 	proxy *httputil.ReverseProxy
// }

// func newSimpleServer(addr string) *simpleServer {
// 	serveUrl, err := url.Parse(addr)
// 	handleErr(err)

// 	return &simpleServer{
// 		addr: addr,
// 		proxy: httputil.NewSingleHostReverseProxy(serveUrl),
// 	}
// }

// func handleErr(err error) {
// 	if err != nil {
// 		fmt.Println("error: %v \n", err)
// 		os.Exit(1)
// 	}
// }

// type loadBalancer struct {
// 	port string
// 	roundRobinIndex int
// 	servers []Server
// }

// func newLoadBalancer(port string, servers []Server) *loadBalancer {
// 	return &loadBalancer{
// 		port: port,
// 		roundRobinIndex: 0,
// 		servers: servers,
// 	}
// }

// func (s *simpleServer) Address() string {
// 	return s.addr
// }

// func (s *simpleServer) IsAlive() bool {
// 	return true
// }

// func (s *simpleServer) Serve(rw http.ResponseWriter, req *http.Request) {
// 	s.proxy.ServeHTTP(rw, req)
// }

// func (lb *loadBalancer) pickServer() Server {
// 	server := lb.servers[lb.roundRobinIndex%len(lb.servers)]
// 	for !server.IsAlive() {
// 		lb.roundRobinIndex = (lb.roundRobinIndex + 1) % len(lb.servers)
// 		server = lb.servers[lb.roundRobinIndex%len(lb.servers)]
// 	}
// 	lb.roundRobinIndex = (lb.roundRobinIndex + 1) % len(lb.servers)
// 	return server
// }

// func (lb *loadBalancer) serveProxy(rw http.ResponseWriter, req *http.Request) {
// 	targetServer := lb.pickServer()
// 	fmt.Println("Redirecting request to server: %s \n", targetServer.Address())
// 	targetServer.Serve(rw, req)
// }

// func main() {
//     servers := []Server{
// 		newSimpleServer("https://www.facebook.com"),
// 		newSimpleServer("http://www.bing.com"),
// 		newSimpleServer("http://www.duckduckgo.com"),
// 	}

// 	lb := newLoadBalancer("8000", servers)
// 	handleRedirect := func(rw http.ResponseWriter, req *http.Request) {
// 		lb.serveProxy(rw, req)
// 	}
// 	http.HandleFunc("/", handleRedirect)

// 	fmt.Printf("Load Balancer serving at localhost: %s \n", lb.port)
// 	http.ListenAndServe(":"+lb.port, nil)
// }
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
	port            string
	roundRobinIndex int
	servers         []Server
}

func newLoadBalancer(port string, servers []Server) *loadBalancer {
	return &loadBalancer{
		port:            port,
		roundRobinIndex: 0,
		servers:         servers,
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

func (lb *loadBalancer) pickServer() Server {
	startIndex := lb.roundRobinIndex
	for {
		server := lb.servers[lb.roundRobinIndex%len(lb.servers)]
		lb.roundRobinIndex = (lb.roundRobinIndex + 1) % len(lb.servers)

		if server.IsAlive() {
			return server
		}

		// All servers down, return nil
		if lb.roundRobinIndex == startIndex {
			log.Println("All servers are down")
			return nil
		}
	}
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
