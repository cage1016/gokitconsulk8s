package service

import (
	"context"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

type loggingMiddleware struct {
	logger log.Logger    `json:""`
	next   FoosvcService `json:""`
}

// LoggingMiddleware takes a logger as a dependency
// and returns a ServiceMiddleware.
func LoggingMiddleware(logger log.Logger) Middleware {
	return func(next FoosvcService) FoosvcService {
		return loggingMiddleware{level.Info(logger), next}
	}
}

func (lm loggingMiddleware) Foo(ctx context.Context, s string) (res string, err error) {
	defer func(begin time.Time) {
		lm.logger.Log("method", "Foo", "s", s, "err", err)
	}(time.Now())

	return lm.next.Foo(ctx, s)
}
