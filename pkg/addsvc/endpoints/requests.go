package endpoints

type Request interface {
	validate() error
}

// SumRequest collects the request parameters for the Sum method.
type SumRequest struct {
	A int64 `json:"a"`
	B int64 `json:"b"`
}

func (r SumRequest) validate() error {
	return nil // TBA
}

// ConcatRequest collects the request parameters for the Concat method.
type ConcatRequest struct {
	A string `json:"a"`
	B string `json:"b"`
}

func (r ConcatRequest) validate() error {
	return nil // TBA
}