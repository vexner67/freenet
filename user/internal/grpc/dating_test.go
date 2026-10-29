package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDating(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, annID := s.member(t, "ann")
	bob, bobID := s.member(t, "bob")
	stranger := s.signIn(t, "eve@example.com")

	if _, err := s.client.GetDating(context.Background(), &userv1.GetDatingRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("GetDating by a guest: %v", err)
	}
	if _, err := s.client.SaveDatingCard(stranger, &userv1.SaveDatingCardRequest{Goal: "chat", On: true}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("SaveDatingCard without a profile: %v", err)
	}
	for _, bad := range []*userv1.SaveDatingCardRequest{{Goal: "money"}, {Goal: "chat", Seek: "neutral"}} {
		if _, err := s.client.SaveDatingCard(ann, bad); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("SaveDatingCard(%v): %v", bad, err)
		}
	}
	if _, err := s.client.AnswerDating(ann, &userv1.AnswerDatingRequest{UserId: bobID, Like: true}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("AnswerDating without a card: %v", err)
	}

	for _, ctx := range []context.Context{ann, bob} {
		saved, err := s.client.SaveDatingCard(ctx, &userv1.SaveDatingCardRequest{Goal: "love", About: " о себе ", On: true})
		if err != nil || saved.GetCard().GetAbout() != "о себе" || !saved.GetCard().GetOn() {
			t.Fatalf("SaveDatingCard = %v, %v", saved, err)
		}
	}

	view, err := s.client.GetDating(ann, &userv1.GetDatingRequest{})
	if err != nil || view.GetCard().GetGoal() != "love" || len(view.GetDeck()) != 1 || view.GetDeck()[0].GetUserId() != bobID {
		t.Fatalf("GetDating = %v, %v", view, err)
	}
	if _, err = s.client.AnswerDating(ann, &userv1.AnswerDatingRequest{UserId: "bad"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("AnswerDating of a bad id: %v", err)
	}
	if _, err = s.client.AnswerDating(ann, &userv1.AnswerDatingRequest{UserId: annID, Like: true}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("liking oneself: %v", err)
	}
	if answer, err := s.client.AnswerDating(ann, &userv1.AnswerDatingRequest{UserId: bobID, Like: true}); err != nil || answer.GetMatched() {
		t.Fatalf("the first like = %v, %v", answer, err)
	}
	if _, err = s.client.AnswerDating(ann, &userv1.AnswerDatingRequest{UserId: bobID, Like: true}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("a second like: %v", err)
	}
	if answer, err := s.client.AnswerDating(bob, &userv1.AnswerDatingRequest{UserId: annID, Like: true}); err != nil || !answer.GetMatched() {
		t.Fatalf("the like back = %v, %v", answer, err)
	}
	if view, err = s.client.GetDating(bob, &userv1.GetDatingRequest{}); err != nil || len(view.GetMatches()) != 1 || view.GetMatches()[0].GetUserId() != annID {
		t.Fatalf("the matches of Bob = %v, %v", view, err)
	}
}
