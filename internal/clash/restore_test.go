package clash

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

type reloadTimeout struct{}

func (reloadTimeout) Error() string   { return "response timed out" }
func (reloadTimeout) Timeout() bool   { return true }
func (reloadTimeout) Temporary() bool { return true }

func TestRestoreChecksActualStateAfterResponseTimeout(t *testing.T) {
	for _, remaining := range []bool{false, true} {
		t.Run(fmt.Sprint(remaining), func(t *testing.T) {
			m := &SystemdManager{HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method == "PUT" {
					return nil, reloadTimeout{}
				}
				w := httptest.NewRecorder()
				switch r.URL.Path {
				case "/rules":
					fmt.Fprint(w, `{"rules":[]}`)
				case "/proxies":
					if remaining {
						fmt.Fprint(w, `{"proxies":{"AGYWARP-WARP":{}}}`)
					} else {
						fmt.Fprint(w, `{"proxies":{}}`)
					}
				}
				return w.Result(), nil
			})}}
			err := m.restoreBase(context.Background(), []byte("rules: [MATCH,DIRECT]"))
			if (err != nil) != remaining {
				t.Fatalf("remaining=%v err=%v", remaining, err)
			}
		})
	}
}
