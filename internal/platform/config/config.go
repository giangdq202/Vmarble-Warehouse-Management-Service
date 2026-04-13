package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	DatabaseURL string `env:"DATABASE_URL,required"`
	Port        string `env:"PORT" envDefault:"8080"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	AuthSecret  string `env:"AUTH_SECRET,required"`

	// RemnantAllocTimeout is the maximum duration a remnant can remain ALLOCATED
	// without being consumed before the background task auto-releases it back to
	// AVAILABLE. Configured via REMNANT_ALLOC_TIMEOUT (e.g. "24h", "30m").
	RemnantAllocTimeout time.Duration `env:"REMNANT_ALLOC_TIMEOUT" envDefault:"24h"`

	// RemnantAllocCheckInterval controls how often the background task scans
	// for expired allocations. Configured via REMNANT_ALLOC_CHECK_INTERVAL.
	RemnantAllocCheckInterval time.Duration `env:"REMNANT_ALLOC_CHECK_INTERVAL" envDefault:"1h"`
}

func Load() (Config, error) {
	return env.ParseAs[Config]()
}
