package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/rehttp"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/gregjones/httpcache"
	"github.com/gregjones/httpcache/diskcache"
	"github.com/peterbourgon/diskv"
)

var (
	bind              = flag.String("bind", "127.0.0.1:7643", "address and port to bind to")
	backend           = flag.String("backend", "", "uri of backend storage service, e.g. s3://my-bazel-cache/prefix")
	cacheDir          = flag.String("cache-dir", "", "local cache directory")
	cacheSize         = flag.Uint64("cache-size", 5, "local cache size in gigabytes")
	cacheRefreshDelay = flag.Int64("cache-refresh-delay", 60, "the delay in minutes after which missed requests will be retried in the next cache tier")
	allowWrites       = flag.Bool("allow-writes", false, "allow PUT requests to the cache")
	maxRetries        = flag.Int("max-retries", 1, "maximum number of retries when hitting the next cache tier")
)

type cachingTransport struct {
	Transport http.RoundTripper
	Cache     httpcache.Cache
}

// cachingReadCloser is a wrapper around ReadCloser R that calls OnEOF
// handler with a full copy of the content read from R when EOF is
// reached.
type cachingReadCloser struct {
	// Underlying ReadCloser.
	R io.ReadCloser
	// OnEOF is called with a copy of the content of R when EOF is reached.
	OnEOF func(io.Reader)

	buf bytes.Buffer // buf stores a copy of the content of R.
}

// Read reads the next len(p) bytes from R or until R is drained. The
// return value n is the number of bytes read. If R has no data to
// return, err is io.EOF and OnEOF is called with a full copy of what
// has been read so far.
func (r *cachingReadCloser) Read(p []byte) (n int, err error) {
	n, err = r.R.Read(p)
	r.buf.Write(p[:n])
	if err == io.EOF {
		r.OnEOF(bytes.NewReader(r.buf.Bytes()))
	}
	return n, err
}

func (r *cachingReadCloser) Close() error {
	return r.R.Close()
}

func cachedResponse(c httpcache.Cache, cacheKey string, req *http.Request) (resp *http.Response, err error) {
	cachedVal, ok := c.Get(cacheKey)
	if !ok {
		return
	}

	b := bytes.NewBuffer(cachedVal)
	return http.ReadResponse(bufio.NewReader(b), req)
}

func newDroppedResponse(req *http.Request) *http.Response {
	var braw bytes.Buffer
	braw.WriteString("HTTP/1.1 405 Method Not Allowed\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(&braw), req)
	if err != nil {
		panic(err)
	}
	return resp
}

func (t *cachingTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	cacheKey := req.URL.Path
	var logAction string

	defer func() {
		if err != nil {
			logAction = "ERROR"
		}
		status := ""
		if resp != nil {
			status = fmt.Sprintf("%d", resp.StatusCode)
		}
		log.Printf("%-15s %3s %15s %4s %s", logAction, status, req.URL.Host, req.Method, req.URL.Path)
	}()

	doCache := req.Method == http.MethodGet && t.Cache != nil

	if doCache {
		resp, err = cachedResponse(t.Cache, cacheKey, req)
		if err != nil {
			fmt.Printf("Failed to read cached response")
		} else if resp != nil {
			var cachedAt time.Time
			cachedAt, err = http.ParseTime(resp.Header["Date"][0])
			if err != nil {
				log.Fatalf("Failed to parse cached request Date: %v", err)
			}
			// We never discard successful cache entries, but if we periodically
			// re-check failed cache entries in case an artifact has been made
			// available in the upstream cache tier.
			old := cachedAt.Add(time.Duration(*cacheRefreshDelay) * time.Minute).Before(time.Now().UTC())
			if resp.StatusCode == http.StatusOK || !old {
				logAction = "CACHE_HIT"
				return
			}
			logAction = "CACHE_REFRESH"
			t.Cache.Delete(cacheKey)
		} else {
			logAction = "CACHE_MISS"
		}
	} else if (req.Method == http.MethodPut && *allowWrites) || req.Method == http.MethodGet {
		logAction = "PASSTHROUGH"
	} else {
		logAction = "DROP"
		resp = newDroppedResponse(req)
		return
	}

	resp, err = t.Transport.RoundTrip(req)

	if err == nil && doCache {
		// Delay caching until EOF is reached.
		resp.Body = &cachingReadCloser{
			R: resp.Body,
			OnEOF: func(r io.Reader) {
				resp := *resp
				resp.Body = ioutil.NopCloser(r)
				respBytes, err := httputil.DumpResponse(&resp, true)
				if err == nil {
					t.Cache.Set(cacheKey, respBytes)
				}
			},
		}
	}
	return
}

func main() {
	flag.Parse()

	if *backend == "" {
		flag.Usage()
		os.Exit(1)
	}

	backendURL, err := url.Parse(*backend)
	if err != nil {
		log.Fatalf("invalid uri: %v (%v)", *backend, err)
	}

	var handler *httputil.ReverseProxy
	switch backendURL.Scheme {
	case "s3":
		d := newS3Director(session.New(), backendURL)
		handler = &httputil.ReverseProxy{
			Director: d.Direct,
		}
	default:
		handler = httputil.NewSingleHostReverseProxy(backendURL)
	}

	baseTransport := http.DefaultTransport
	if *maxRetries > 1 {
		baseTransport = rehttp.NewTransport(
			baseTransport,
			rehttp.RetryMaxRetries(*maxRetries), // Retry for ALL types of errors!
			rehttp.ExpJitterDelay(500*time.Millisecond, 10*time.Second),
		)

	}
	var cache httpcache.Cache
	if *cacheDir != "" {
		diskKeyValueStore := diskv.New(diskv.Options{
			BasePath:     *cacheDir,
			CacheSizeMax: *cacheSize * 1024 * 1024 * 1024,
		})
		cache = diskcache.NewWithDiskv(diskKeyValueStore)
	}

	handler.Transport = &cachingTransport{
		Cache:     cache,
		Transport: http.DefaultTransport,
	}

	addr := *bind
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	s := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	log.Printf("Listening on %s", addr)
	log.Fatal(s.ListenAndServe())
}
