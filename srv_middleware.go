package deflate

import (
	"bytes"
	"context"
	"deflate/compress"
	"path/filepath"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol"
)

type deflateSrvMiddleware struct {
	*Options
	level int
}

func newDeflateSrvMiddleware(level int, opts ...Option) *deflateSrvMiddleware {
	handler := &deflateSrvMiddleware{
		Options: DefaultOptions,
		level:   level,
	}
	for _, fn := range opts {
		fn(handler.Options)
	}
	return handler
}

func (d *deflateSrvMiddleware) SrvMiddleware(ctx context.Context, c *app.RequestContext) {
	if fn := d.DecompressFn; fn != nil && strings.EqualFold(c.Request.Header.Get("Content-Encoding"), "deflate") {
		fn(ctx, c)
	}
	if !d.shouldCompress(&c.Request) {
		return
	}

	c.Next(ctx)

	c.Header("Content-Encoding", "deflate")
	c.Header("Vary", "Accept-Encoding")
	if len(c.Response.Body()) > 0 {
		deflateBytes, err := compress.AppendDeflateBytesLevel(nil, c.Response.Body(), d.level)
		if err != nil {
			return
		}
		c.Response.SetBodyStream(bytes.NewBuffer(deflateBytes), len(deflateBytes))
	}
}

func (d *deflateSrvMiddleware) shouldCompress(req *protocol.Request) bool {
	if !(strings.Contains(req.Header.Get("Accept-Encoding"), "deflate") ||
		strings.TrimSpace(req.Header.Get("Accept-Encoding")) == "*") ||
		strings.Contains(req.Header.Get("Connection"), "Upgrade") ||
		strings.Contains(req.Header.Get("Accept"), "text/event-stream") {
		return false
	}

	path := string(req.URI().RequestURI())
	extension := filepath.Ext(path)
	if d.ExcludedExtensions.Contains(extension) {
		return false
	}

	if d.ExcludedPaths.Contains(path) {
		return false
	}
	if d.ExcludedPathRegexes.Contains(path) {
		return false
	}

	return true
}
