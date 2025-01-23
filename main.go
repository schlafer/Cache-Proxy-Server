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

type ProxyServer struct { //Represents the proxy server.
	targetHost string        //targetHost: The upstream server where requests are forwarded.
	cache      *Cache        //A Cache instance for storing responses.
	defaultTTL time.Duration //The default time-to-live (TTL) for cached data.
}

type Cache struct { //Stores cached data and handles cache operations.
	store map[string]CacheEntry //store: A map with keys (unique identifiers) and values (cached entries).
	mu    sync.RWMutex          //A mutex to ensure thread-safe access to the cache.
}

type CacheEntry struct { //Represents a single cache entry.

	Response []byte        //Response: The response body.
	Headers  http.Header   //Headers: HTTP headers for the response.
	TTL      time.Duration //TTL: Duration for which the entry is valid.
	Created  time.Time     //Created: Timestamp when the entry was cached.
}

func generateCacheKey(r *http.Request) string {
	/* Generates a unique cache key for each HTTP request.
	Combines the request URL and method, hashed using MD5.*/
	hasher := md5.New()
	io.WriteString(hasher, r.URL.String())
	io.WriteString(hasher, r.Method)
	return hex.EncodeToString(hasher.Sum(nil))
}

func (c *Cache) Get(cacheKey string) (CacheEntry, bool) {
	/* Fetches a cache entry if it exists and hasn’t expired. Deletes expired entries.*/
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
	// Stores a new cache entry.
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = cacheData
}

func (c *Cache) ClearCache() {
	//Clears all entries in the cache.
	c.mu.RLock()
	defer c.mu.RUnlock()
	for k := range c.store {
		delete(c.store, k)
	}
}

func (p *ProxyServer) handleProxy(w http.ResponseWriter, r *http.Request) {
	/*
		Handles incoming requests.
		First checks the cache for a response:
		If a cache hit occurs, the response is served directly with an X-Cache: HIT header.
		On a cache miss, the request is forwarded to the targetHost, and the response is cached for future requests.
		Responses include headers and the body from the upstream server.
	*/
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
	// A dedicated endpoint (/clear-cache) to clear all cached entries.
	p.cache.ClearCache()
	log.Println("Cache cleared")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Cache cleared"))
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
