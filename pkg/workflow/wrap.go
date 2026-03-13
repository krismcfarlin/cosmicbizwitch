package workflow

import (
	"context"
	"encoding/json"
	"fmt"
)

// ActivityFunc is the universal activity function signature.
// All activities take and return map[string]any for maximum flexibility.
type ActivityFunc func(ctx context.Context, input map[string]any) (map[string]any, error)

// Wrap converts a typed function into an ActivityFunc using JSON as the bridge.
// In and Out must be JSON-serializable (structs or maps).
//
// Example:
//
//	type SendEmailIn  struct { To string `json:"to"`; Subject string `json:"subject"` }
//	type SendEmailOut struct { Sent bool `json:"sent"` }
//
//	eng.RegisterActivity("send_email", workflow.Wrap(
//	    func(ctx context.Context, in SendEmailIn) (SendEmailOut, error) {
//	        // ... send the email ...
//	        return SendEmailOut{Sent: true}, nil
//	    },
//	))
func Wrap[In, Out any](fn func(ctx context.Context, in In) (Out, error)) ActivityFunc {
	return func(ctx context.Context, input map[string]any) (map[string]any, error) {
		// map[string]any → JSON → In
		b, err := json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("wrap: marshal input: %w", err)
		}
		var typed In
		if err := json.Unmarshal(b, &typed); err != nil {
			return nil, fmt.Errorf("wrap: unmarshal input to typed: %w", err)
		}

		// call the real function
		out, err := fn(ctx, typed)
		if err != nil {
			return nil, err
		}

		// Out → JSON → map[string]any
		b2, err := json.Marshal(out)
		if err != nil {
			return nil, fmt.Errorf("wrap: marshal output: %w", err)
		}
		var result map[string]any
		if err := json.Unmarshal(b2, &result); err != nil {
			return nil, fmt.Errorf("wrap: unmarshal output to map: %w", err)
		}
		return result, nil
	}
}
