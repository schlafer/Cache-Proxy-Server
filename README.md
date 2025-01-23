# Caching Proxy Server

## Overview

A basic CLI tool that starts a caching proxy server, it will forward requests to the actual server and cache the responses. If the same request is made again, it will return the cached response instead of forwarding the request to the server.
[](https://roadmap.sh/projects/caching-server)
## Installation

1. Clone this repo locally & move into the cloned folder
2. Run the server using
```bash
 go run main.go --target={} --port={} --ttl={}
```
Eg. 
```bash
go run main.go -target=https://dummyjson.com -port=8080 -ttl=5m
```
Send a GET request to "http://localhost:8080/clear-cache" to clear all cache entries.

##HTTP Request Flow

1. A client sends a request to the proxy server.
2. The server computes a cache key using generateCacheKey.
3. The cache is checked:
-   If a valid cache entry is found:
        - The cached response is served.
-   If no valid cache entry exists:
        - The proxy forwards the request to the target server (targetHost).
        - The upstream server's response is cached for future use.
4. The server responds to the client with the data.

## Program Flow

1. Startup
-   Command-line arguments are parsed:
        - port: Port for the proxy server.
        - target: The upstream server (e.g., http://example.com).
        - ttl: TTL for cache entries (e.g., 5m for 5 minutes).
-   The ProxyServer and Cache are initialized.
2. Endpoints

- /: Handles proxy requests.
- /clear-cache: Clears the cache.
3. Main Function

-   Starts the HTTP server on the specified port.
-   Logs server startup and configuration details.


## License

[MIT](https://choosealicense.com/licenses/mit/)
