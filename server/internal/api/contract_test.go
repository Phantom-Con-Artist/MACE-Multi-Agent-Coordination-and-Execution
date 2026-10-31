package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestTheClientContractIsServedAndDocumented(t *testing.T) {
	s, _, _ := setupTestServer(t, "secret")
	served := map[string]bool{}
	chi.Walk(s.Router(), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		served[method+" "+strings.TrimSuffix(route, "/")] = true
		return nil
	})
	for _, r := range clientRoutes {
		if !served[strings.TrimSuffix(r, "/")] {
			t.Errorf("the client API promises %q, which is not served", r)
		}
	}
	doc, err := os.ReadFile("../../../docs/API.md")
	if err != nil {
		t.Fatal(err)
	}
	documented := []string{}
	for _, m := range regexp.MustCompile("`((?:GET|POST|DELETE)(?:\\|(?:GET|POST|DELETE))*) (/[^`?]*)").FindAllStringSubmatch(string(doc), -1) {
		path := strings.NewReplacer("{files|apps|host/open|catalog|metrics}", "*").Replace(m[2])
		for _, method := range strings.Split(m[1], "|") {
			documented = append(documented, method+" "+path)
		}
	}
	for _, r := range clientRoutes {
		if !slices.Contains(documented, r) {
			t.Errorf("docs/API.md does not describe %q", r)
		}
	}
	for _, d := range documented {
		if !slices.Contains(clientRoutes, d) {
			t.Errorf("docs/API.md describes %q, which is not in the client contract", d)
		}
	}
}

func TestAppsSignInWithACodeAndCanBeRevoked(t *testing.T) {
	s, _, _ := setupTestServer(t, "secret")
	do := func(method, path, auth string, body any) *httptest.ResponseRecorder {
		var b strings.Builder
		if body != nil {
			json.NewEncoder(&b).Encode(body)
		}
		r := httptest.NewRequest(method, path, strings.NewReader(b.String()))
		r.RemoteAddr = "192.168.0.9:4000"
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		s.Router().ServeHTTP(w, r)
		return w
	}
	var health map[string]any
	json.Unmarshal(do("GET", "/health", "", nil).Body.Bytes(), &health)
	if health["api"] != float64(apiVersion) {
		t.Fatalf("health should report the client API version: %v", health)
	}
	if w := do("POST", "/api/device", "", map[string]string{"name": "MACE on Pixel"}); w.Code != 401 {
		t.Fatalf("an app needs a code off the owner's tailnet: %d", w.Code)
	}
	code, _ := s.handler.newSignInCode(t.Context(), 10*60e9)
	w := do("POST", "/api/device", "", map[string]string{"code": code, "name": "MACE on Pixel"})
	var out struct {
		Credential string `json:"credential"`
		API        int    `json:"api"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || out.API != apiVersion || !strings.HasPrefix(out.Credential, "dev_") {
		t.Fatalf("sign-in: %d %s", w.Code, w.Body)
	}
	if w := do("POST", "/api/device", "", map[string]string{"code": code}); w.Code != 401 {
		t.Fatal("a code works once")
	}
	auth := "Device " + out.Credential
	if w := do("GET", "/api/v1/rooms", auth, nil); w.Code != 200 {
		t.Fatalf("the credential should open the API: %d", w.Code)
	}
	if w := do("GET", "/api/v1/rooms", "Device "+out.Credential+"x", nil); w.Code != 401 {
		t.Fatal("a wrong credential must be refused")
	}
	var devs []Device
	json.Unmarshal(do("GET", "/api/v1/devices", auth, nil).Body.Bytes(), &devs)
	if len(devs) != 1 || devs[0].Name != "MACE on Pixel" || !devs[0].Current {
		t.Fatalf("the app should be listed as this device: %+v", devs)
	}
	do("POST", "/api/v1/devices/"+devs[0].ID+"/revoke", auth, nil)
	if w := do("GET", "/api/v1/rooms", auth, nil); w.Code != 401 {
		t.Fatal("a revoked app must be signed out")
	}
}
