package response

// ErrorDetail is one machine-readable error entry. A response may carry
// several (e.g. multiple field-validation failures at once).
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

// Envelope is the canonical shape of every HTTP response body.
type Envelope struct {
	Success       bool          `json:"success"`
	Message       string        `json:"message"`
	Data          any           `json:"data,omitempty"`
	Errors        []ErrorDetail `json:"errors,omitempty"`
	CorrelationID string        `json:"correlation_id"`
}

// NewSuccess builds a success envelope.
func NewSuccess(correlationID, message string, data any) Envelope {
	return Envelope{Success: true, Message: message, Data: data, CorrelationID: correlationID}
}

// NewError builds a failure envelope carrying one or more error details.
func NewError(correlationID, message string, errs ...ErrorDetail) Envelope {
	return Envelope{Success: false, Message: message, Errors: errs, CorrelationID: correlationID}
}
