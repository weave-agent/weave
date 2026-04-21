package sdk

type NoopUI struct{}

func (NoopUI) Select(_ string, items []string) (int, error) {
	if len(items) == 0 {
		return -1, nil
	}

	return 0, nil
}

func (NoopUI) Confirm(_ string) (bool, error) {
	return true, nil
}

func (NoopUI) Input(_ string) (string, error) {
	return "", nil
}

func (NoopUI) SetStatus(_, _ string) {}

func (NoopUI) Notify(_ string) {}

func (NoopUI) RegisterCommand(_ string, _ func(args string) error) {}

func (NoopUI) RegisterRenderer(_ string, _ ToolRenderer) {}

func (NoopUI) RegisterKeybinding(_ Keybinding) {}
