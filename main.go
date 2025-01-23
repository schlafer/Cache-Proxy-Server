package main

import (
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

type ProxyServer struct {
	targetHost string
	cache      *Cache
	defaultTTL time.Duration
}

type Cache struct {
	store map[string]CacheEntry
	mu    sync.RWMutex
}

type CacheEntry struct {
	Response []byte
	Headers  http.Header
	TTL      time.Duration
	Created  time.Time
}

func generateCacheKey(r *http.Request) string {
	hasher := md5.New()
	io.WriteString(hasher, r.URL.String())
	io.WriteString(hasher, r.Method)
	return hex.EncodeToString(hasher.Sum(nil))
}

func (c *Cache) Get(cacheKey string) (CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, found := c.store[cacheKey]
	if !found {
		return CacheEntry{}, false
	}
	if time.Since(entry.Created) > entry.TTL {
		delete(c.store, cacheKey)
		return CacheEntry{}, false
	}
	return entry, true
}

func (c *Cache) Set(key string, cacheData CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = cacheData
}

func (p *ProxyServer) handleProxy(w http.ResponseWriter, r *http.Request) {
	key := generateCacheKey(r)
	if entry, found := p.cache.Get(key); found {
		log.Printf("Cache hit for %s", r.URL.Path)
		w.Header().Add("X-Cache", "HIT")
		for k, v := range entry.Headers {
			w.Header()[k] = v
		}
		w.Write(entry.Response)
		return
	}
	w.Header().Add("X-Cache", "MISS")
	log.Printf("Cache miss for %s", r.URL.Path)
	client := &http.Client{}

	targetUrl := p.targetHost + r.URL.Path

	if r.URL.RawQuery != "" {
		targetUrl += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequest(r.Method, targetUrl, r.Body)
	if err != nil {
		http.Error(w, "Error while creating request", http.StatusInternalServerError)
	}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Error while sending request", http.StatusInternalServerError)
	}

	for header, values := range r.Header {
		for _, val := range values {
			req.Header.Add(header, val)
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Error while reading body", http.StatusInternalServerError)
	}
	p.cache.Set(key, CacheEntry{
		Response: body,
		Headers:  req.Header,
		Created:  time.Now(),
		TTL:      p.defaultTTL,
	})

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.Write([]byte(body))
}

func (p *ProxyServer) clearCacheHandler(w http.ResponseWriter, r *http.Request) {
	p.cache.ClearCache()
	log.Println("Cache cleared")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Cache cleared"))
}

func (c *Cache) ClearCache() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for k := range c.store {
		delete(c.store, k)
	}
}

func main() {
	// Port for the server & Target URL where the requests should be forwarded
	port := flag.Int("port", 8080, "")
	targetHost := flag.String("target", "", "Requests to be forwarded on the server")
	ttl := flag.String("ttl", "5m", "Time to live for cached data")
	flag.Parse()

	if *targetHost == "" {
		log.Fatal("Target host is required")
	}

	duration, _ := time.ParseDuration(*ttl)

	p := &ProxyServer{
		targetHost: *targetHost,
		cache: &Cache{
			store: map[string]CacheEntry{},
		},
		defaultTTL: duration,
	}

	log.Printf("Starting proxy server on port %d", *port)
	log.Printf("Proxying requests to %s", *targetHost)

	http.HandleFunc("/", p.handleProxy)
	http.HandleFunc("/clear-cache", p.clearCacheHandler)

	serverPort := fmt.Sprintf(":%d", *port)
	log.Fatal(http.ListenAndServe(serverPort, nil))
}
