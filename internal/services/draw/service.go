package draw

import (
	"context"
	"errors"
	"log/slog"
	mrand "math/rand"
	"sync"
	"time"

	"github.com/google/uuid"

	"team-invite/internal/cache"
	"team-invite/internal/database"
	"team-invite/internal/models"
	"team-invite/internal/util/invitecode"
)

type Service struct {
	store  *database.Store
	cache  *cache.PrizeConfigCache
	logger *slog.Logger
	rng    *mrand.Rand
	mu     sync.Mutex
}

type SpinOutcome struct {
	Prize      models.PrizeConfigItem `json:"prize"`
	Invite     *models.InviteCode     `json:"invite,omitempty"`
	Quota      int                    `json:"quota"`
	SpinStatus string                 `json:"spinStatus"`
}

func NewService(store *database.Store, cache *cache.PrizeConfigCache, logger *slog.Logger) *Service {
	src := mrand.NewSource(time.Now().UnixNano())
	return &Service{
		store:  store,
		cache:  cache,
		logger: logger,
		rng:    mrand.New(src),
	}
}

func (s *Service) Spin(ctx context.Context, user *models.User, spinID string) (*SpinOutcome, error) {
	if err := s.store.ConsumeChance(ctx, user.ID); err != nil {
		return nil, err
	}
	prizes, err := s.cache.Get(ctx)
	if err != nil {
		return nil, err
	}
	prize := s.pickPrize(prizes)

	if spinID == "" {
		spinID = uuid.NewString()
	}
	outcome := &SpinOutcome{
		Prize:      prize,
		SpinStatus: prize.Type,
	}

	switch prize.Type {
	case "win":
		code, err := invitecode.Generate()
		if err != nil {
			return nil, err
		}
		invite, quota, err := s.store.AwardWin(ctx, user.ID, code)
		if err != nil {
			if errors.Is(err, database.ErrQuotaEmpty) {
				_ = s.store.AddChance(ctx, user.ID, 1)
			}
			return nil, err
		}
		outcome.Invite = invite
		outcome.Quota = quota
	case "retry":
		if err := s.store.AddChance(ctx, user.ID, 1); err != nil {
			return nil, err
		}
		q, err := s.store.GetQuota(ctx)
		if err != nil {
			return nil, err
		}
		outcome.Quota = q
	case "lose":
		fallthrough
	default:
		q, err := s.store.GetQuota(ctx)
		if err != nil {
			return nil, err
		}
		outcome.Quota = q
	}

	record := models.SpinRecord{
		UserID:   user.ID,
		Username: user.Username,
		Prize:    prize.Name,
		Status:   prize.Type,
		SpinID:   spinID,
		Detail:   "",
	}
	if err := s.store.RecordSpin(ctx, record); err != nil {
		s.logger.Warn("failed to record spin", "error", err)
	}
	return outcome, nil
}

func (s *Service) pickPrize(items []models.PrizeConfigItem) models.PrizeConfigItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rng.Float64()
	var cumulative float64
	for _, item := range items {
		cumulative += item.Probability
		if r <= cumulative {
			return item
		}
	}
	return items[len(items)-1]
}
