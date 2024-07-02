/*
 * Copyright 2022 CloudWeGo Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * The MIT License (MIT)
 *
 * Copyright (c) 2015-present Aliaksandr Valialkin, VertaMedia, Kirill Danshin, Erik Dubbelboer, FastHTTP Authors
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
 * THE SOFTWARE.
 *
 * This file may have been modified by CloudWeGo authors. All CloudWeGo
 * Modifications are Copyright 2022 CloudWeGo Authors.
 */

package compress

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"sync"

	"github.com/cloudwego/hertz/pkg/common/bytebufferpool"
	"github.com/cloudwego/hertz/pkg/common/stackless"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/cloudwego/hertz/pkg/network"
)

const CompressDefaultCompression = 6 // flate.DefaultCompression

func newCompressWriterPoolMap() []*sync.Pool {
	// Initialize pools for all the compression levels defined
	// in https://golang.org/pkg/compress/flate/#pkg-constants .
	// Compression levels are normalized with normalizeCompressLevel,
	// so the fit [0..11].
	var m []*sync.Pool
	for i := 0; i < 12; i++ {
		m = append(m, &sync.Pool{})
	}
	return m
}

type compressCtx struct {
	w     io.Writer
	p     []byte
	level int
}

type byteSliceWriter struct {
	b []byte
}

func (w *byteSliceWriter) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}

type byteSliceReader struct {
	b []byte
}

func (r *byteSliceReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}

// normalizes compression level into [0..11], so it could be used as an index
// in *PoolMap.
func normalizeCompressLevel(level int) int {
	// -2 is the lowest compression level - CompressHuffmanOnly
	// 9 is the highest compression level - CompressBestCompression
	if level < -2 || level > 9 {
		level = CompressDefaultCompression
	}
	return level + 2
}

func acquireFlateReader(r io.Reader) (io.ReadCloser, error) {
	v := flateReaderPool.Get()
	if v == nil {
		zr, err := zlib.NewReader(r)
		if err != nil {
			return nil, err
		}
		return zr, nil
	}
	zr := v.(io.ReadCloser)
	if err := resetFlateReader(zr, r); err != nil {
		return nil, err
	}
	return zr, nil
}

func releaseFlateReader(zr io.ReadCloser) {
	zr.Close()
	flateReaderPool.Put(zr)
}

func resetFlateReader(zr io.ReadCloser, r io.Reader) error {
	zrr, ok := zr.(zlib.Resetter)
	if !ok {
		// sanity check. should only be called with a zlib.Reader
		panic("BUG: zlib.Reader doesn't implement zlib.Resetter???")
	}
	return zrr.Reset(r, nil)
}

var flateReaderPool sync.Pool

// AppendDeflateBytesLevel appends deflated src to dst using the given
// compression level and returns the resulting dst.
//
// Supported compression levels are:
//
//   - CompressNoCompression
//   - CompressBestSpeed
//   - CompressBestCompression
//   - CompressDefaultCompression
//   - CompressHuffmanOnly
func AppendDeflateBytesLevel(dst, src []byte, level int) ([]byte, error) {
	w := &byteSliceWriter{dst}
	_, err := WriteDeflateLevel(w, src, level)
	return w.b, err
}

// WriteDeflateLevel writes deflated p to w using the given compression level
// and returns the number of compressed bytes written to w.
//
// Supported compression levels are:
//
//   - CompressNoCompression
//   - CompressBestSpeed
//   - CompressBestCompression
//   - CompressDefaultCompression
//   - CompressHuffmanOnly
func WriteDeflateLevel(w io.Writer, p []byte, level int) (int, error) {
	switch w.(type) {
	case *byteSliceWriter,
		*bytes.Buffer,
		*bytebufferpool.ByteBuffer:
		// These writers don't block, so we can just use stacklessWriteDeflate
		ctx := &compressCtx{
			w:     w,
			p:     p,
			level: level,
		}
		stacklessWriteDeflate(ctx)
		return len(p), nil
	default:
		zw := AcquireStacklessDeflateWriter(w, level)
		n, err := zw.Write(p)
		releaseStacklessDeflateWriter(zw, level)
		return n, err
	}
}

var (
	stacklessWriteDeflateOnce sync.Once
	stacklessWriteDeflateFunc func(ctx interface{}) bool
)

func stacklessWriteDeflate(ctx interface{}) {
	stacklessWriteDeflateOnce.Do(func() {
		stacklessWriteDeflateFunc = stackless.NewFunc(nonblockingWriteDeflate)
	})
	stacklessWriteDeflateFunc(ctx)
}

func nonblockingWriteDeflate(ctxv interface{}) {
	ctx := ctxv.(*compressCtx)
	zw := acquireRealDeflateWriter(ctx.w, ctx.level)

	zw.Write(ctx.p) //nolint:errcheck // no way to handle this error anyway

	releaseRealDeflateWriter(zw, ctx.level)
}

// WriteInflate writes inflated p to w and returns the number of uncompressed
// bytes written to w.
func WriteInflate(w io.Writer, p []byte) (int, error) {
	r := &byteSliceReader{p}
	zr, err := acquireFlateReader(r)
	if err != nil {
		return 0, err
	}
	zw := network.NewWriter(w)
	n, err := utils.CopyZeroAlloc(zw, zr)
	releaseFlateReader(zr)
	nn := int(n)
	if int64(nn) != n {
		return 0, fmt.Errorf("too much data inflated: %d", n)
	}
	return nn, err
}

// AppendInflateBytes appends inflated src to dst and returns the resulting dst.
func AppendInflateBytes(dst, src []byte) ([]byte, error) {
	w := &byteSliceWriter{dst}
	_, err := WriteInflate(w, src)
	return w.b, err
}

func AcquireStacklessDeflateWriter(w io.Writer, level int) stackless.Writer {
	nLevel := normalizeCompressLevel(level)
	p := stacklessDeflateWriterPoolMap[nLevel]
	v := p.Get()
	if v == nil {
		return stackless.NewWriter(w, func(w io.Writer) stackless.Writer {
			return acquireRealDeflateWriter(w, level)
		})
	}
	sw := v.(stackless.Writer)
	sw.Reset(w)
	return sw
}

func releaseStacklessDeflateWriter(sw stackless.Writer, level int) {
	sw.Close()
	nLevel := normalizeCompressLevel(level)
	p := stacklessDeflateWriterPoolMap[nLevel]
	p.Put(sw)
}

func acquireRealDeflateWriter(w io.Writer, level int) *zlib.Writer {
	nLevel := normalizeCompressLevel(level)
	p := realDeflateWriterPoolMap[nLevel]
	v := p.Get()
	if v == nil {
		zw, err := zlib.NewWriterLevel(w, level)
		if err != nil {
			// zlib.NewWriterLevel only errors for invalid
			// compression levels. Clamp it to be min or max.
			if level < zlib.HuffmanOnly {
				level = zlib.HuffmanOnly
			} else {
				level = zlib.BestCompression
			}
			zw, _ = zlib.NewWriterLevel(w, level)
		}
		return zw
	}
	zw := v.(*zlib.Writer)
	zw.Reset(w)
	return zw
}

func releaseRealDeflateWriter(zw *zlib.Writer, level int) {
	zw.Close()
	nLevel := normalizeCompressLevel(level)
	p := realDeflateWriterPoolMap[nLevel]
	p.Put(zw)
}

var (
	stacklessDeflateWriterPoolMap = newCompressWriterPoolMap()
	realDeflateWriterPoolMap      = newCompressWriterPoolMap()
)
