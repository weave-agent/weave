package sdk

//go:generate moq -fmt goimports -stub -out config_mock_test.go . Config

type Config interface {
	FilePath() string
}

type noopConfig struct{}

func (noopConfig) FilePath() string { return "" }

// FilePathConfig is a Config that returns the given path from FilePath().
type FilePathConfig string

func (f FilePathConfig) FilePath() string { return string(f) }

func configOrDefault(cfg Config) Config {
	if cfg != nil {
		return cfg
	}

	return noopConfig{}
}
