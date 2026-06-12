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

	// RemnantOverflowThresholdPct is the RED threshold for overflow status.
	// Configured via REMNANT_OVERFLOW_THRESHOLD_PCT. Values <=0 or >100 are
	// normalized to module default in inventory.NewServiceWithOverflowThreshold.
	RemnantOverflowThresholdPct float64 `env:"REMNANT_OVERFLOW_THRESHOLD_PCT" envDefault:"15"`

	// ContainerCBMOverheadPct widens the CBM/weight cap on delivery.AddLine
	// and TransferLine by this percentage. Real-world container loading rarely
	// reaches the ISO max so allowing a small overhead keeps the API from
	// rejecting realistic packing plans. 5% matches the operational tolerance
	// the warehouse team has been using by hand.
	ContainerCBMOverheadPct float64 `env:"CONTAINER_CBM_OVERHEAD_PCT" envDefault:"5"`

	// R2 object storage (Cloudflare). All fields optional; when any is absent
	// the presign endpoint returns 503 rather than crashing on startup.
	R2AccountID     string `env:"R2_ACCOUNT_ID"`
	R2AccessKeyID   string `env:"R2_ACCESS_KEY_ID"`
	R2SecretKey     string `env:"R2_SECRET_KEY"`
	R2BucketName    string `env:"R2_BUCKET_NAME"`
	R2PublicBaseURL string `env:"R2_PUBLIC_BASE_URL"`
}

func Load() (Config, error) {
	return env.ParseAs[Config]()
}
