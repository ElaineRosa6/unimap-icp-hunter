package config

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDockerGateExecutesDigestPinnedSmoke(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Jobs map[string]struct {
			Steps []struct {
				ID   string            `yaml:"id"`
				Name string            `yaml:"name"`
				Uses string            `yaml:"uses"`
				Run  string            `yaml:"run"`
				Env  map[string]string `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	buildID := ""
	for _, step := range cfg.Jobs["docker"].Steps {
		if strings.HasPrefix(step.Uses, "docker/build-push-action@") {
			buildID = step.ID
		}
		if step.Name == "Verify image" {
			if buildID == "" || !strings.Contains(step.Run, "scripts/ci_image_smoke.py") ||
				step.Env["IMAGE_DIGEST"] != "${{ steps."+buildID+".outputs.digest }}" ||
				step.Env["EXPECTED_COMMIT"] != "${{ github.sha }}" {
				t.Fatal("image gate must execute runtime smoke pinned to the build output digest and commit")
			}
			return
		}
	}
	t.Fatal("missing Verify image gate")
}
