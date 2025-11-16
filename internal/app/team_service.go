package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/glebmavi/pr_reviewer_service/internal/domain"
)

const (
	maxReviewers = 2
)

type TeamService struct {
	teamRepo domain.TeamRepository
	userRepo domain.UserRepository
	prSvc    *PullRequestService
	tx       domain.Transactor
	log      *slog.Logger
}

func NewTeamService(
	teamRepo domain.TeamRepository,
	userRepo domain.UserRepository,
	prSvc *PullRequestService,
	tx domain.Transactor,
	log *slog.Logger,
) *TeamService {
	return &TeamService{
		teamRepo: teamRepo,
		userRepo: userRepo,
		prSvc:    prSvc,
		tx:       tx,
		log:      log,
	}
}

func (s *TeamService) CreateTeam(ctx context.Context, name string, userNames []string) (*domain.Team, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: team name is required", domain.ErrValidation)
	}

	tx, err := s.tx.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func(tx2 domain.Transactor, ctx context.Context, tx pgx.Tx) {
		if err := tx2.RollbackTx(ctx, tx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			s.log.Error("failed to rollback transaction", "error", err)
		}
	}(s.tx, ctx, tx)

	teamToCreate := &domain.Team{TeamName: name, IsActive: true}
	createdTeam, err := s.teamRepo.CreateTeam(ctx, tx, teamToCreate)
	if err != nil {
		return nil, err
	}

	createdUsers := make([]domain.User, 0, len(userNames))
	for _, username := range userNames {
		if username == "" {
			return nil, fmt.Errorf("%w: username is required", domain.ErrValidation)
		}
		userToCreate := &domain.User{
			ID:       uuid.New().String(),
			Username: username,
			TeamID:   createdTeam.ID,
			IsActive: true,
		}
		createdUser, err := s.userRepo.CreateUser(ctx, tx, userToCreate)
		if err != nil {
			return nil, err
		}
		createdUsers = append(createdUsers, *createdUser)
	}
	createdTeam.Members = createdUsers

	if err := s.tx.CommitTx(ctx, tx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return createdTeam, nil
}

func (s *TeamService) UpdateTeam(ctx context.Context, oldName, newName string) (*domain.Team, error) {
	tx, err := s.tx.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func(tx2 domain.Transactor, ctx context.Context, tx pgx.Tx) {
		if err := tx2.RollbackTx(ctx, tx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			s.log.Error("failed to rollback transaction", "error", err)
		}
	}(s.tx, ctx, tx)

	updatedTeam, err := s.teamRepo.UpdateTeam(ctx, tx, oldName, newName)
	if err != nil {
		return nil, err
	}

	if err := s.tx.CommitTx(ctx, tx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return updatedTeam, nil
}

func (s *TeamService) DeactivateTeamAndReassign(ctx context.Context, teamName string) (int, int, error) {
	tx, err := s.tx.BeginTx(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func(tx2 domain.Transactor, ctx context.Context, tx pgx.Tx) {
		if err := tx2.RollbackTx(ctx, tx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			s.log.Error("failed to rollback transaction", "error", err)
		}
	}(s.tx, ctx, tx)

	team, err := s.teamRepo.GetTeamByName(ctx, teamName)
	if err != nil {
		return 0, 0, err
	}

	if err := s.teamRepo.DeactivateTeam(ctx, tx, teamName); err != nil {
		return 0, 0, err
	}

	deactivatedUserIDs, err := s.userRepo.DeactivateUsersByTeam(ctx, tx, team.ID)
	if err != nil {
		return 0, 0, err
	}

	reassignedCount, err := s.prSvc.reassignReviewsForUsers(ctx, tx, deactivatedUserIDs)
	if err != nil {
		return 0, 0, err
	}

	if err := s.tx.CommitTx(ctx, tx); err != nil {
		return 0, 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return len(deactivatedUserIDs), reassignedCount, nil
}

func (s *TeamService) GetTeam(ctx context.Context, teamName string) (*domain.Team, error) {
	team, err := s.teamRepo.GetTeamByName(ctx, teamName)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("team with name %s not found", teamName)
		}
		return nil, fmt.Errorf("failed to get team by name %s: %w", teamName, err)
	}

	users, err := s.userRepo.GetUsersByTeam(ctx, team.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get users for team %s: %w", teamName, err)
	}

	team.Members = users
	return team, nil
}
