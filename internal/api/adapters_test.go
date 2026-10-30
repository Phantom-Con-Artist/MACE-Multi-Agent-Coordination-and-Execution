package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeAgentAdapters(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	fixtures := map[string]string{"codex": `{"type":"item.completed","item":{"type":"agent_message","text":"codex response"}}`, "claude": `{"result":"claude response","is_error":false}`, "agy": "agy response", "muse": "muse response"}
	for provider, output := range fixtures {
		if e := os.WriteFile(filepath.Join(root, provider), []byte("#!/bin/sh\nprintf '%s\\n' '"+output+"'\n"), 0700); e != nil {
			t.Fatal(e)
		}
	}
	s, sup, store := setupTestServer(t, "secret")
	defer store.Close()
	defer sup.Close()
	for provider := range fixtures {
		var a Session
		if call(t, s, "POST", "/api/v1/sessions", map[string]string{"provider": provider}, &a) != 201 {
			t.Fatal(provider)
		}
		reply, e := s.handler.turn(context.Background(), a.ID, "test prompt")
		if e != nil || reply != provider+" response" {
			t.Fatal(provider, reply, e)
		}
	}
}
