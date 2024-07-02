package deflate

import (
	"compress/flate"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/client"
)

const (
	BestCompression    = flate.BestCompression
	BestSpeed          = flate.BestSpeed
	DefaultCompression = flate.DefaultCompression
	NoCompression      = flate.NoCompression
)

func Deflate(level int, options ...Option) app.HandlerFunc {
	return newDeflateSrvMiddleware(level, options...).SrvMiddleware
}

func DeflateForClient(level int, options ...ClientOption) client.Middleware {
	return newDeflateClientMiddleware(level, options...).ClientMiddleware
}
