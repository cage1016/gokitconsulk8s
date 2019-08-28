package service

import (
	"context"

	"github.com/go-kit/kit/log"
)

// Middleware describes a service (as opposed to endpoint) middleware.
type Middleware func(AddsvcService) AddsvcService

// Service describes a service that adds things together
// Implement yor service methods methods.
// e.x: Foo(ctx context.Context, s string)(rs string, err error)
type AddsvcService interface {
	Sum(ctx context.Context, a int64, b int64) (rs int64, err error)
	Concat(ctx context.Context, a string, b string) (rs string, err error)
}

// the concrete implementation of service interface
type stubAddsvcService struct {
	logger log.Logger `json:"logger"`
}

// New return a new instance of the service.
// If you want to add service middleware this is the place to put them.
func New(logger log.Logger) (s AddsvcService) {
	var svc AddsvcService
	{
		svc = &stubAddsvcService{logger: logger}
		svc = LoggingMiddleware(logger)(svc)
	}
	return svc
}

// Implement the business logic of Sum
func (ad *stubAddsvcService) Sum(ctx context.Context, a int64, b int64) (rs int64, err error) {
	return a + b, err
}

// Implement the business logic of Concat
func (ad *stubAddsvcService) Concat(ctx context.Context, a string, b string) (rs string, err error) {
	return a + b, err
}
