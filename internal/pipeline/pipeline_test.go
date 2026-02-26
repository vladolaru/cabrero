package pipeline

import "testing"

func TestDefaultPipelineConfig_AllModelFieldsSet(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cases := []struct {
		name  string
		field string
	}{
		{"CuratorCheckModel", cfg.CuratorCheckModel},
		{"ApplyModel", cfg.ApplyModel},
		{"ChatModel", cfg.ChatModel},
		{"MetaModel", cfg.MetaModel},
	}
	for _, c := range cases {
		if c.field == "" {
			t.Errorf("%s is empty in DefaultPipelineConfig", c.name)
		}
	}
}
