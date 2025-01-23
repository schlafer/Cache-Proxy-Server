# Caching Proxy

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
go run main.go --target=https://dummyjson.com --port=8080 --ttl=10s
```

## Features
- Supports TTL(Time To Live)

## License

[MIT](https://choosealicense.com/licenses/mit/)
