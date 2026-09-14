package config

import "time"

// Config is the complete config.yaml schema. Configuration is immutable after startup.
type Config struct {
	Server     Server     `yaml:"server"`
	Simulation Simulation `yaml:"simulation"`
	TCP        TCP        `yaml:"tcp"`
	UDP        UDP        `yaml:"udp"`
	Logging    Logging    `yaml:"logging"`
	UI         UI         `yaml:"ui"`
}

// Server configures the HTTP listener and finite request/shutdown budgets.
type Server struct {
	// Address is a host:port pair, defaulting to a loopback interface.
	Address           string        `yaml:"address"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout   time.Duration `yaml:"shutdown_timeout"`
	// MaxBodyBytes bounds API request bodies.
	MaxBodyBytes int64 `yaml:"max_body_bytes"`
}

// Simulation configures deterministic virtual time and persistent scenario data.
type Simulation struct {
	TickInterval time.Duration `yaml:"tick_interval"`
	// Speed is a finite positive wall-time multiplier; pause is an explicit command.
	Speed float64 `yaml:"speed"`
	// Seed controls reproducible synthetic traffic.
	Seed          int64  `yaml:"seed"`
	DataDirectory string `yaml:"data_directory"`
	// AutostartScenario is empty when the simulator should start idle.
	AutostartScenario string `yaml:"autostart_scenario"`
}

// TCP configures the outbound AIS stream served to connected TCP clients.
type TCP struct {
	Enabled    bool   `yaml:"enabled"`
	Address    string `yaml:"address"`
	MaxClients int    `yaml:"max_clients"`
	// QueueCapacity is the maximum queued sentence batches per client.
	QueueCapacity int           `yaml:"queue_capacity"`
	WriteTimeout  time.Duration `yaml:"write_timeout"`
}

// UDP configures outbound datagrams to explicit host:port destinations.
type UDP struct {
	Enabled      bool     `yaml:"enabled"`
	LocalAddress string   `yaml:"local_address"`
	Destinations []string `yaml:"destinations"`
	// Broadcast enables IPv4 broadcast socket behavior; unicast is the default.
	Broadcast     bool          `yaml:"broadcast"`
	QueueCapacity int           `yaml:"queue_capacity"`
	WriteTimeout  time.Duration `yaml:"write_timeout"`
}

// Logging configures structured application logs.
type Logging struct {
	// Level is debug, info, warn, or error.
	Level string `yaml:"level"`
	// Format is json or text.
	Format string `yaml:"format"`
}

// UI independently enables the two embedded frontend mounts.
type UI struct {
	AdminEnabled  bool `yaml:"admin_enabled"`
	ViewerEnabled bool `yaml:"viewer_enabled"`
}
