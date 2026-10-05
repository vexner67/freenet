package grpc

import (
	"errors"
	"testing"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
)

func TestToStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		known   bool
		code    codes.Code
		message string
	}{
		{"not found", errs.Wrap("s.userRepo.GetByID", user.ErrNotFound), true, codes.NotFound, "user not found"},
		{"email taken", errs.Wrap("s.userRepo.Save", user.ErrAlreadyExists), true, codes.AlreadyExists, "user already exists"},
		{"nickname taken", errs.Wrap("s.userRepo.Update", user.ErrNicknameTaken), true, codes.AlreadyExists, "nickname is already taken"},
		{"unknown", errors.New("connection refused"), false, codes.OK, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			st, known := toStatus(tt.err)
			if known != tt.known {
				t.Fatalf("known = %v, want %v", known, tt.known)
			}
			if known && (st.Code() != tt.code || st.Message() != tt.message) {
				t.Fatalf("got %v %q, want %v %q", st.Code(), st.Message(), tt.code, tt.message)
			}
		})
	}
}
