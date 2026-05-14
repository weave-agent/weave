package sdk

type NoopUI struct{}

func (NoopUI) Select(_ string, items []string, _ ...SelectOption) (int, error) {
	if len(items) == 0 {
		return -1, nil
	}

	return 0, nil
}

func (NoopUI) Confirm(_ string, _ ...ConfirmOption) (bool, error) {
	return true, nil
}

func (NoopUI) Input(_ string, _ ...InputOption) (string, error) {
	return "", nil
}

func (NoopUI) MultiSelect(_ string, _ []string, _ []bool, _ ...SelectOption) ([]int, error) {
	return nil, nil
}

func (NoopUI) Editor(_, _ string, _ ...EditorOption) (string, error) {
	return "", nil
}

func (NoopUI) SetStatus(_, _ string) {}

func (NoopUI) Notify(_ string) {}

func (NoopUI) NotifyTyped(_ string, _ NotifyLevel) {}

func (NoopUI) ShowError(_ string) {}

func (NoopUI) SetWorking(_ string) {}

func (NoopUI) ClearWorking() {}

func (NoopUI) RegisterCommand(_ string, _ func(args string) error) {}

func (NoopUI) RegisterRenderer(_ string, _ ToolRenderer) {}

func (NoopUI) RegisterKeybinding(_ Keybinding) {}

func (NoopUI) SetTheme(_ string) error { return nil }

func (NoopUI) ListThemes() []string { return nil }
