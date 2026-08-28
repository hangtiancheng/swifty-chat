// Copyright (c) 2026 hangtiancheng
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package handler

import (
	"github.com/hangtiancheng/swifty.go/swifty_http"
)

func JsonBack(ctx *swifty_http.Context, message string, ret int, data any) {
	ctx.Status = 200
	switch ret {
	case 0:
		resp := swifty_http.H{"code": 200, "message": message}
		if data != nil {
			resp["data"] = data
		}
		ctx.JSON(resp)
	case -2:
		ctx.JSON(swifty_http.H{"code": 400, "message": message})
	default:
		ctx.JSON(swifty_http.H{"code": 500, "message": message})
	}
}

// JsonStatus writes an explicit envelope code, for auth failures that the
// ret convention cannot express.
func JsonStatus(ctx *swifty_http.Context, code int, message string) {
	ctx.Status = 200
	ctx.JSON(swifty_http.H{"code": code, "message": message})
}
