package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fltLi/peek-lofter/pkg/model"
)

// Source is the subset of the LOFTER client used by the user service.
type Source interface {
	BlogPage(ctx context.Context, q model.BlogQuery) (model.BlogPage, error)
	Reactions(ctx context.Context, q model.ReactionQuery) (model.RefPage, error)
}

// Options bounds a user list request.
type Options struct {
	Limit int
	Since time.Time
	Until time.Time
}

// Service enumerates a user's profile, posts, likes and recommendations.
type Service struct {
	Source Source
}

// New creates a user service backed by source.
func New(source Source) *Service {
	return &Service{Source: source}
}

// Profile returns the public identity and monthly archive counts. LOFTER has no
// dedicated profile endpoint, so identity comes from the first post's blogInfo.
func (s *Service) Profile(ctx context.Context, blog string) (model.User, error) {
	blog = strings.TrimSpace(blog)
	if blog == "" {
		return model.User{}, errors.New("user: empty blog name")
	}
	page, err := s.Source.BlogPage(ctx, model.BlogQuery{BlogName: blog, Limit: 20, Offset: 0})
	if err != nil {
		return model.User{}, err
	}
	u := model.User{BlogName: blog, Archives: page.Archives}
	if len(page.Refs) > 0 {
		u.BlogID = page.Refs[0].Author.BlogID
		u.BlogName = page.Refs[0].Author.BlogName
		u.Nickname = page.Refs[0].Author.Nickname
	}
	if u.BlogName == "" {
		u.BlogName = blog
	}
	return u, nil
}

// Posts emits a user's posts until the API is exhausted or opt.Limit is reached.
func (s *Service) Posts(ctx context.Context, blog string, opt Options, emit func(model.Ref) error) error {
	if emit == nil {
		emit = func(model.Ref) error { return nil }
	}
	offset := 0
	count := 0
	for {
		page, err := s.Source.BlogPage(ctx, model.BlogQuery{BlogName: blog, Limit: 50, Offset: offset})
		if err != nil {
			return err
		}
		if len(page.Refs) == 0 {
			return nil
		}
		for _, ref := range page.Refs {
			if !opt.Since.IsZero() && ref.PublishedAt.Before(opt.Since) {
				continue
			}
			if !opt.Until.IsZero() && !ref.PublishedAt.Before(opt.Until) {
				continue
			}
			ref.Source = &model.Source{Kind: "user-posts", Name: blog}
			if err := emit(ref); err != nil {
				return err
			}
			count++
			if opt.Limit > 0 && count >= opt.Limit {
				return nil
			}
		}
		next := page.Next
		if next == -1 || next <= offset {
			return nil
		}
		offset = next
	}
}

// Likes emits posts the user liked.
func (s *Service) Likes(ctx context.Context, blog string, opt Options, emit func(model.Ref) error) error {
	return s.reactions(ctx, blog, "like", "user-likes", opt, emit)
}

// Recommends emits posts the user recommended.
func (s *Service) Recommends(ctx context.Context, blog string, opt Options, emit func(model.Ref) error) error {
	return s.reactions(ctx, blog, "share", "user-recommends", opt, emit)
}

func (s *Service) reactions(
	ctx context.Context,
	blog, kind, sourceKind string,
	opt Options,
	emit func(model.Ref) error,
) error {
	if emit == nil {
		emit = func(model.Ref) error { return nil }
	}
	profile, err := s.Profile(ctx, blog)
	if err != nil {
		return err
	}
	if profile.BlogID == 0 {
		return fmt.Errorf("user: cannot resolve blog id for %s", blog)
	}
	offset := 0
	count := 0
	for {
		page, err := s.Source.Reactions(
			ctx,
			model.ReactionQuery{BlogID: profile.BlogID, Kind: kind, Limit: 20, Offset: offset},
		)
		if err != nil {
			return err
		}
		if len(page.Refs) == 0 {
			return nil
		}
		for _, ref := range page.Refs {
			ref.Source = &model.Source{Kind: sourceKind, Name: blog}
			if err := emit(ref); err != nil {
				return err
			}
			count++
			if opt.Limit > 0 && count >= opt.Limit {
				return nil
			}
		}
		next := page.Next
		if next == -1 || next <= offset {
			return nil
		}
		offset = next
	}
}
