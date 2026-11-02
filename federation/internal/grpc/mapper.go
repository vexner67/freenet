package grpc

import (
	federationv1 "github.com/vexner67/freenet/federation/api/federation/v1"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/service"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoAccount(a service.Account) *federationv1.Account {
	return toProtoRemote(a.Remote, a.Accepted)
}

func toProtoRemote(r *fedi.Remote, accepted bool) *federationv1.Account {
	p := r.Profile()

	return &federationv1.Account{
		Id:        r.ID().String(),
		Handle:    r.Handle(),
		Name:      p.Name,
		PageUrl:   p.Page.String(),
		HasAvatar: p.Icon != "",
		Accepted:  accepted,
	}
}

func toProtoAccounts(accounts []service.Account) []*federationv1.Account {
	out := make([]*federationv1.Account, len(accounts))
	for i, a := range accounts {
		out[i] = toProtoAccount(a)
	}

	return out
}

func toProtoNewsPost(n service.NewsPost) *federationv1.NewsPost {
	p := n.Post

	images := make([]*federationv1.NewsImage, len(p.Images()))
	for i, img := range p.Images() {
		images[i] = &federationv1.NewsImage{Description: img.Description}
	}

	out := &federationv1.NewsPost{
		Id:          p.ID().String(),
		Url:         p.URI().String(),
		PageUrl:     p.Page().String(),
		Author:      toProtoRemote(n.Author, true),
		Text:        p.Text(),
		Summary:     p.Summary(),
		Sensitive:   p.Sensitive(),
		Images:      images,
		Language:    p.Language(),
		PublishedAt: timestamppb.New(p.Published()),
	}

	if !p.Updated().IsZero() {
		out.UpdatedAt = timestamppb.New(p.Updated())
	}

	return out
}
