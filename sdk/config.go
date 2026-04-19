package sdk

type Config interface {
	FilePath() string
}

type noopConfig struct{}

func (noopConfig) FilePath() string { return "" }

func configOrDefault(cfg Config) Config {
	if cfg != nil {
		return cfg
	}

	return noopConfig{}
}
