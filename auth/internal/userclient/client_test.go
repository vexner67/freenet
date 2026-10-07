package userclient_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/account"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/auth/internal/userclient"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAPI struct {
	userv1.UserServiceClient

	resp  *userv1.GetOrCreateByEmailResponse
	state *userv1.GetAccountStateResponse
	err   error
	req   *userv1.GetOrCreateByEmailRequest
}

func (f *fakeAPI) GetAccountState(
	_ context.Context,
	_ *userv1.GetAccountStateRequest,
	_ ...grpc.CallOption,
) (*userv1.GetAccountStateResponse, error) {
	return f.state, f.err
}

func (f *fakeAPI) GetOrCreateByEmail(
	_ context.Context,
	req *userv1.GetOrCreateByEmailRequest,
	_ ...grpc.CallOption,
) (*userv1.GetOrCreateByEmailResponse, error) {
	f.req = req
	return f.resp, f.err
}

func TestClient_GetOrCreateByEmail(t *testing.T) {
	t.Parallel()

	t.Run("returns the user id", func(t *testing.T) {
		t.Parallel()

		want := uuid.New()
		api := &fakeAPI{resp: &userv1.GetOrCreateByEmailResponse{UserId: want.String(), Deactivated: true}}

		got, err := userclient.New(api).GetOrCreateByEmail(context.Background(), "ann@example.com")
		if err != nil {
			t.Fatalf("GetOrCreateByEmail: %v", err)
		}
		if got != (service.Account{UserID: want, Deactivated: true}) {
			t.Fatalf("got %+v, want %s deactivated", got, want)
		}
		if api.req.GetEmail() != "ann@example.com" {
			t.Fatalf("email = %q", api.req.GetEmail())
		}
	})

	t.Run("malformed id from the user service", func(t *testing.T) {
		t.Parallel()

		api := &fakeAPI{resp: &userv1.GetOrCreateByEmailResponse{UserId: "not-a-uuid"}}

		if _, err := userclient.New(api).GetOrCreateByEmail(context.Background(), "ann@example.com"); err == nil {
			t.Fatal("expected an error")
		}
	})

	errTests := []struct {
		name string
		code codes.Code
		want error
	}{
		{"invalid argument", codes.InvalidArgument, account.ErrInvalidEmail},
		{"unavailable", codes.Unavailable, service.ErrUserServiceUnavailable},
		{"deadline exceeded", codes.DeadlineExceeded, service.ErrUserServiceUnavailable},
	}

	for _, tt := range errTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := &fakeAPI{err: status.Error(tt.code, "details of the user service")}

			_, err := userclient.New(api).GetOrCreateByEmail(context.Background(), "ann@example.com")
			if !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestClient_Account(t *testing.T) {
	t.Parallel()

	id := uuid.New()

	got, err := userclient.New(&fakeAPI{state: &userv1.GetAccountStateResponse{Deactivated: true}}).Account(context.Background(), id)
	if err != nil || got != (service.Account{UserID: id, Deactivated: true}) {
		t.Fatalf("Account = %+v, %v", got, err)
	}

	_, err = userclient.New(&fakeAPI{err: status.Error(codes.NotFound, "user not found")}).Account(context.Background(), id)
	if !errors.Is(err, service.ErrAccountNotFound) {
		t.Fatalf("erased user: %v, want ErrAccountNotFound", err)
	}
}
