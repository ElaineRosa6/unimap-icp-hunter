package utils

import (
	"testing"
	"time"

	"github.com/unimap/project/internal/model"
)

func TestAuditCacheAssetOwnership(t *testing.T) {
	for _, direction := range []string{"set_input", "setmulti_input", "get_output", "getmulti_output"} {
		for _, field := range []string{"scalar", "headers", "extra_nested", "extra_slice"} {
			t.Run(direction+"/"+field, func(t *testing.T) {
				c := NewMemoryCache(2, 0)
				defer c.Close()
				input := []model.UnifiedAsset{{Title: "original", Headers: map[string]string{"X-Fixture": "original"}, Extra: map[string]interface{}{"nested": map[string]interface{}{"value": "original"}, "list": []string{"original"}}}}
				if direction == "setmulti_input" {
					c.SetMulti(map[string][]model.UnifiedAsset{"k": input}, time.Hour)
				} else {
					c.Set("k", input, time.Hour)
				}
				target := input
				switch direction {
				case "get_output":
					target, _ = c.Get("k")
				case "getmulti_output":
					target = c.GetMulti([]string{"k"})["k"]
				}
				switch field {
				case "scalar":
					target[0].Title = "changed"
				case "headers":
					target[0].Headers["X-Fixture"] = "changed"
				case "extra_nested":
					target[0].Extra["nested"].(map[string]interface{})["value"] = "changed"
				case "extra_slice":
					target[0].Extra["list"].([]string)[0] = "changed"
				}
				got, ok := c.Get("k")
				if !ok || len(got) != 1 {
					t.Fatal("missing cache entry")
				}
				value := ""
				switch field {
				case "scalar":
					value = got[0].Title
				case "headers":
					value = got[0].Headers["X-Fixture"]
				case "extra_nested":
					value = got[0].Extra["nested"].(map[string]interface{})["value"].(string)
				case "extra_slice":
					value = got[0].Extra["list"].([]string)[0]
				}
				if value != "original" {
					t.Fatalf("caller mutation changed cached %s to %q", field, value)
				}
			})
		}
	}
}
