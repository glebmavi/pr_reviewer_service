package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/glebmavi/pr_reviewer_service/internal/domain"
	"github.com/glebmavi/pr_reviewer_service/internal/storage/postgres/models"
)

type Repository struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

func NewRepository(pool *pgxpool.Pool, log *slog.Logger) *Repository {
	return &Repository{
		pool: pool,
		log:  log,
	}
}

func (r *Repository) querier(tx pgx.Tx) models.Querier {
	if tx != nil {
		return models.New(tx)
	}
	return models.New(r.pool)
}

// --- Transactor Implementation ---

func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

func (r *Repository) CommitTx(ctx context.Context, tx pgx.Tx) error {
	return tx.Commit(ctx)
}

func (r *Repository) RollbackTx(ctx context.Context, tx pgx.Tx) error {
	return tx.Rollback(ctx)
}

// --- TeamRepository Implementation ---

func (r *Repository) CreateTeam(ctx context.Context, tx pgx.Tx, team *domain.Team) (*domain.Team, error) {
	q := r.querier(tx)
	dbTeam, err := q.CreateTeam(ctx, team.TeamName)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return nil, fmt.Errorf("%w: team '%s'", domain.ErrTeamExists, team.TeamName)
		}
		return nil, domain.ErrInternalError
	}
	return &domain.Team{ID: dbTeam.TeamID, TeamName: dbTeam.TeamName, IsActive: dbTeam.IsActive}, nil
}

func (r *Repository) GetTeamByName(ctx context.Context, teamName string) (*domain.Team, error) {
	q := r.querier(nil)
	dbTeam, err := q.GetTeamByName(ctx, teamName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: team '%s'", domain.ErrNotFound, teamName)
		}
		return nil, domain.ErrInternalError
	}
	return &domain.Team{ID: dbTeam.TeamID, TeamName: dbTeam.TeamName, IsActive: dbTeam.IsActive}, nil
}

func (r *Repository) GetTeamByID(ctx context.Context, teamID int32) (*domain.Team, error) {
	q := r.querier(nil)
	dbTeam, err := q.GetTeamByID(ctx, teamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: team with id '%d'", domain.ErrNotFound, teamID)
		}
		return nil, domain.ErrInternalError
	}
	return &domain.Team{ID: dbTeam.TeamID, TeamName: dbTeam.TeamName, IsActive: dbTeam.IsActive}, nil
}

func (r *Repository) UpdateTeam(ctx context.Context, tx pgx.Tx, oldTeamName, newTeamName string) (*domain.Team, error) {
	q := r.querier(tx)
	team, err := r.GetTeamByName(ctx, oldTeamName)
	if err != nil {
		return nil, err
	}
	dbTeam, err := q.UpdateTeamName(ctx, models.UpdateTeamNameParams{
		TeamID:   team.ID,
		TeamName: newTeamName,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: team '%s'", domain.ErrNotFound, oldTeamName)
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return nil, fmt.Errorf("%w: team '%s'", domain.ErrTeamExists, newTeamName)
		}
		return nil, domain.ErrInternalError
	}
	return &domain.Team{ID: dbTeam.TeamID, TeamName: dbTeam.TeamName, IsActive: dbTeam.IsActive}, nil
}

func (r *Repository) DeactivateTeam(ctx context.Context, tx pgx.Tx, teamName string) error {
	q := r.querier(tx)
	team, err := r.GetTeamByName(ctx, teamName)
	if err != nil {
		return err
	}
	if _, err := q.DeactivateTeam(ctx, team.ID); err != nil {
		return domain.ErrInternalError
	}
	return nil
}

// --- UserRepository Implementation ---

func (r *Repository) CreateUser(ctx context.Context, tx pgx.Tx, user *domain.User) (*domain.User, error) {
	q := r.querier(tx)
	dbUser, err := q.CreateUser(ctx, models.CreateUserParams{
		UserID:   user.ID,
		Username: user.Username,
		TeamID:   user.TeamID,
		IsActive: true,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return nil, fmt.Errorf("%w: user '%s'", domain.ErrValidation, user.ID)
		}
		return nil, domain.ErrInternalError
	}
	return &domain.User{ID: dbUser.UserID, Username: dbUser.Username, TeamID: dbUser.TeamID, IsActive: dbUser.IsActive}, nil
}

func (r *Repository) GetUserByID(ctx context.Context, userID string) (*domain.User, error) {
	q := r.querier(nil)
	dbUser, err := q.GetUserWithTeam(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: user '%s'", domain.ErrNotFound, userID)
		}
		return nil, domain.ErrInternalError
	}
	return &domain.User{ID: dbUser.UserID, Username: dbUser.Username, TeamID: dbUser.TeamID, TeamName: dbUser.TeamName, IsActive: dbUser.IsActive}, nil
}

func (r *Repository) GetUsersByTeam(ctx context.Context, teamID int32) ([]domain.User, error) {
	q := r.querier(nil)
	dbUsers, err := q.GetTeamMembers(ctx, teamID)
	if err != nil {
		return nil, domain.ErrInternalError
	}
	users := make([]domain.User, len(dbUsers))
	for i, u := range dbUsers {
		users[i] = domain.User{ID: u.UserID, Username: u.Username, TeamID: u.TeamID, IsActive: u.IsActive}
	}
	return users, nil
}

func (r *Repository) UpdateUser(ctx context.Context, tx pgx.Tx, user *domain.User) (*domain.User, error) {
	q := r.querier(tx)
	dbUser, err := q.UpdateUser(ctx, models.UpdateUserParams{
		UserID:   user.ID,
		Username: user.Username,
		TeamID:   user.TeamID,
		IsActive: user.IsActive,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: user '%s'", domain.ErrNotFound, user.ID)
		}
		return nil, domain.ErrInternalError
	}
	return &domain.User{ID: dbUser.UserID, Username: dbUser.Username, TeamID: dbUser.TeamID, IsActive: dbUser.IsActive}, nil
}

func (r *Repository) SetUserActiveStatus(ctx context.Context, tx pgx.Tx, userID string, isActive bool) (*domain.User, error) {
	q := r.querier(tx)
	dbUser, err := q.SetUserActiveStatus(ctx, models.SetUserActiveStatusParams{
		UserID:   userID,
		IsActive: isActive,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: user '%s'", domain.ErrNotFound, userID)
		}
		return nil, domain.ErrInternalError
	}

	return &domain.User{ID: dbUser.UserID, Username: dbUser.Username, TeamID: dbUser.TeamID, IsActive: dbUser.IsActive}, nil
}

func (r *Repository) MoveUserToTeam(ctx context.Context, tx pgx.Tx, userID string, newTeamID int32) (*domain.User, error) {
	q := r.querier(tx)
	dbUser, err := q.MoveUserToTeam(ctx, models.MoveUserToTeamParams{
		UserID: userID,
		TeamID: newTeamID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: user '%s'", domain.ErrNotFound, userID)
		}
		return nil, domain.ErrInternalError
	}
	return &domain.User{ID: dbUser.UserID, Username: dbUser.Username, TeamID: dbUser.TeamID, IsActive: dbUser.IsActive}, nil
}

func (r *Repository) DeactivateUsersByTeam(ctx context.Context, tx pgx.Tx, teamID int32) ([]string, error) {
	q := r.querier(tx)
	userIDs, err := q.DeactivateUsersByTeam(ctx, teamID)
	if err != nil {
		return nil, domain.ErrInternalError
	}
	return userIDs, nil
}

func (r *Repository) FindReviewCandidates(ctx context.Context, teamID int32, authorID string, excludeUserIDs []string, limit int) ([]domain.User, error) {
	q := r.querier(nil)
	dbUsers, err := q.FindReplacementCandidates(ctx, models.FindReplacementCandidatesParams{
		TeamID:  teamID,
		UserID:  authorID,
		Column3: excludeUserIDs,
		Limit:   int32(limit),
	})
	if err != nil {
		return nil, domain.ErrInternalError
	}
	users := make([]domain.User, len(dbUsers))
	for i, u := range dbUsers {
		users[i] = domain.User{ID: u.UserID, Username: u.Username, TeamID: u.TeamID, IsActive: u.IsActive}
	}
	return users, nil
}

// --- PullRequestRepository Implementation ---

func (r *Repository) CreatePR(ctx context.Context, tx pgx.Tx, pr *domain.PullRequest) (*domain.PullRequest, error) {
	q := r.querier(tx)
	dbPR, err := q.CreatePR(ctx, models.CreatePRParams{
		PrID:     pr.ID,
		PrName:   pr.Name,
		AuthorID: pr.AuthorID,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case pgerrcode.UniqueViolation:
				return nil, fmt.Errorf("%w: PR '%s'", domain.ErrPRExists, pr.ID)
			case pgerrcode.ForeignKeyViolation:
				return nil, fmt.Errorf("%w: author '%s'", domain.ErrNotFound, pr.AuthorID)
			}
		}
		return nil, domain.ErrInternalError
	}
	return &domain.PullRequest{ID: dbPR.PrID, Name: dbPR.PrName, AuthorID: dbPR.AuthorID, Status: domain.PRStatus(dbPR.Status), CreatedAt: dbPR.CreatedAt.Time}, nil
}

func (r *Repository) GetPRByID(ctx context.Context, prID string) (*domain.PullRequest, error) {
	q := r.querier(nil)
	dbPR, err := q.GetPRByID(ctx, prID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: PR '%s'", domain.ErrNotFound, prID)
		}
		return nil, domain.ErrInternalError
	}
	pr := &domain.PullRequest{
		ID:        dbPR.PrID,
		Name:      dbPR.PrName,
		AuthorID:  dbPR.AuthorID,
		Status:    domain.PRStatus(dbPR.Status),
		CreatedAt: dbPR.CreatedAt.Time,
	}
	if dbPR.MergedAt.Valid {
		pr.MergedAt = &dbPR.MergedAt.Time
	}
	return pr, nil
}

func (r *Repository) MergePR(ctx context.Context, tx pgx.Tx, prID string) (*domain.PullRequest, error) {
	q := r.querier(tx)
	mergedDBPR, err := q.MergePR(ctx, prID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: PR '%s'", domain.ErrNotFound, prID)
		}
		return nil, domain.ErrInternalError
	}

	reviewersUser, err := q.GetReviewersForPR(ctx, prID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: PR '%s'", domain.ErrNotFound, prID)
		}
		return nil, domain.ErrInternalError
	}
	reviewers := make([]domain.Reviewer, len(reviewersUser))
	for i, reviewer := range reviewersUser {
		reviewers[i] = domain.Reviewer{ID: reviewer.UserID, Username: reviewer.Username}
	}

	pr := &domain.PullRequest{
		ID:        mergedDBPR.PrID,
		Name:      mergedDBPR.PrName,
		AuthorID:  mergedDBPR.AuthorID,
		Status:    domain.PRStatus(mergedDBPR.Status),
		Reviewers: reviewers,
		CreatedAt: mergedDBPR.CreatedAt.Time,
	}
	if mergedDBPR.MergedAt.Valid {
		pr.MergedAt = &mergedDBPR.MergedAt.Time
	}
	return pr, nil
}

func (r *Repository) GetReviewers(ctx context.Context, prID string) ([]domain.User, error) {
	q := r.querier(nil)
	dbReviewers, err := q.GetReviewersForPR(ctx, prID)
	if err != nil {
		return nil, domain.ErrInternalError
	}
	reviewers := make([]domain.User, len(dbReviewers))
	for i, rev := range dbReviewers {
		reviewers[i] = domain.User{ID: rev.UserID, Username: rev.Username}
	}
	return reviewers, nil
}

func (r *Repository) RemoveReviewer(ctx context.Context, tx pgx.Tx, prID string, userID string) error {
	q := r.querier(tx)
	if err := q.RemoveReviewerFromPR(ctx, models.RemoveReviewerFromPRParams{PrID: prID, UserID: userID}); err != nil {
		return domain.ErrInternalError
	}
	return nil
}

func (r *Repository) AssignReviewers(ctx context.Context, tx pgx.Tx, prID string, userIDs []string) error {
	q := r.querier(tx)
	for _, userID := range userIDs {
		if err := q.AddReviewerToPR(ctx, models.AddReviewerToPRParams{PrID: prID, UserID: userID}); err != nil {
			return domain.ErrInternalError
		}
	}
	return nil
}

func (r *Repository) GetOpenPRsByReviewer(ctx context.Context, tx pgx.Tx, userID string) ([]domain.PullRequest, error) {
	q := r.querier(tx)
	dbPRs, err := q.GetPRsForReviewer(ctx, userID)
	if err != nil {
		return nil, domain.ErrInternalError
	}
	prs := make([]domain.PullRequest, len(dbPRs))
	for i, p := range dbPRs {
		prs[i] = domain.PullRequest{ID: p.PrID, AuthorID: p.AuthorID}
	}
	return prs, nil
}

func (r *Repository) GetPRsByReviewer(ctx context.Context, userID string) ([]domain.PullRequest, error) {
	q := r.querier(nil)
	dbPRs, err := q.GetPRsForReviewer(ctx, userID)
	if err != nil {
		return nil, domain.ErrInternalError
	}
	prs := make([]domain.PullRequest, len(dbPRs))
	for i, p := range dbPRs {
		prs[i] = domain.PullRequest{ID: p.PrID, AuthorID: p.AuthorID}
	}
	return prs, nil
}

func (r *Repository) GetOpenPRsWithoutReviewers(ctx context.Context) ([]domain.PullRequest, error) {
	q := r.querier(nil)
	dbPRs, err := q.GetOpenPRsWithoutReviewers(ctx)
	if err != nil {
		return nil, domain.ErrInternalError
	}
	prs := make([]domain.PullRequest, len(dbPRs))
	for i, p := range dbPRs {
		prs[i] = domain.PullRequest{
			ID:        p.PrID,
			Name:      p.PrName,
			AuthorID:  p.AuthorID,
			Status:    domain.PRStatus(p.Status),
			CreatedAt: p.CreatedAt.Time,
		}
	}
	return prs, nil
}

// --- StatsRepository Implementation ---

func (r *Repository) GetReviewStats(ctx context.Context) ([]domain.StatItem, error) {
	q := r.querier(nil)
	dbStats, err := q.GetReviewStats(ctx)
	if err != nil {
		return nil, domain.ErrInternalError
	}
	stats := make([]domain.StatItem, len(dbStats))
	for i, s := range dbStats {
		stats[i] = domain.StatItem{
			UserID:      s.UserID,
			ReviewCount: s.ReviewCount,
		}
	}
	return stats, nil
}

func (r *Repository) GetOpenReviewCountForTeam(ctx context.Context, teamName string) (int, error) {
	q := r.querier(nil)
	team, err := r.GetTeamByName(ctx, teamName)
	if err != nil {
		return 0, err
	}
	count, err := q.CountOpenReviewsByTeam(ctx, team.ID)
	if err != nil {
		return 0, domain.ErrInternalError
	}
	return int(count), nil
}

func (r *Repository) GetMergedReviewCountForTeam(ctx context.Context, teamName string) (int, error) {
	q := r.querier(nil)
	team, err := r.GetTeamByName(ctx, teamName)
	if err != nil {
		return 0, err
	}
	count, err := q.CountMergedReviewsByTeam(ctx, team.ID)
	if err != nil {
		return 0, domain.ErrInternalError
	}
	return int(count), nil
}

func (r *Repository) GetOpenReviewCountForUser(ctx context.Context, userID string) (int, error) {
	q := r.querier(nil)
	count, err := q.CountOpenReviewsByUser(ctx, userID)
	if err != nil {
		return 0, domain.ErrInternalError
	}
	return int(count), nil
}

func (r *Repository) GetMergedReviewCountForUser(ctx context.Context, userID string) (int, error) {
	q := r.querier(nil)
	count, err := q.CountMergedReviewsByUser(ctx, userID)
	if err != nil {
		return 0, domain.ErrInternalError
	}
	return int(count), nil
}
