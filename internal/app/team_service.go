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
	prRepo   domain.PullRequestRepository
	tx       domain.Transactor
	log      *slog.Logger
}

func NewTeamService(
	teamRepo domain.TeamRepository,
	userRepo domain.UserRepository,
	prRepo domain.PullRequestRepository,
	tx domain.Transactor,
	log *slog.Logger,
) *TeamService {
	return &TeamService{
		teamRepo: teamRepo,
		userRepo: userRepo,
		prRepo:   prRepo,
		tx:       tx,
		log:      log,
	}
}

// TODO: when adding users does it check that they dont already exist?
func (s *TeamService) CreateTeam(ctx context.Context, name string, userNames []string) (*domain.Team, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: team name is required", domain.ErrValidation)
	}

	tx, err := s.tx.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer s.tx.RollbackTx(ctx, tx)

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
	defer s.tx.RollbackTx(ctx, tx)

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
	defer s.tx.RollbackTx(ctx, tx)

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

	reassignedCount, err := s.reassignReviewsForUsers(ctx, tx, deactivatedUserIDs)
	if err != nil {
		return 0, 0, err
	}

	if err := s.tx.CommitTx(ctx, tx); err != nil {
		return 0, 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return len(deactivatedUserIDs), reassignedCount, nil
}

// TODO: belongs to user_service
func (s *TeamService) MoveUserToTeam(ctx context.Context, userID, newTeamName string) (*domain.User, error) {
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
	defer s.tx.RollbackTx(ctx, tx)

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

// TODO: belongs to user_service
func (s *TeamService) SetUserActiveStatus(ctx context.Context, userID string, isActive bool) (*domain.User, error) {
	tx, err := s.tx.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer s.tx.RollbackTx(ctx, tx)

	user, err := s.userRepo.SetUserActiveStatus(ctx, tx, userID, isActive)
	if err != nil {
		return nil, err
	}

	if !isActive {
		if _, err := s.reassignReviewsForUsers(ctx, tx, []string{userID}); err != nil {
			return nil, err
		}
	}

	if err := s.tx.CommitTx(ctx, tx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return user, nil
}

// TODO: belongs to pr_service
func (s *TeamService) reassignReviewsForUsers(ctx context.Context, tx pgx.Tx, userIDs []string) (int, error) {
	reassignedCount := 0
	for _, userID := range userIDs {
		prs, err := s.prRepo.GetOpenPRsByReviewer(ctx, tx, userID)
		if err != nil {
			return 0, fmt.Errorf("failed to get open PRs for user %s: %w", userID, err)
		}

		for _, pr := range prs {
			if err := s.prRepo.RemoveReviewer(ctx, tx, pr.ID, userID); err != nil {
				return 0, fmt.Errorf("failed to remove reviewer %s from PR %s: %w", userID, pr.ID, err)
			}

			currentReviewers, err := s.prRepo.GetReviewers(ctx, pr.ID)
			if err != nil {
				return 0, fmt.Errorf("failed to get reviewers for PR %s: %w", pr.ID, err)
			}

			if len(currentReviewers) == 0 {
				author, err := s.userRepo.GetUserByID(ctx, pr.AuthorID)
				if err != nil {
					return 0, fmt.Errorf("failed to get author for PR %s: %w", pr.ID, err)
				}

				authorTeam, err := s.teamRepo.GetTeamByID(ctx, author.TeamID)
				if err != nil {
					return 0, fmt.Errorf("failed to get author's team for PR %s: %w", pr.ID, err)
				}

				if authorTeam.IsActive {
					excludeIDs := currentReviewersToIDs(currentReviewers)
					candidates, err := s.userRepo.FindReviewCandidates(ctx, author.TeamID, pr.AuthorID, excludeIDs, maxReviewers-len(currentReviewers))
					if err != nil {
						return 0, fmt.Errorf("failed to find review candidates for PR %s: %w", pr.ID, err)
					}

					if len(candidates) > 0 {
						candidateIDs := make([]string, len(candidates))
						for i, c := range candidates {
							candidateIDs[i] = c.ID
						}
						if err := s.prRepo.AssignReviewers(ctx, tx, pr.ID, candidateIDs); err != nil {
							return 0, fmt.Errorf("failed to assign new reviewers for PR %s: %w", pr.ID, err)
						}
						reassignedCount++
					}
				}
			}
		}
	}
	return reassignedCount, nil
}

func currentReviewersToIDs(reviewers []domain.User) []string {
	ids := make([]string, len(reviewers))
	for i, r := range reviewers {
		ids[i] = r.ID
	}
	return ids
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
