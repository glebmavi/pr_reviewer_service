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

type PullRequestService struct {
	prRepo   domain.PullRequestRepository
	userRepo domain.UserRepository
	teamRepo domain.TeamRepository
	tx       domain.Transactor
	log      *slog.Logger
}

func NewPullRequestService(
	prRepo domain.PullRequestRepository,
	userRepo domain.UserRepository,
	teamRepo domain.TeamRepository,
	tx domain.Transactor,
	log *slog.Logger,
) *PullRequestService {
	return &PullRequestService{
		prRepo:   prRepo,
		userRepo: userRepo,
		teamRepo: teamRepo,
		tx:       tx,
		log:      log,
	}
}

func (s *PullRequestService) CreatePR(ctx context.Context, name, authorID string) (*domain.PullRequest, error) {
	if name == "" || authorID == "" {
		return nil, fmt.Errorf("%w: name and authorID are required", domain.ErrValidation)
	}

	author, err := s.userRepo.GetUserByID(ctx, authorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get author: %w", err)
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

	prToCreate := &domain.PullRequest{
		ID:       uuid.New().String(),
		Name:     name,
		AuthorID: authorID,
		Status:   domain.StatusOpen,
	}

	createdPR, err := s.prRepo.CreatePR(ctx, tx, prToCreate)
	if err != nil {
		return nil, err
	}

	candidates, err := s.userRepo.FindReviewCandidates(ctx, author.TeamID, authorID, []string{}, maxReviewers)
	if err != nil {
		return nil, fmt.Errorf("failed to find review candidates: %w", err)
	}

	if len(candidates) > 0 {
		candidateIDs := make([]string, len(candidates))
		reviewers := make([]domain.Reviewer, len(candidates))
		for i, c := range candidates {
			candidateIDs[i] = c.ID
			reviewers[i] = domain.Reviewer{ID: c.ID, Username: c.Username}
		}
		if err := s.prRepo.AssignReviewers(ctx, tx, createdPR.ID, candidateIDs); err != nil {
			return nil, fmt.Errorf("failed to assign reviewers: %w", err)
		}
		createdPR.Reviewers = reviewers
	}

	if err := s.tx.CommitTx(ctx, tx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return createdPR, nil
}

func (s *PullRequestService) GetPR(ctx context.Context, prID string) (*domain.PullRequest, error) {
	pr, err := s.prRepo.GetPRByID(ctx, prID)
	if err != nil {
		return nil, err
	}

	reviewers, err := s.prRepo.GetReviewers(ctx, prID)
	if err != nil {
		return nil, err
	}

	pr.Reviewers = make([]domain.Reviewer, len(reviewers))
	for i, r := range reviewers {
		pr.Reviewers[i] = domain.Reviewer{ID: r.ID, Username: r.Username}
	}

	return pr, nil
}

func (s *PullRequestService) MergePR(ctx context.Context, prID string) (*domain.PullRequest, error) {
	pr, err := s.prRepo.GetPRByID(ctx, prID)
	if err != nil {
		return nil, err
	}

	if !pr.IsOpen() {
		return nil, domain.ErrPRMerged
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

	mergedPR, err := s.prRepo.MergePR(ctx, tx, prID)
	if err != nil {
		return nil, err
	}

	if err := s.tx.CommitTx(ctx, tx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return mergedPR, nil
}

func (s *PullRequestService) AssignReviewer(ctx context.Context, prID string, userID string) (*domain.PullRequest, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user to assign: %w", err)
	}
	if !user.IsActive {
		return nil, domain.ErrUserNotActive
	}

	pr, err := s.GetPR(ctx, prID)
	if err != nil {
		return nil, err
	}
	if !pr.IsOpen() {
		return nil, domain.ErrPRMerged
	}

	if pr.AuthorID == userID {
		return nil, fmt.Errorf("%w: author cannot be assigned as a reviewer to their own PR", domain.ErrValidation)
	}

	for _, r := range pr.Reviewers {
		if r.ID == userID {
			return pr, nil
		}
	}

	if len(pr.Reviewers) >= maxReviewers {
		return nil, fmt.Errorf("%w: pull request already has the maximum number of reviewers", domain.ErrValidation)
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

	if err := s.prRepo.AssignReviewers(ctx, tx, prID, []string{userID}); err != nil {
		return nil, fmt.Errorf("failed to assign reviewer in repo: %w", err)
	}

	if err := s.tx.CommitTx(ctx, tx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return s.GetPR(ctx, prID)
}

func (s *PullRequestService) ReassignReviewer(ctx context.Context, prID string, oldUserID string) (*domain.PullRequest, string, error) {
	pr, err := s.GetPR(ctx, prID)
	if err != nil {
		return nil, "", err
	}

	if err := s.validateReassignment(pr, oldUserID); err != nil {
		return nil, "", err
	}

	tx, err := s.tx.BeginTx(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func(tx2 domain.Transactor, ctx context.Context, tx pgx.Tx) {
		if err := tx2.RollbackTx(ctx, tx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			s.log.Error("failed to rollback transaction", "error", err)
		}
	}(s.tx, ctx, tx)

	newReviewerID, err := s.reassignReviewerInTx(ctx, tx, pr, oldUserID)
	if err != nil {
		return nil, "", err
	}

	if err := s.tx.CommitTx(ctx, tx); err != nil {
		return nil, "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	retPR, err := s.GetPR(ctx, prID)
	if err != nil {
		return nil, "", err
	}

	return retPR, newReviewerID, nil
}

func (s *PullRequestService) validateReassignment(pr *domain.PullRequest, oldUserID string) error {
	if !pr.IsOpen() {
		return domain.ErrPRMerged
	}

	isAssigned := false
	for _, r := range pr.Reviewers {
		if r.ID == oldUserID {
			isAssigned = true
			break
		}
	}
	if !isAssigned {
		return fmt.Errorf("%w: user %s, PR %s", domain.ErrNotAssigned, oldUserID, pr.ID)
	}
	return nil
}

func (s *PullRequestService) reassignReviewerInTx(ctx context.Context, tx pgx.Tx, pr *domain.PullRequest, oldUserID string) (string, error) {
	if err := s.prRepo.RemoveReviewer(ctx, tx, pr.ID, oldUserID); err != nil {
		return "", fmt.Errorf("failed to remove reviewer: %w", err)
	}

	// Refetch reviewers inside the transaction to get the current state after removal.
	currentReviewers, err := s.prRepo.GetReviewers(ctx, pr.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get current reviewers: %w", err)
	}

	currentReviewerIDs := make([]string, 0, len(currentReviewers))
	for _, r := range currentReviewers {
		if r.ID != oldUserID {
			currentReviewerIDs = append(currentReviewerIDs, r.ID)
		}
	}
	author, err := s.userRepo.GetUserByID(ctx, pr.AuthorID)
	if err != nil {
		return "", fmt.Errorf("failed to get author: %w", err)
	}

	excludeIDs := append(currentReviewerIDs, oldUserID)
	candidates, err := s.userRepo.FindReviewCandidates(ctx, author.TeamID, pr.AuthorID, excludeIDs, 1)
	if err != nil {
		return "", fmt.Errorf("failed to find new candidate: %w", err)
	}

	if len(candidates) == 0 {
		s.log.Warn("no new reviewer found for PR", "pr_id", pr.ID)
		return "", nil
	}

	newReviewerID := candidates[0].ID
	if err := s.prRepo.AssignReviewers(ctx, tx, pr.ID, []string{newReviewerID}); err != nil {
		return "", fmt.Errorf("failed to assign new reviewer: %w", err)
	}

	return newReviewerID, nil
}

func (s *PullRequestService) GetReviewsForUser(ctx context.Context, userID string) ([]domain.PullRequest, error) {
	return s.prRepo.GetPRsByReviewer(ctx, userID)
}

func (s *PullRequestService) GetOpenPRsWithoutReviewers(ctx context.Context) ([]domain.PullRequest, error) {
	return s.prRepo.GetOpenPRsWithoutReviewers(ctx)
}

func (s *PullRequestService) reassignReviewsForUsers(ctx context.Context, tx pgx.Tx, userIDs []string) (int, error) {
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
