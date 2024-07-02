package deflate

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"

	"deflate/compress"
	"github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/protocol"
)

type deflateClientMiddleware struct {
	*ClientOptions
	level int
}

func newDeflateClientMiddleware(level int, opts ...ClientOption) *deflateClientMiddleware {
	middleware := &deflateClientMiddleware{
		ClientOptions: DefaultClientOptions,
		level:         level,
	}
	for _, fn := range opts {
		fn(middleware.ClientOptions)
	}
	return middleware
}

func (d *deflateClientMiddleware) ClientMiddleware(next client.Endpoint) client.Endpoint {
	return func(ctx context.Context, req *protocol.Request, resp *protocol.Response) (err error) {
		if !d.shouldCompress(req) {
			return
		}
		req.SetHeader("Content-Encoding", "deflate")
		req.SetHeader("Vary", "Accept-Encoding")
		if len(req.Body()) > 0 {
			gzipBytes, err1 := compress.AppendDeflateBytesLevel(nil, req.Body(), d.level)
			if err1 != nil {
				return
			}
			req.SetBodyStream(bytes.NewBuffer(gzipBytes), len(gzipBytes))
		}
		err = next(ctx, req, resp)
		if err != nil {
			return
		}
		if fn := d.DecompressFnForClient; fn != nil && strings.EqualFold(resp.Header.Get("Content-Encoding"), "deflate") {
			f := fn(next)
			err = f(ctx, req, resp)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func (d *deflateClientMiddleware) shouldCompress(req *protocol.Request) bool {
	if strings.Contains(req.Header.Get("Connection"), "Upgrade") ||
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
