package rest

import (
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// Parts of a page, as the user's privacy hides them ("Кому видно …"), and
// what the refusal says.
var (
	pagePart = pagePartRule{func(model.Access) bool { return true }, ""}

	photosPart  = pagePartRule{func(a model.Access) bool { return a.Photos }, "this user's photos are hidden"}
	friendsPart = pagePartRule{func(a model.Access) bool { return a.Friends }, "this user's follows are hidden"}
	clubsPart   = pagePartRule{func(a model.Access) bool { return a.Clubs }, "this user's clubs are hidden"}
	notesPart   = pagePartRule{func(a model.Access) bool { return a.Notes }, "this user's notes are hidden"}
	giftsPart   = pagePartRule{func(a model.Access) bool { return a.Gifts }, "this user's gifts are hidden"}
)

type pagePartRule struct {
	visible func(model.Access) bool
	refusal string
}

// visiblePage is the person of the path, refused unless the caller,
// anonymous without token, may see the page ("Кому в интернете видно мою
// страницу") and part of it. A deleted page shows nothing, as on openvk.
func (a *api) visiblePage(r *http.Request, token string, part pagePartRule) (model.PublicProfile, error) {
	owner, err := a.users.GetProfile(r.Context(), token, r.PathValue("nickname"))
	if err != nil {
		return model.PublicProfile{}, err
	}

	if err = a.checkPart(r, token, owner, part); err != nil {
		return model.PublicProfile{}, err
	}

	return owner, nil
}

// checkPart refuses a part of owner's page the caller may not see.
func (a *api) checkPart(r *http.Request, token string, owner model.PublicProfile, part pagePartRule) error {
	if owner.Deactivated {
		return failure.New(failure.PermissionDenied, "this page is deleted")
	}

	if owner.PageHidden {
		return failure.New(failure.PermissionDenied, "this page is hidden")
	}

	if part.refusal == "" {
		return nil
	}

	access, err := a.users.Access(r.Context(), token, owner.ID)
	if err != nil {
		return err
	}

	if !part.visible(access) {
		return failure.New(failure.PermissionDenied, part.refusal)
	}

	return nil
}
