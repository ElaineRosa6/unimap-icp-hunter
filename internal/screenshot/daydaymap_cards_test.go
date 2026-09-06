package screenshot

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestDayDayMapCardExtraction(t *testing.T) {
	chrome, err := ResolveChromePath("")
	if err != nil {
		t.Skip("Chrome is required for DOM execution")
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chrome), chromedp.NoSandbox)
	a, ac := chromedp.NewExecAllocator(context.Background(), opts...)
	defer ac()
	c, cc := chromedp.NewContext(a)
	defer cc()
	c, cancel := context.WithTimeout(c, 20*time.Second)
	defer cancel()
	fixtures := []struct {
		name, html string
		want       int
	}{
		{"cards_ignore_summary_and_duplicate", `<table><tbody><tr><td>Country</td><td>42</td></tr></tbody></table><div class="ant-pro-card"><div class="ant-pro-card-header"><div class="ant-pro-card-title"><span>https://192.0.2.1:80</span></div></div><div class="ant-pro-card-header"><div class="ant-pro-card-title"><span>https://192.0.2.1:80</span></div></div></div><div class="ant-pro-card"><div class="ant-pro-card-header"><div class="ant-pro-card-title"><span>http://192.0.2.1:8080</span></div></div></div><aside>https://192.0.2.99:443</aside>`, 2},
		{"invalid_card", `<div class="ant-pro-card"><div class="ant-pro-card-header"><div class="ant-pro-card-title"><span>https://999.1.1.1:80</span></div></div></div>`, 0},
		{"table_compatibility", `<table><tbody><tr class="ant-table-row"><td>1</td><td>192.0.2.2</td><td>--</td><td>443</td><td>https</td></tr></tbody></table>`, 1},
	}

	for _, endpoint := range []string{"https://192.0.2.3:0", "https://192.0.2.3:65536", "https://fixture:fixture@192.0.2.3", "https://example.test:443"} {
		fixtures = append(fixtures, struct {
			name, html string
			want       int
		}{endpoint, `<div class="ant-pro-card"><div class="ant-pro-card-header"><div class="ant-pro-card-title"><span>` + endpoint + `</span></div></div></div>`, 0})
	}
	for _, endpoint := range []string{"https://192.0.2.3", "http://[2001:db8::1]:8080"} {
		fixtures = append(fixtures, struct {
			name, html string
			want       int
		}{endpoint, `<div class="ant-pro-card"><div class="ant-pro-card-header"><div class="ant-pro-card-title"><span>` + endpoint + `</span></div></div></div>`, 1})
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			b, _ := json.Marshal(f.html)
			var raw string
			if e := chromedp.Run(c, chromedp.Navigate("about:blank"), chromedp.Evaluate("document.body.innerHTML="+string(b), nil), chromedp.Evaluate(extractDayDayMapJS, &raw)); e != nil {
				t.Fatal(e)
			}
			var got struct {
				Assets []struct {
					IP       string `json:"ip"`
					Port     int    `json:"port"`
					Protocol string `json:"protocol"`
				} `json:"assets"`
			}
			if e := json.Unmarshal([]byte(raw), &got); e != nil {
				t.Fatal(e)
			}
			if len(got.Assets) != f.want {
				t.Fatalf("assets=%d want=%d", len(got.Assets), f.want)
			}
			if f.name == "https://192.0.2.3" && (got.Assets[0].IP != "192.0.2.3" || got.Assets[0].Port != 443 || got.Assets[0].Protocol != "https") {
				t.Fatalf("default port fields=%+v", got.Assets)
			}
			if f.name == "http://[2001:db8::1]:8080" && (got.Assets[0].IP != "2001:db8::1" || got.Assets[0].Port != 8080 || got.Assets[0].Protocol != "http") {
				t.Fatalf("IPv6 fields=%+v", got.Assets)
			}
			if f.name == "cards_ignore_summary_and_duplicate" {
				if got.Assets[0].IP != "192.0.2.1" || got.Assets[0].Port != 80 || got.Assets[0].Protocol != "https" || got.Assets[1].Port != 8080 {
					t.Fatalf("card fields=%+v", got.Assets)
				}
			}
		})
	}
}
