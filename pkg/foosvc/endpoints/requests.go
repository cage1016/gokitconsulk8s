package endpoints

type Request interface {
	validate() error
}

// FooRequest collects the request parameters for the Foo method.
type FooRequest struct {
	S string `json:"s"`
}

func (r FooRequest) validate() error {
	return nil // TBA
}