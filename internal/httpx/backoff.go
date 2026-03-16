package httpx

import "time"

// FixedBackoff returns a slice of delays for retries: [d, d, d, ...].
func FixedBackoff(delay time.Duration, attempts int) []time.Duration {
	if attempts <= 0 {
		return nil
	}
	out := make([]time.Duration, attempts)
	for i := range out {
		out[i] = delay
	}
	return out
}
