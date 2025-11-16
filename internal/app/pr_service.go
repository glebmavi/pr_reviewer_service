package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/glebmavi/pr_reviewer_service/internal/domain"
	"github.com/google/uuid"
)

type PullRequestService struct {
	prRepo   domain.PullRequestRepository
	userRepo domain.UserRepository
	tx       domain.Transactor
	log      *slog.Logger
}

func NewPullRequestService(
	prRepo domain.PullRequestRepository,
	userRepo domain.UserRepository,
	tx domain.Transactor,
	log *slog.Logger,
) *PullRequestService {
	return &PullRequestService{
		prRepo:   prRepo,
		userRepo: userRepo,
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
	defer s.tx.RollbackTx(ctx, tx)

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

	if !pr.CanChangeReviewers() {
		return nil, domain.ErrPRMerged
	}

	tx, err := s.tx.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer s.tx.RollbackTx(ctx, tx)

	mergedPR, err := s.prRepo.MergePR(ctx, tx, prID)
	if err != nil {
		return nil, err
	}

	if err := s.tx.CommitTx(ctx, tx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return mergedPR, nil
}

// TODO: fix returns, they should match the definition
func (s *PullRequestService) ReassignReviewer(ctx context.Context, prID string, oldUserID string) (*domain.PullRequest, newReviewerID string, error) {
	pr, err := s.GetPR(ctx, prID)
	if err != nil {
		return nil, "", err
	}

	if !pr.CanChangeReviewers() {
		return nil, "", domain.ErrPRMerged
	}

	isAssigned := false
	for _, r := range pr.Reviewers {
		if r.ID == oldUserID {
			isAssigned = true
			break
		}
	}
	if !isAssigned {
		return nil, fmt.Errorf("%w: user %s", domain.ErrNotAssigned, oldUserID)
	}

	author, err := s.userRepo.GetUserByID(ctx, pr.AuthorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get author: %w", err)
	}

	tx, err := s.tx.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer s.tx.RollbackTx(ctx, tx)

	if err := s.prRepo.RemoveReviewer(ctx, tx, prID, oldUserID); err != nil {
		return nil, fmt.Errorf("failed to remove reviewer: %w", err)
	}

	currentReviewerIDs := make([]string, 0, len(pr.Reviewers)-1)
	for _, r := range pr.Reviewers {
		if r.ID != oldUserID {
			currentReviewerIDs = append(currentReviewerIDs, r.ID)
		}
	}

	excludeIDs := append(currentReviewerIDs, oldUserID)
	candidates, err := s.userRepo.FindReviewCandidates(ctx, author.TeamID, pr.AuthorID, excludeIDs, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to find new candidate: %w", err)
	}

	if len(candidates) > 0 {
		newReviewerID := candidates[0].ID
		if err := s.prRepo.AssignReviewers(ctx, tx, prID, []string{newReviewerID}); err != nil {
			return nil, fmt.Errorf("failed to assign new reviewer: %w", err)
		}
	}

	if err := s.tx.CommitTx(ctx, tx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	retPR, err := s.GetPR(ctx, prID)

	return retPR, newReviewerID, nil
}
