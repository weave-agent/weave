package instructions

import (
	"os"
	"path/filepath"

	"weave/sdk"
)

//go:generate moq -fmt goimports -skip-ensure -pkg instructions -out mock_test.go ../../sdk Bus

// TopicInstructionsLoaded is the bus topic for when instructions have been discovered and loaded.
const TopicInstructionsLoaded = "instructions.loaded"

type InstructionsExtension struct {
	cfg sdk.Config
}

func init() {
	sdk.RegisterExtension[struct{}]("instructions", func(cfg sdk.Config, _ struct{}) (sdk.Extension, error) {
		return NewInstructionsExtension(cfg)
	})
}

func NewInstructionsExtension(cfg sdk.Config) (*InstructionsExtension, error) {
	return &InstructionsExtension{cfg: cfg}, nil
}

func (e *InstructionsExtension) Name() string { return "instructions" }

func (e *InstructionsExtension) Subscribe(bus sdk.Bus) error {
	projectDir := e.projectDir()
	globalDir := globalConfigDir()

	if projectDir != "" {
		if abs, err := filepath.Abs(projectDir); err == nil {
			projectDir = abs
		}
	}

	contextFiles := discoverContextFiles(projectDir, globalDir)
	systemBase, systemAppend := loadSystemPrompt(projectDir, globalDir)
	prompt := formatInstructionsPrompt(contextFiles, systemBase, systemAppend)

	bus.Publish(sdk.NewEvent(TopicInstructionsLoaded, prompt))

	return nil
}

func (e *InstructionsExtension) Close() error {
	return nil
}

func (e *InstructionsExtension) projectDir() string {
	if pd := e.cfg.ProjectDir(); pd != "" {
		return pd
	}

	fp := e.cfg.FilePath()
	if fp == "" {
		return ""
	}

	dir := filepath.Dir(fp)
	if filepath.Base(dir) == ".weave" {
		dir = filepath.Dir(dir)
	}

	return dir
}

func globalConfigDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}

	return filepath.Join(home, ".weave")
}
