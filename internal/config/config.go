// Package config defines the application's configuration structure.
package config

const (
	// DefaultAddr is the default address the server listens on.
	DefaultAddr = ":8080"
	// DefaultDataDir is the default directory for application data.
	DefaultDataDir = "."
	// DefaultBaseURL is the default base URL of the server.
	DefaultBaseURL = "http://localhost:8080"
)

// Config holds the application's runtime configuration.
type Config struct {
	Addr    string
	DataDir string
	BaseURL string
}

// New returns a Config populated with default values.
func New() *Config {
	return &Config{
		Addr:    DefaultAddr,
		DataDir: DefaultDataDir,
		BaseURL: DefaultBaseURL,
	}
}
