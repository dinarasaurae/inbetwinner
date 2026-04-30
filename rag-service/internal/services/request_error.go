package services

// RequestError marks failures caused by invalid or unusable client input so
// handlers can return a 4xx status instead of a generic 500.
type RequestError struct {
	Status  int
	Message string
}

func (e *RequestError) Error() string {
	return e.Message
}
