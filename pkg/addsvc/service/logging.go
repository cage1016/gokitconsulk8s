package service

import (
	"context"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

type loggingMiddleware struct {
	logger log.Logger    `json:""`
	next   AddsvcService `json:""`
}

// LoggingMiddleware takes a logger as a dependency
// and returns a ServiceMiddleware.
func LoggingMiddleware(logger log.Logger) Middleware {
	return func(next AddsvcService) AddsvcService {
		return loggingMiddleware{level.Info(logger), next}
	}
}

func (lm loggingMiddleware) Sum(ctx context.Context, a int64, b int64) (rs int64, err error) {
	defer func(begin time.Time) {
		lm.logger.Log("method", "Sum", "a", a, "b", b, "err", err)
	}(time.Now())

	return lm.next.Sum(ctx, a, b)
}

func (lm loggingMiddleware) Concat(ctx context.Context, a string, b string) (rs string, err error) {
	defer func(begin time.Time) {
		lm.logger.Log("method", "Concat", "a", a, "b", b, "err", err)
	}(time.Now())

	return lm.next.Concat(ctx, a, b)
}
