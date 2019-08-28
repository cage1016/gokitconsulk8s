package endpoints

import (
	"net/http"
	
	httptransport "github.com/go-kit/kit/transport/http"
)

var (
	_ httptransport.Headerer = (*FooResponse)(nil)

	_ httptransport.StatusCoder = (*FooResponse)(nil)
)

// FooResponse collects the response values for the Foo method.
type FooResponse struct {
	Res string `json:"res"`
	Err error  `json:"err"`
}

func (r FooResponse) StatusCode() int {
	return http.StatusOK // TBA
}

func (r FooResponse) Headers() http.Header {
	return http.Header{}
}

