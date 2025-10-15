package ai

// CodegenTask describes an automated generation task for future AI workflows.
type CodegenTask interface {
	Name() string
	Inputs() map[string]string
	ExpectedOutputs() []string
}

// StaticTask is a minimal implementation of CodegenTask suitable for stubs.
type StaticTask struct {
	TaskName string
	Params   map[string]string
	Outputs  []string
}

func (t StaticTask) Name() string { return t.TaskName }

func (t StaticTask) Inputs() map[string]string { return t.Params }

func (t StaticTask) ExpectedOutputs() []string { return t.Outputs }
