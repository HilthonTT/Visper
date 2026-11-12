package message

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/domain/repository"
	"github.com/hilthontt/visper/services/api/infrastructure/logger"
	"go.uber.org/zap"
)

// memRepo is an in-memory MessageRepository.
type memRepo struct {
	messages map[string]*model.Message
}

func (r *memRepo) GetByID(_ context.Context, _, id string) (*model.Message, error) {
	m, ok := r.messages[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	c := *m
	return &c, nil
}
func (r *memRepo) Update(_ context.Context, m *model.Message) error { r.messages[m.ID] = m; return nil }
func (r *memRepo) Mutate(_ context.Context, _, id string, fn func(*model.Message) error) (*model.Message, error) {
	m, ok := r.messages[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	c := *m
	c.Reactions = append([]model.Reaction(nil), m.Reactions...)
	if err := fn(&c); err != nil {
		return nil, err
	}
	c.CreatedAt, c.UpdatedAt = m.CreatedAt, m.UpdatedAt
	r.messages[id] = &c
	return &c, nil
}
func (r *memRepo) Delete(_ context.Context, _, id string) error { delete(r.messages, id); return nil }
func (r *memRepo) Create(_ context.Context, m *model.Message) error {
	r.messages[m.ID] = m
	return nil
}
func (r *memRepo) GetByRoom(context.Context, string, int64) ([]*model.Message, error) {
	return nil, nil
}
func (r *memRepo) GetByRoomAfter(context.Context, string, time.Time, int64) ([]*model.Message, error) {
	return nil, nil
}
func (r *memRepo) Count(context.Context, string) (int64, error) { return int64(len(r.messages)), nil }

func newTestUseCase() (MessageUseCase, *memRepo) {
	repo := &memRepo{messages: map[string]*model.Message{}}
	return NewMessageUseCase(repo, nil, &logger.Logger{Log: zap.NewNop()}), repo
}

func TestSendReply(t *testing.T) {
	uc, _ := newTestUseCase()
	ctx := context.Background()

	first, err := uc.Send(ctx, "r", "u1", "Ann", "hi", true, "")
	if err != nil {
		t.Fatal(err)
	}
	reply, err := uc.Send(ctx, "r", "u2", "Bo", "hey", true, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reply.ReplyToID != first.ID {
		t.Fatalf("ReplyToID = %q, want %q", reply.ReplyToID, first.ID)
	}
	if _, err := uc.Send(ctx, "r", "u2", "Bo", "hey", true, "gone"); !errors.Is(err, ErrReplyTargetNotFound) {
		t.Fatalf("reply to a missing message: %v", err)
	}
}

func TestSetReactionKeepsOnePerMemberAndIsNotAnEdit(t *testing.T) {
	uc, repo := newTestUseCase()
	ctx := context.Background()
	msg, _ := uc.Send(ctx, "r", "u1", "Ann", "hi", true, "")

	if _, err := uc.SetReaction(ctx, "r", msg.ID, "u2", "enc-thumbs", true); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.SetReaction(ctx, "r", msg.ID, "u3", "enc-heart", true); err != nil {
		t.Fatal(err)
	}
	got, err := uc.SetReaction(ctx, "r", msg.ID, "u2", "enc-laugh", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Reactions) != 2 {
		t.Fatalf("reactions = %+v, want one each for u2 and u3", got.Reactions)
	}
	for _, r := range got.Reactions {
		if r.UserID == "u2" && r.Emoji != "enc-laugh" {
			t.Fatalf("u2's reaction = %q, want it replaced", r.Emoji)
		}
	}
	if !repo.messages[msg.ID].UpdatedAt.IsZero() {
		t.Fatal("a reaction marked the message as edited")
	}

	got, _ = uc.SetReaction(ctx, "r", msg.ID, "u2", "", false)
	if len(got.Reactions) != 1 || got.Reactions[0].UserID != "u3" {
		t.Fatalf("after removing u2's reaction: %+v", got.Reactions)
	}

	if _, err := uc.SetReaction(ctx, "r", "missing", "u2", "x", true); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("missing message: %v", err)
	}
	long := make([]byte, maxReactionLength+1)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := uc.SetReaction(ctx, "r", msg.ID, "u2", string(long), true); !errors.Is(err, ErrReactionTooLong) {
		t.Fatalf("long reaction: %v", err)
	}
}
