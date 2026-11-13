package grpc_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGames(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()
	game, pdf := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()

	if _, err := s.client.AddDocument(s.as(t, ann), &postv1.AddDocumentRequest{Id: game, FileName: "snake.html", Size: 100}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.client.AddDocument(s.as(t, ann), &postv1.AddDocumentRequest{Id: pdf, FileName: "a.pdf", Size: 100}); err != nil {
		t.Fatal(err)
	}

	// Nothing saved yet.
	loaded, err := s.client.LoadGame(s.as(t, bob), &postv1.LoadGameRequest{GameId: game})
	if err != nil || loaded.GetFound() {
		t.Fatalf("LoadGame before a save = %v, %v", loaded, err)
	}

	if _, err = s.client.SaveGame(s.as(t, bob), &postv1.SaveGameRequest{GameId: game, Data: `{"level":3}`}); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	if loaded, err = s.client.LoadGame(s.as(t, bob), &postv1.LoadGameRequest{GameId: game}); err != nil || !loaded.GetFound() || loaded.GetData() != `{"level":3}` {
		t.Fatalf("LoadGame = %v, %v", loaded, err)
	}
	// Each player has their own.
	if loaded, err = s.client.LoadGame(s.as(t, ann), &postv1.LoadGameRequest{GameId: game}); err != nil || loaded.GetFound() {
		t.Fatalf("LoadGame of another = %v, %v", loaded, err)
	}

	for name, call := range map[string]func() error{
		"a save over 64 KB": func() error {
			_, err := s.client.SaveGame(s.as(t, bob), &postv1.SaveGameRequest{GameId: game, Data: strings.Repeat("x", 64<<10+1)})
			return err
		},
		"a negative score": func() error {
			_, err := s.client.RecordGameScore(s.as(t, bob), &postv1.RecordGameScoreRequest{GameId: game, Score: -1})
			return err
		},
		"a bad game id": func() error {
			_, err := s.client.LoadGame(s.as(t, bob), &postv1.LoadGameRequest{GameId: "x"})
			return err
		},
	} {
		if status.Code(call()) != codes.InvalidArgument {
			t.Errorf("%s: not InvalidArgument", name)
		}
	}

	// A document that is not a game, or none at all.
	for _, id := range []string{pdf, uuid.NewString()} {
		if _, err = s.client.SaveGame(s.as(t, bob), &postv1.SaveGameRequest{GameId: id, Data: "1"}); status.Code(err) != codes.NotFound {
			t.Errorf("SaveGame of %s: %v", id, err)
		}
	}

	if _, err = s.client.LoadGame(context.Background(), &postv1.LoadGameRequest{GameId: game}); status.Code(err) != codes.Unauthenticated {
		t.Errorf("LoadGame without a token: %v", err)
	}

	// The best score stays.
	for _, score := range []int64{40, 70, 50} {
		if _, err = s.client.RecordGameScore(s.as(t, bob), &postv1.RecordGameScoreRequest{GameId: game, Score: score}); err != nil {
			t.Fatal(err)
		}
	}
	best, err := s.client.RecordGameScore(s.as(t, ann), &postv1.RecordGameScoreRequest{GameId: game, Score: 10})
	if err != nil || best.GetBest() != 10 {
		t.Fatalf("RecordGameScore = %v, %v", best, err)
	}

	records, err := s.client.ListGameRecords(s.as(t, ann), &postv1.ListGameRecordsRequest{GameId: game, UserIds: []string{ann.String(), bob.String(), uuid.NewString()}})
	if err != nil || len(records.GetRecords()) != 2 {
		t.Fatalf("ListGameRecords = %v, %v", records, err)
	}
	if r := records.GetRecords(); r[0].GetUserId() != bob.String() || r[0].GetScore() != 70 || r[1].GetUserId() != ann.String() || r[1].GetScore() != 10 {
		t.Fatalf("records = %v", r)
	}
}
