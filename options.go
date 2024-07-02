package deflate

import (
	"bytes"
	"context"
	"deflate/compress"
	"net/http"
	"regexp"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/protocol"
)

var (
	DefaultExcludedExtensions = NewExcludedExtensions([]string{
		".png", ".gif", ".jpeg", ".jpg",
	})
	DefaultOptions = &Options{
		ExcludedExtensions: DefaultExcludedExtensions,
	}
	DefaultClientExcludedExtensions = NewExcludedExtensions([]string{
		".png", ".gif", ".jpeg", ".jpg",
	})
	DefaultClientOptions = &ClientOptions{
		ExcludedExtensions: DefaultExcludedExtensions,
	}
)

type (
	Options struct {
		ExcludedExtensions  ExcludedExtensions
		ExcludedPaths       ExcludedPaths
		ExcludedPathRegexes ExcludedPathRegexes
		DecompressFn        app.HandlerFunc
	}
	ClientOptions struct {
		ExcludedExtensions    ExcludedExtensions
		ExcludedPaths         ExcludedPaths
		ExcludedPathRegexes   ExcludedPathRegexes
		DecompressFnForClient client.Middleware
	}
	Option       func(*Options)
	ClientOption func(*ClientOptions)

	ExcludedExtensions  map[string]bool
	ExcludedPaths       []string
	ExcludedPathRegexes []*regexp.Regexp
)

// WithExcludedExtensions customize excluded extensions
func WithExcludedExtensions(args []string) Option {
	return func(o *Options) {
		o.ExcludedExtensions = NewExcludedExtensions(args)
	}
}

// WithExcludedPathRegexes customize paths' regexes
func WithExcludedPathRegexes(args []string) Option {
	return func(o *Options) {
		o.ExcludedPathRegexes = NewExcludedPathRegexes(args)
	}
}

// WithExcludedPathsRegexs customize path's regexes
// NOTE: WithExcludedPathRegexs is exactly same as WithExcludedPathRegexes, this just for aligning with gin
func WithExcludedPathsRegexs(args []string) Option {
	return func(o *Options) {
		o.ExcludedPathRegexes = NewExcludedPathRegexes(args)
	}
}

func WithExcludedPaths(args []string) Option {
	return func(o *Options) {
		o.ExcludedPaths = NewExcludedPaths(args)
	}
}

func WithDecompressFn(decompressFn app.HandlerFunc) Option {
	return func(o *Options) {
		o.DecompressFn = decompressFn
	}
}

func WithDecompressFnForClient(decompressFnForClient client.Middleware) ClientOption {
	return func(o *ClientOptions) {
		o.DecompressFnForClient = decompressFnForClient
	}
}

// WithExcludedExtensionsForClient customize excluded extensions
func WithExcludedExtensionsForClient(args []string) ClientOption {
	return func(o *ClientOptions) {
		o.ExcludedExtensions = NewExcludedExtensions(args)
	}
}

// WithExcludedPathRegexesForClient customize paths' regexes
func WithExcludedPathRegexesForClient(args []string) ClientOption {
	return func(o *ClientOptions) {
		o.ExcludedPathRegexes = NewExcludedPathRegexes(args)
	}
}

func WithExcludedPathsForClient(args []string) ClientOption {
	return func(o *ClientOptions) {
		o.ExcludedPaths = NewExcludedPaths(args)
	}
}

func NewExcludedPaths(paths []string) ExcludedPaths {
	return ExcludedPaths(paths)
}

func NewExcludedExtensions(extensions []string) ExcludedExtensions {
	res := make(ExcludedExtensions)
	for _, e := range extensions {
		res[e] = true
	}
	return res
}

func NewExcludedPathRegexes(regexes []string) ExcludedPathRegexes {
	result := make([]*regexp.Regexp, len(regexes))
	for i, reg := range regexes {
		result[i] = regexp.MustCompile(reg)
	}
	return result
}

func (e ExcludedPathRegexes) Contains(requestURI string) bool {
	for _, reg := range e {
		if reg.MatchString(requestURI) {
			return true
		}
	}
	return false
}

func (e ExcludedExtensions) Contains(target string) bool {
	_, ok := e[target]
	return ok
}

func (e ExcludedPaths) Contains(requestURI string) bool {
	for _, path := range e {
		if strings.HasPrefix(requestURI, path) {
			return true
		}
	}
	return false
}

func DefaultDecompressHandle(ctx context.Context, c *app.RequestContext) {
	if len(c.Request.Body()) <= 0 {
		return
	}
	inflateBytes, err := compress.AppendInflateBytes(nil, c.Request.Body())
	if err != nil {
		_ = c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	c.Request.Header.DelBytes([]byte("Content-Encoding"))
	c.Request.Header.DelBytes([]byte("Content-Length"))
	c.Request.SetBody(inflateBytes)
}

func DefaultDecompressMiddlewareForClient(next client.Endpoint) client.Endpoint {
	return func(ctx context.Context, req *protocol.Request, resp *protocol.Response) (err error) {
		if len(resp.Body()) <= 0 {
			return
		}
		inflateBytes, err := compress.AppendInflateBytes(nil, resp.Body())
		if err != nil {
			return err
		}
		resp.Header.DelBytes([]byte("Content-Encoding"))
		resp.Header.DelBytes([]byte("Content-Length"))
		resp.Header.DelBytes([]byte("Vary"))
		resp.SetBodyStream(bytes.NewBuffer(inflateBytes), len(inflateBytes))
		return nil
	}
}
