package safety

import "time"

type Config struct {
	MaxOutputLength int
	RateMaxCalls    int
	RateWindow      time.Duration
}

func DefaultConfig() Config {
	return Config{
		MaxOutputLength: 10000,
		RateMaxCalls:    100,
		RateWindow:      time.Minute,
	}
}
