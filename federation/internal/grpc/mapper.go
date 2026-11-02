package grpc

import (
	"time"

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

	videos := make([]*federationv1.NewsVideo, len(p.Videos()))
	for i, v := range p.Videos() {
		videos[i] = &federationv1.NewsVideo{
			Description:     v.Description,
			Blurhash:        v.Blurhash,
			Width:           int32(v.Width),  //nolint:gosec // at most fedi.maxVideoSide
			Height:          int32(v.Height), //nolint:gosec // at most fedi.maxVideoSide
			DurationSeconds: int32(v.Duration.Round(time.Second) / time.Second),
		}
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
		Videos:      videos,
		Language:    p.Language(),
		PublishedAt: timestamppb.New(p.Published()),
	}

	if !p.Updated().IsZero() {
		out.UpdatedAt = timestamppb.New(p.Updated())
	}

	return out
}
