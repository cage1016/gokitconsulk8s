package transport

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

type TransportRouter struct {
	Router *mux.Router
}

func NewHandlerBuilder() TransportRouter {
	r := mux.NewRouter()

	r.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("ok"))
	})

	return TransportRouter{r}
}

func (tr TransportRouter) AddHandler(prefix string, h http.Handler) {
	buf := fmt.Sprintf("/%s", prefix)
	tr.Router.PathPrefix(buf).Handler(http.StripPrefix(buf, h))
}
