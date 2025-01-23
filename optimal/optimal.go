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
	client     *http.Client
}

type Cache struct {
	store    map[string]CacheEntry
	mu       sync.RWMutex
	maxSize  int
	eviction chan string
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

// Get retrieves a cache entry if it exists and hasn't expired.
func (c *Cache) Get(cacheKey string) (CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, found := c.store[cacheKey]
	if !found || time.Since(entry.Created) > entry.TTL {
		if found {
			delete(c.store, cacheKey)
		}
		return CacheEntry{}, false
	}
	return entry, true
}

// Set adds a new entry to the cache and ensures size limits are maintained.
func (c *Cache) Set(key string, cacheData CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.store) >= c.maxSize {
		// Evict the oldest entry
		oldestKey := <-c.eviction
		delete(c.store, oldestKey)
	}

	c.store[key] = cacheData
	c.eviction <- key
}

// ClearCache clears all entries from the cache.
func (c *Cache) ClearCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store = make(map[string]CacheEntry)
	for len(c.eviction) > 0 {
		<-c.eviction
	}
}

// handleProxy handles incoming requests and serves cached or forwarded responses.
func (p *ProxyServer) handleProxy(w http.ResponseWriter, r *http.Request) {
	key := generateCacheKey(r)

	// Check the cache
	if entry, found := p.cache.Get(key); found {
		log.Printf("Cache hit for %s", r.URL.Path)
		w.Header().Set("X-Cache", "HIT")
		copyHeaders(entry.Headers, w.Header())
		w.Write(entry.Response)
		return
	}

	// Cache miss
	log.Printf("Cache miss for %s", r.URL.Path)
	w.Header().Set("X-Cache", "MISS")

	// Forward request
	targetURL := p.targetHost + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	copyHeaders(r.Header, req.Header)

	resp, err := p.client.Do(req)
	if err != nil {
		http.Error(w, "Failed to forward request", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response body", http.StatusInternalServerError)
		return
	}

	// Cache the response
	p.cache.Set(key, CacheEntry{
		Response: body,
		Headers:  resp.Header,
		Created:  time.Now(),
		TTL:      p.defaultTTL,
	})

	copyHeaders(resp.Header, w.Header())
	w.Write(body)
}

// clearCacheHandler clears the cache via an HTTP endpoint.
func (p *ProxyServer) clearCacheHandler(w http.ResponseWriter, r *http.Request) {
	p.cache.ClearCache()
	log.Println("Cache cleared")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Cache cleared"))
}

// Utility function to copy headers
func copyHeaders(src, dst http.Header) {
	for k, v := range src {
		dst[k] = v
	}
}

func main() {
	// Parse command-line arguments
	port := flag.Int("port", 8080, "Port to run the proxy server on")
	targetHost := flag.String("target", "", "Upstream server to proxy requests to")
	ttl := flag.String("ttl", "5m", "Time to live for cached entries")
	cacheSize := flag.Int("cache-size", 100, "Maximum number of cache entries")
	flag.Parse()

	if *targetHost == "" {
		log.Fatal("Target host is required")
	}

	duration, err := time.ParseDuration(*ttl)
	if err != nil {
		log.Fatalf("Invalid TTL duration: %v", err)
	}

	cache := &Cache{
		store:    make(map[string]CacheEntry),
		eviction: make(chan string, *cacheSize),
		maxSize:  *cacheSize,
	}

	proxy := &ProxyServer{
		targetHost: *targetHost,
		cache:      cache,
		defaultTTL: duration,
		client:     &http.Client{Timeout: 10 * time.Second},
	}

	log.Printf("Starting proxy server on port %d", *port)
	log.Printf("Proxying requests to %s", *targetHost)

	http.HandleFunc("/", proxy.handleProxy)
	http.HandleFunc("/clear-cache", proxy.clearCacheHandler)

	serverAddr := fmt.Sprintf(":%d", *port)
	log.Fatal(http.ListenAndServe(serverAddr, nil))
}
