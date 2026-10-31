package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
)

func TestMessagesMigrateFromDocumentsInOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mace.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(baseSchema + migrations[1] + `PRAGMA user_version = 2;`); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"b", "a", "c"} {
		old.Exec(`INSERT INTO documents(kind,id,data) VALUES(?,?,?)`, "message:room1", id, fmt.Sprintf(`{"id":%q,"n":%d}`, id, i))
	}
	old.Exec(`INSERT INTO documents(kind,id,data) VALUES('client-message:room1','c1','{"message":"b"}')`)
	old.Close()

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, more, err := store.Messages(context.Background(), "room1", 0, 0, 10)
	if err != nil || more || len(got) != 3 {
		t.Fatalf("got %d messages, more=%v, err=%v", len(got), more, err)
	}
	for i, want := range []string{"b", "a", "c"} {
		var m map[string]any
		json.Unmarshal(got[i].Data, &m)
		if m["id"] != want {
			t.Fatalf("message %d is %v, want %s: insertion order must be kept", i, m["id"], want)
		}
	}
	if left, _ := store.Documents(context.Background(), "message:room1"); len(left) != 0 {
		t.Fatal("old message documents should be removed after the move")
	}
	var link map[string]string
	if store.Document(context.Background(), "client-message:room1", "c1", &link) != nil {
		t.Fatal("client-message documents must not be touched")
	}
}

func TestMessagesPaging(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	var seqs []int64
	for i := 0; i < 7; i++ {
		seq, err := store.AppendMessage(ctx, "r", fmt.Sprint("m", i), map[string]int{"n": i})
		if err != nil {
			t.Fatal(err)
		}
		seqs = append(seqs, seq)
		store.AppendMessage(ctx, "other", fmt.Sprint("o", i), map[string]int{"n": i})
	}
	n := func(rows []StoredMessage) (out []int) {
		for _, r := range rows {
			var m map[string]int
			json.Unmarshal(r.Data, &m)
			out = append(out, m["n"])
		}
		return
	}
	latest, more, _ := store.Messages(ctx, "r", 0, 0, 3)
	if fmt.Sprint(n(latest)) != "[4 5 6]" || !more {
		t.Fatalf("newest page wrong: %v more=%v", n(latest), more)
	}
	earlier, more, _ := store.Messages(ctx, "r", latest[0].Seq, 0, 3)
	if fmt.Sprint(n(earlier)) != "[1 2 3]" || !more {
		t.Fatalf("earlier page wrong: %v more=%v", n(earlier), more)
	}
	first, more, _ := store.Messages(ctx, "r", earlier[0].Seq, 0, 3)
	if fmt.Sprint(n(first)) != "[0]" || more {
		t.Fatalf("first page wrong: %v more=%v", n(first), more)
	}
	newer, more, _ := store.Messages(ctx, "r", 0, seqs[4], 10)
	if fmt.Sprint(n(newer)) != "[5 6]" || more {
		t.Fatalf("after cursor wrong: %v more=%v", n(newer), more)
	}
	store.UpdateMessage(ctx, "m2", map[string]int{"n": 22})
	one, _ := store.MessageByID(ctx, "m2")
	if one.Seq != seqs[2] || string(one.Data) != `{"n":22}` {
		t.Fatalf("update must keep position: %+v", one)
	}
}
