package apjson_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/vexner67/freenet/federation/internal/apjson"
	"github.com/vexner67/freenet/federation/internal/fedi"
)

func decode(t *testing.T, data []byte) map[string]any {
	t.Helper()

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v\n%s", err, data)
	}

	return m
}

func TestPerson(t *testing.T) {
	t.Parallel()

	data, err := apjson.Person(fedi.Person{
		ID:        "https://f.example/ap/users/1",
		Page:      "https://f.example/anya",
		Nickname:  "anya",
		Name:      "Аня",
		Summary:   "<p>hi & bye</p>",
		Inbox:     "https://f.example/ap/users/1/inbox",
		Shared:    "https://f.example/ap/inbox",
		KeyID:     "https://f.example/ap/users/1#main-key",
		PublicKey: "PEM",
		Published: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Person: %v", err)
	}

	m := decode(t, data)
	if m["type"] != "Person" || m["preferredUsername"] != "anya" || m["summary"] != "<p>hi & bye</p>" || m["published"] != "2026-01-02T03:04:05Z" {
		t.Fatalf("Person = %s", data)
	}
	if m["publicKey"].(map[string]any)["owner"] != "https://f.example/ap/users/1" {
		t.Fatalf("publicKey = %v", m["publicKey"])
	}
	if m["endpoints"].(map[string]any)["sharedInbox"] != "https://f.example/ap/inbox" {
		t.Fatalf("endpoints = %v", m["endpoints"])
	}
	if _, ok := m["icon"]; ok {
		t.Fatal("a page without a photo has an icon")
	}
}

func TestActivity(t *testing.T) {
	t.Parallel()

	note := fedi.Note{ID: "https://f.example/ap/posts/1", Content: "<p>x</p>", Attachments: []fedi.URL{"https://f.example/p.jpg"}}

	tests := []struct {
		name   string
		in     fedi.Outgoing
		object func(t *testing.T, object any)
	}{
		{"create", fedi.Outgoing{Type: fedi.TypeCreate, Note: &note}, func(t *testing.T, object any) {
			o := object.(map[string]any)
			if o["type"] != "Note" || len(o["attachment"].([]any)) != 1 || o["updated"] != nil {
				t.Fatalf("object = %v", o)
			}
		}},
		{"delete a note", fedi.Outgoing{Type: fedi.TypeDelete, Tombstone: "https://f.example/ap/posts/1"}, func(t *testing.T, object any) {
			if object.(map[string]any)["type"] != "Tombstone" {
				t.Fatalf("object = %v", object)
			}
		}},
		{"accept", fedi.Outgoing{Type: fedi.TypeAccept, Follow: &fedi.Incoming{Type: fedi.TypeFollow, ID: "https://m.example/f/1", Actor: "https://m.example/u"}}, func(t *testing.T, object any) {
			o := object.(map[string]any)
			if o["id"] != "https://m.example/f/1" || o["actor"] != "https://m.example/u" {
				t.Fatalf("object = %v", o)
			}
		}},
		{"delete the actor", fedi.Outgoing{Type: fedi.TypeDelete, Person: "https://f.example/ap/users/1"}, func(t *testing.T, object any) {
			if object != "https://f.example/ap/users/1" {
				t.Fatalf("object = %v", object)
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := apjson.Activity(tt.in)
			if err != nil {
				t.Fatalf("Activity: %v", err)
			}

			m := decode(t, data)
			if m["type"] != tt.in.Type {
				t.Fatalf("type = %v", m["type"])
			}
			tt.object(t, m["object"])
		})
	}
}

func TestIncoming(t *testing.T) {
	t.Parallel()

	follow, err := apjson.Incoming([]byte(`{"id":"https://m.example/f/1","type":"Follow","actor":"https://m.example/u","object":"https://f.example/ap/users/1"}`))
	if err != nil || follow.Type != fedi.TypeFollow || follow.Object != "https://f.example/ap/users/1" || follow.Actor != "https://m.example/u" {
		t.Fatalf("Follow = %+v, %v", follow, err)
	}

	undo, err := apjson.Incoming([]byte(`{"type":"Undo","actor":{"id":"https://m.example/u"},
		"object":{"id":"https://m.example/f/1","type":"Follow","object":"https://f.example/ap/users/1"}}`))
	if err != nil || undo.Inner == nil || undo.Inner.Type != fedi.TypeFollow ||
		undo.Inner.Object != "https://f.example/ap/users/1" || undo.Inner.Actor != "https://m.example/u" {
		t.Fatalf("Undo = %+v, %v", undo, err)
	}

	for _, bad := range []string{`nonsense`, `{"type":"Follow"}`, `{"type":"Follow","actor":"http://m.example/u"}`, `{"actor":"https://m.example/u"}`} {
		if _, err := apjson.Incoming([]byte(bad)); !errors.Is(err, fedi.ErrInvalidActivity) {
			t.Errorf("Incoming(%s) = %v, want ErrInvalidActivity", bad, err)
		}
	}
}

func TestActor(t *testing.T) {
	t.Parallel()

	doc, err := apjson.Actor([]byte(`{"id":"https://m.example/u","inbox":"https://m.example/u/inbox",
		"endpoints":{"sharedInbox":"https://m.example/inbox"},"publicKey":{"id":"https://m.example/u#main-key","publicKeyPem":"PEM"}}`))
	if err != nil {
		t.Fatalf("Actor: %v", err)
	}

	want := fedi.RemoteDoc{ID: "https://m.example/u", Inbox: "https://m.example/u/inbox", SharedInbox: "https://m.example/inbox", KeyID: "https://m.example/u#main-key", PublicKey: "PEM"}
	if doc != want {
		t.Fatalf("Actor = %+v, want %+v", doc, want)
	}

	if _, err := apjson.Actor([]byte(`[]`)); !errors.Is(err, fedi.ErrInvalidActivity) {
		t.Fatalf("Actor of an array = %v", err)
	}
}

func TestFingerAndCollection(t *testing.T) {
	t.Parallel()

	data, err := apjson.Finger(fedi.Person{ID: "https://f.example/ap/users/1", Page: "https://f.example/anya", Nickname: "anya"}, "f.example")
	if err != nil {
		t.Fatalf("Finger: %v", err)
	}

	m := decode(t, data)
	if m["subject"] != "acct:anya@f.example" || m["links"].([]any)[0].(map[string]any)["href"] != "https://f.example/ap/users/1" {
		t.Fatalf("Finger = %s", data)
	}

	data, err = apjson.Collection("https://f.example/ap/users/1/followers", 3)
	if err != nil {
		t.Fatalf("Collection: %v", err)
	}

	if m = decode(t, data); m["totalItems"] != float64(3) || m["orderedItems"] != nil {
		t.Fatalf("Collection = %s", data)
	}
}
