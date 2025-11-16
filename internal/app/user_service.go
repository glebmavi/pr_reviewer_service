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

type UserService struct {
	userRepo domain.UserRepository
	teamRepo domain.TeamRepository
	prSvc    *PullRequestService
	tx       domain.Transactor
	log      *slog.Logger
}

func NewUserService(
	userRepo domain.UserRepository,
	teamRepo domain.TeamRepository,
	prSvc *PullRequestService,
	tx domain.Transactor,
	log *slog.Logger,
) *UserService {
	return &UserService{
		userRepo: userRepo,
		teamRepo: teamRepo,
		prSvc:    prSvc,
		tx:       tx,
		log:      log,
	}
}

func (s *UserService) AddUser(ctx context.Context, username, teamName string, isActive bool) (*domain.User, error) {
	if username == "" || teamName == "" {
		return nil, fmt.Errorf("%w: username and teamName are required", domain.ErrValidation)
	}

	team, err := s.teamRepo.GetTeamByName(ctx, teamName)
	if err != nil {
		return nil, err
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

	userToCreate := &domain.User{
		ID:       uuid.New().String(),
		Username: username,
		TeamID:   team.ID,
		IsActive: isActive,
	}

	createdUser, err := s.userRepo.CreateUser(ctx, tx, userToCreate)
	if err != nil {
		return nil, err
	}
	createdUser.TeamName = team.TeamName

	if err := s.tx.CommitTx(ctx, tx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return createdUser, nil
}

func (s *UserService) UpdateUser(ctx context.Context, user *domain.User) (*domain.User, error) {
	if user.ID == "" {
		return nil, fmt.Errorf("%w: user ID is required", domain.ErrValidation)
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

	updatedUser, err := s.userRepo.UpdateUser(ctx, tx, user)
	if err != nil {
		return nil, err
	}

	if err := s.tx.CommitTx(ctx, tx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return updatedUser, nil
}

func (s *UserService) MoveUserToTeam(ctx context.Context, userID, newTeamName string) (*domain.User, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.CanBeMoved() {
		return nil, fmt.Errorf("%w: user is not active", domain.ErrValidation)
	}

	newTeam, err := s.teamRepo.GetTeamByName(ctx, newTeamName)
	if err != nil {
		return nil, err
	}
	if !newTeam.CanBeMoved() {
		return nil, fmt.Errorf("%w: new team is not active", domain.ErrValidation)
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

	updatedUser, err := s.userRepo.MoveUserToTeam(ctx, tx, userID, newTeam.ID)
	if err != nil {
		return nil, err
	}

	if err := s.tx.CommitTx(ctx, tx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	updatedUser.TeamName = newTeam.TeamName
	return updatedUser, nil
}

func (s *UserService) SetUserActiveStatus(ctx context.Context, userID string, isActive bool) (*domain.User, error) {
	tx, err := s.tx.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func(tx2 domain.Transactor, ctx context.Context, tx pgx.Tx) {
		if err := tx2.RollbackTx(ctx, tx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			s.log.Error("failed to rollback transaction", "error", err)
		}
	}(s.tx, ctx, tx)

	user, err := s.userRepo.SetUserActiveStatus(ctx, tx, userID, isActive)
	if err != nil {
		return nil, err
	}

	if !isActive {
		if _, err := s.prSvc.reassignReviewsForUsers(ctx, tx, []string{userID}); err != nil {
			return nil, err
		}
	}

	if err := s.tx.CommitTx(ctx, tx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return user, nil
}
