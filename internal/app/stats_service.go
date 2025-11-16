package app

import (
	"context"
	"log/slog"

	"github.com/glebmavi/pr_reviewer_service/internal/domain"
)

type StatsService struct {
	statsRepo domain.StatsRepository
	log       *slog.Logger
}

func NewStatsService(statsRepo domain.StatsRepository, log *slog.Logger) *StatsService {
	return &StatsService{
		statsRepo: statsRepo,
		log:       log,
	}
}

func (s *StatsService) GetStats(ctx context.Context) ([]domain.StatItem, error) {
	return s.statsRepo.GetReviewStats(ctx)
}

func (s *StatsService) GetOpenReviewCountForTeam(ctx context.Context, teamName string) (int, error) {
	return s.statsRepo.GetOpenReviewCountForTeam(ctx, teamName)
}

func (s *StatsService) GetMergedReviewCountForTeam(ctx context.Context, teamName string) (int, error) {
	return s.statsRepo.GetMergedReviewCountForTeam(ctx, teamName)
}

func (s *StatsService) GetOpenReviewCountForUser(ctx context.Context, userID string) (int, error) {
	return s.statsRepo.GetOpenReviewCountForUser(ctx, userID)
}

func (s *StatsService) GetMergedReviewCountForUser(ctx context.Context, userID string) (int, error) {
	return s.statsRepo.GetMergedReviewCountForUser(ctx, userID)
}
