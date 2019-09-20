package service

import (
	"context"

	"github.com/go-kit/kit/log"

	addsvcservice "github.com/cage1016/gokitconsulk8s/pkg/addsvc/service"
)

// Middleware describes a service (as opposed to endpoint) middleware.
type Middleware func(FoosvcService) FoosvcService

// Service describes a service that adds things together
// Implement yor service methods methods.
// e.x: Foo(ctx context.Context, s string)(rs string, err error)
type FoosvcService interface {
	Foo(ctx context.Context, s string) (res string, err error)
}

// the concrete implementation of service interface
type stubFoosvcService struct {
	logger log.Logger `json:"logger"`
	addsvc addsvcservice.AddsvcService
}

// New return a new instance of the service.
// If you want to add service middleware this is the place to put them.
func New(addsvc addsvcservice.AddsvcService, logger log.Logger) (s FoosvcService) {
	var svc FoosvcService
	{
		svc = &stubFoosvcService{logger: logger, addsvc: addsvc}
		svc = LoggingMiddleware(logger)(svc)
	}
	return svc
}

// Implement the business logic of Foo
func (fo *stubFoosvcService) Foo(ctx context.Context, s string) (res string, err error) {
	return fo.addsvc.Concat(ctx, s, "bar")
}
