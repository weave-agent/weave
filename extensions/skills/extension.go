package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"weave/sdk"
)

type SkillsExtension struct {
	cfg            sdk.Config
	discoveryPaths []string
}

func init() {
	sdk.RegisterExtension("skills", func(cfg sdk.Config) (sdk.Extension, error) {
		return NewSkillsExtension(cfg)
	})
}

func NewSkillsExtension(cfg sdk.Config) (*SkillsExtension, error) {
	return &SkillsExtension{cfg: cfg}, nil
}

func (e *SkillsExtension) Name() string { return "skills" }

func (e *SkillsExtension) Subscribe(bus sdk.Bus) {
	var paths []string
	if len(e.discoveryPaths) > 0 {
		paths = e.discoveryPaths
	} else {
		home, _ := os.UserHomeDir()
		if home != "" {
			paths = append(paths, filepath.Join(home, ".weave", "skills"))
		}

		if e.cfg.FilePath() != "" {
			projectRoot := filepath.Dir(e.cfg.FilePath())
			paths = append(paths, filepath.Join(projectRoot, ".weave", "skills"))
		}
	}

	skills, err := discoverSkills(paths...)
	if err != nil {
		bus.Publish(sdk.NewEvent("skills.error", err.Error()))
	}

	bus.Publish(sdk.NewEvent(sdk.TopicSkillsLoaded, formatSkillsPrompt(skills)))

	ui, err := sdk.GetUI("tui")
	if err != nil {
		return
	}

	for i := range skills {
		skill := skills[i]
		cmdName := "/skill:" + skill.Name
		ui.RegisterCommand(cmdName, makeSkillHandler(skill, bus))
	}
}

func (e *SkillsExtension) Close() error {
	return nil
}

func makeSkillHandler(skill Skill, bus sdk.Bus) func(args string) error {
	return func(args string) error {
		body := skill.Body()

		var msg strings.Builder
		fmt.Fprintf(&msg, "<skill name=%q location=%q>\n", skill.Name, skill.FilePath)
		fmt.Fprintf(&msg, "References are relative to %s.\n\n", skill.BaseDir)
		msg.WriteString(body)
		msg.WriteString("\n</skill>")

		if args != "" {
			msg.WriteString("\n\n")
			msg.WriteString(args)
		}

		bus.Publish(sdk.NewEvent("agent.prompt", msg.String()))

		return nil
	}
}
