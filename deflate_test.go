package deflate

import (
	"bytes"
	"compress/flate"
	"context"
	"deflate/compress"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/route"
	"github.com/stretchr/testify/assert"
)

const (
	testResponse = "Deflate Test Response"
)

func newServer() *route.Engine {
	router := route.NewEngine(config.NewOptions([]config.Option{}))
	router.Use(Deflate(DefaultCompression))
	router.GET("/", func(ctx context.Context, c *app.RequestContext) {
		c.Header("Content-Length", strconv.Itoa(len(testResponse)))
		c.String(200, testResponse)
	})
	return router
}

func TestDeflate(t *testing.T) {
	request := ut.PerformRequest(newServer(), consts.MethodGet, "/", nil, ut.Header{
		Key: "Accept-Encoding", Value: "deflate",
	})
	w := request.Result()
	assert.Equal(t, w.StatusCode(), 200)
	assert.Equal(t, w.Header.Get("Vary"), "Accept-Encoding")
	assert.Equal(t, w.Header.Get("Content-Encoding"), "deflate")
	assert.NotEqual(t, w.Header.Get("Content-Length"), "0")
	assert.NotEqual(t, len(w.Body()), 19)
	assert.Equal(t, fmt.Sprint(len(w.Body())), w.Header.Get("Content-Length"))
}

func TestWildcard(t *testing.T) {
	request := ut.PerformRequest(newServer(), consts.MethodGet, "/", nil, ut.Header{
		Key: "Accept-Encoding", Value: "*",
	})
	w := request.Result()
	assert.Equal(t, w.StatusCode(), 200)
	assert.Equal(t, w.Header.Get("Vary"), "Accept-Encoding")
	assert.Equal(t, w.Header.Get("Content-Encoding"), "deflate")
	assert.NotEqual(t, w.Header.Get("Content-Length"), "0")
	assert.NotEqual(t, len(w.Body()), 19)
	assert.Equal(t, fmt.Sprint(len(w.Body())), w.Header.Get("Content-Length"))
}

func TestDeflatePNG(t *testing.T) {
	router := route.NewEngine(config.NewOptions([]config.Option{}))
	router.Use(Deflate(DefaultCompression))
	router.GET("/image.png", func(ctx context.Context, c *app.RequestContext) {
		c.String(200, "this is a PNG!")
	})
	request := ut.PerformRequest(router, consts.MethodGet, "/image.png", nil, ut.Header{
		Key: "Accept-Encoding", Value: "deflate",
	})
	w := request.Result()
	assert.Equal(t, w.StatusCode(), 200)
	assert.Equal(t, w.Header.Get("Content-Encoding"), "")
	assert.Equal(t, w.Header.Get("Vary"), "")
	assert.Equal(t, string(w.Body()), "this is a PNG!")
}

func TestExcludedExtensions(t *testing.T) {
	router := route.NewEngine(config.NewOptions([]config.Option{}))
	router.Use(Deflate(DefaultCompression, WithExcludedExtensions([]string{".html"})))
	router.GET("/index.html", func(ctx context.Context, c *app.RequestContext) {
		c.String(200, "this is a HTML!")
	})
	request := ut.PerformRequest(router, consts.MethodGet, "/index.html", nil, ut.Header{
		Key: "Accept-Encoding", Value: "deflate",
	})
	w := request.Result()
	assert.Equal(t, http.StatusOK, w.StatusCode())
	assert.Equal(t, "", w.Header.Get("Content-Encoding"))
	assert.Equal(t, "", w.Header.Get("Vary"))
	assert.Equal(t, "this is a HTML!", string(w.Body()))
	assert.Equal(t, "15", w.Header.Get("Content-Length"))
}

func TestExcludedPaths(t *testing.T) {
	router := route.NewEngine(config.NewOptions([]config.Option{}))
	router.Use(Deflate(DefaultCompression, WithExcludedPaths([]string{"/api/"})))
	router.GET("/api/books", func(ctx context.Context, c *app.RequestContext) {
		c.String(200, "this is books!")
	})
	request := ut.PerformRequest(router, consts.MethodGet, "/api/books", nil, ut.Header{
		Key: "Accept-Encoding", Value: "deflate",
	})
	w := request.Result()
	assert.Equal(t, http.StatusOK, w.StatusCode())
	assert.Equal(t, "", w.Header.Get("Content-Encoding"))
	assert.Equal(t, "", w.Header.Get("Vary"))
	assert.Equal(t, "this is books!", string(w.Body()))
	assert.Equal(t, "14", w.Header.Get("Content-Length"))
}

func TestNoDeflate(t *testing.T) {
	request := ut.PerformRequest(newServer(), consts.MethodGet, "/", nil)
	w := request.Result()
	assert.Equal(t, w.StatusCode(), 200)
	assert.Equal(t, w.Header.Get("Content-Encoding"), "")
	assert.Equal(t, w.Header.Get("Content-Length"), "21")
	assert.Equal(t, string(w.Body()), testResponse)
}

func TestDecompressDeflate(t *testing.T) {
	buf := &bytes.Buffer{}
	gz := compress.AcquireStacklessDeflateWriter(buf, flate.DefaultCompression)
	if _, err := gz.Write([]byte(testResponse)); err != nil {
		gz.Close()
		t.Fatal(err)
	}
	gz.Close()
	router := route.NewEngine(config.NewOptions([]config.Option{}))
	router.Use(Deflate(DefaultCompression, WithDecompressFn(DefaultDecompressHandle)))
	router.POST("/", func(ctx context.Context, c *app.RequestContext) {
		if v := c.Request.Header.Get("Content-Encoding"); v != "" {
			t.Errorf("unexpected `Content-Encoding`: %s header", v)
		}
		if v := c.Request.Header.Get("Content-Length"); v != "" {
			t.Errorf("unexpected `Content-Length`: %s header", v)
		}
		data := c.GetRawData()
		c.Data(200, "text/plain", data)
	})
	request := ut.PerformRequest(router, consts.MethodPost, "/", &ut.Body{Body: buf, Len: buf.Len()}, ut.Header{
		Key: "Content-Encoding", Value: "deflate",
	})
	w := request.Result()
	assert.Equal(t, http.StatusOK, w.StatusCode())
	assert.Equal(t, "", w.Header.Get("Content-Encoding"))
	assert.Equal(t, "", w.Header.Get("Vary"))
	assert.Equal(t, testResponse, string(w.Body()))
	assert.Equal(t, "21", w.Header.Get("Content-Length"))
}

func TestDecompressDeflateWithEmptyBody(t *testing.T) {
	router := route.NewEngine(config.NewOptions([]config.Option{}))
	router.Use(Deflate(
		DefaultCompression, WithDecompressFn(DefaultDecompressHandle)))
	router.POST("/", func(ctx context.Context, c *app.RequestContext) {
		c.String(200, "ok")
	})

	request := ut.PerformRequest(router, consts.MethodPost, "/", nil,
		ut.Header{Key: "Content-Encoding", Value: "deflate"})
	w := request.Result()
	assert.Equal(t, http.StatusOK, w.StatusCode())
	assert.Equal(t, "", w.Header.Get("Content-Encoding"))
	assert.Equal(t, "", w.Header.Get("Vary"))
	assert.Equal(t, "ok", string(w.Body()))
	assert.Equal(t, "2", w.Header.Get("Content-Length"))
}

func TestDecompressDeflateWithSkipFunc(t *testing.T) {
	router := route.NewEngine(config.NewOptions([]config.Option{}))
	router.Use(Deflate(DefaultCompression, WithDecompressFn(DefaultDecompressHandle)))
	router.POST("/", func(ctx context.Context, c *app.RequestContext) {
		c.SetStatusCode(200)
	})

	request := ut.PerformRequest(router, consts.MethodPost, "/", nil,
		ut.Header{Key: "Accept-Encoding", Value: "deflate"})
	w := request.Result()
	assert.Equal(t, http.StatusOK, w.StatusCode())
	assert.Equal(t, "deflate", w.Header.Get("Content-Encoding"))
	assert.Equal(t, "Accept-Encoding", w.Header.Get("Vary"))
	assert.Equal(t, "", string(w.Body()))
	assert.Equal(t, "0", w.Header.Get("Content-Length"))
}

func TestDecompressDeflateWithIncorrectData(t *testing.T) {
	router := route.NewEngine(config.NewOptions([]config.Option{}))
	router.Use(Deflate(DefaultCompression, WithDecompressFn(DefaultDecompressHandle)))
	router.POST("/", func(ctx context.Context, c *app.RequestContext) {
		c.String(200, "ok")
	})
	reader := bytes.NewReader([]byte(testResponse))
	request := ut.PerformRequest(router, consts.MethodPost, "/", &ut.Body{Body: reader, Len: reader.Len()},
		ut.Header{Key: "Content-Encoding", Value: "deflate"})
	w := request.Result()
	assert.Equal(t, http.StatusBadRequest, w.StatusCode())
}

func TestDeflateForClient(t *testing.T) {
	h := server.Default(server.WithHostPorts("127.0.0.1:2333"))

	h.GET("/ping", func(ctx context.Context, c *app.RequestContext) {
		c.Header("Content-Length", strconv.Itoa(len(testResponse)))
		c.String(200, testResponse)
	})
	go h.Spin()
	time.Sleep(time.Second)

	cli, err := client.NewClient()
	if err != nil {
		panic(err)
	}
	cli.Use(DeflateForClient(DefaultCompression))

	req := protocol.AcquireRequest()
	res := protocol.AcquireResponse()

	req.SetBodyString("bar")
	req.SetRequestURI("http://127.0.0.1:2333/ping")

	err = cli.Do(context.Background(), req, res)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	assert.Equal(t, res.StatusCode(), 200)
	assert.Equal(t, req.Header.Get("Vary"), "Accept-Encoding")
	assert.Equal(t, req.Header.Get("Content-Encoding"), "deflate")
	assert.NotEqual(t, req.Header.Get("Content-Length"), "0")
	assert.NotEqual(t, fmt.Sprint(len(req.Body())), req.Header.Get("Content-Length"))
}

func TestDeflatePNGForClient(t *testing.T) {
	h := server.Default(server.WithHostPorts("127.0.0.1:2334"))

	h.GET("/image.png", func(ctx context.Context, c *app.RequestContext) {
		c.Header("Content-Length", strconv.Itoa(len(testResponse)))
		c.String(200, testResponse)
	})
	go h.Spin()
	time.Sleep(time.Second)

	cli, err := client.NewClient()
	if err != nil {
		panic(err)
	}
	cli.Use(DeflateForClient(DefaultCompression, WithExcludedExtensionsForClient([]string{".png"})))

	req := protocol.AcquireRequest()
	res := protocol.AcquireResponse()

	req.SetBodyString("bar")
	req.SetRequestURI("http://127.0.0.1:2334/image.png")

	err = cli.Do(context.Background(), req, res)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	assert.Equal(t, res.StatusCode(), 200)
	assert.Equal(t, req.Header.Get("Vary"), "")
	assert.Equal(t, req.Header.Get("Content-Encoding"), "")
}

func TestExcludedExtensionsForClient(t *testing.T) {
	h := server.Default(server.WithHostPorts("127.0.0.1:3333"))

	h.GET("/index.html", func(ctx context.Context, c *app.RequestContext) {
		c.Header("Content-Length", strconv.Itoa(len(testResponse)))
		c.String(200, testResponse)
	})
	go h.Spin()
	time.Sleep(time.Second)

	cli, err := client.NewClient()
	if err != nil {
		panic(err)
	}
	cli.Use(DeflateForClient(DefaultCompression, WithExcludedExtensionsForClient([]string{".html"})))

	req := protocol.AcquireRequest()
	res := protocol.AcquireResponse()

	req.SetBodyString("bar")
	req.SetRequestURI("http://127.0.0.1:3333/index.html")

	err = cli.Do(context.Background(), req, res)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	assert.Equal(t, res.StatusCode(), 200)
	assert.Equal(t, req.Header.Get("Vary"), "")
	assert.Equal(t, req.Header.Get("Content-Encoding"), "")
}

func TestExcludedPathsForClient(t *testing.T) {
	h := server.Default(server.WithHostPorts("127.0.0.1:2336"))

	h.GET("/api/books", func(ctx context.Context, c *app.RequestContext) {
		c.Header("Content-Length", strconv.Itoa(len(testResponse)))
		c.String(200, testResponse)
	})
	go h.Spin()
	time.Sleep(time.Second)

	cli, err := client.NewClient()
	if err != nil {
		panic(err)
	}
	cli.Use(DeflateForClient(DefaultCompression, WithExcludedPathsForClient([]string{"/api/"})))

	req := protocol.AcquireRequest()
	res := protocol.AcquireResponse()

	req.SetBodyString("bar")
	req.SetRequestURI("http://127.0.0.1:2336/api/books")

	err = cli.Do(context.Background(), req, res)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	assert.Equal(t, res.StatusCode(), 200)
	assert.Equal(t, req.Header.Get("Vary"), "")
	assert.Equal(t, req.Header.Get("Content-Encoding"), "")
}

func TestNoDeflateForClient(t *testing.T) {
	h := server.Default(server.WithHostPorts("127.0.0.1:2337"))

	h.GET("/", func(ctx context.Context, c *app.RequestContext) {
		c.Header("Content-Length", strconv.Itoa(len(testResponse)))
		c.String(200, testResponse)
	})
	go h.Spin()

	time.Sleep(time.Second)

	cli, err := client.NewClient()
	if err != nil {
		panic(err)
	}
	req := protocol.AcquireRequest()
	res := protocol.AcquireResponse()

	req.SetBodyString("bar")
	req.SetRequestURI("http://127.0.0.1:2337/")

	err = cli.Do(context.Background(), req, res)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	assert.Equal(t, res.StatusCode(), 200)
	assert.Equal(t, req.Header.Get("Content-Encoding"), "")
	assert.Equal(t, req.Header.Get("Content-Length"), "3")
}

func TestDecompressDeflateForClient(t *testing.T) {
	h := server.Default(server.WithHostPorts("127.0.0.1:2338"))
	h.Use(Deflate(DefaultCompression, WithDecompressFn(DefaultDecompressHandle)))
	h.GET("/", func(ctx context.Context, c *app.RequestContext) {
		c.Header("Content-Length", strconv.Itoa(len(testResponse)))
		c.String(200, testResponse)
	})

	go h.Spin()

	time.Sleep(time.Second)

	cli, err := client.NewClient()
	if err != nil {
		panic(err)
	}
	cli.Use(DeflateForClient(DefaultCompression, WithDecompressFnForClient(DefaultDecompressMiddlewareForClient)))

	req := protocol.AcquireRequest()
	res := protocol.AcquireResponse()

	req.SetBodyString("bar")
	req.SetRequestURI("http://127.0.0.1:2338/")
	req.SetHeader("Accept-Encoding", "deflate")

	err = cli.Do(context.Background(), req, res)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	assert.Equal(t, res.StatusCode(), 200)
	assert.Equal(t, res.Header.Get("Content-Encoding"), "")
	assert.Equal(t, res.Header.Get("Vary"), "")
	assert.Equal(t, testResponse, string(res.Body()))
	assert.Equal(t, "21", res.Header.Get("Content-Length"))
}
