package pkg

import (
	"context"
	"io"
)

// ReadAll reads from r until an error, given context is cancelled or EOF and returns the data it read.
func ReadAll(ctx context.Context, r io.Reader) ([]byte, error) {
	b := make([]byte, 0, 512)
	for {
		// Add support for context
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		n, err := r.Read(b[len(b):cap(b)])
		b = b[:len(b)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return b, err
		}

		if len(b) == cap(b) {
			// Add more capacity (let append pick how much).
			b = append(b, 0)[:len(b)]
		}
	}
}
