package config

import "github.com/caarlos0/env/v11"

type Config struct {
	DatabaseURL string `env:"DATABASE_URL,required"`
	Port        string `env:"PORT" envDefault:"8080"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	AuthSecret  string `env:"AUTH_SECRET,required"`
}

func Load() (Config, error) {
	return env.ParseAs[Config]()
}
