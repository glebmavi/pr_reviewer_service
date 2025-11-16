package domain

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type Transactor interface {
	BeginTx(ctx context.Context) (pgx.Tx, error)
	CommitTx(ctx context.Context, tx pgx.Tx) error
	RollbackTx(ctx context.Context, tx pgx.Tx) error
}

type TeamRepository interface {
	CreateTeam(ctx context.Context, tx pgx.Tx, team *Team) (*Team, error)
	GetTeamByName(ctx context.Context, teamName string) (*Team, error)
	GetTeamByID(ctx context.Context, teamID int32) (*Team, error)
	UpdateTeam(ctx context.Context, tx pgx.Tx, oldTeamName, newTeamName string) (*Team, error)
	DeactivateTeam(ctx context.Context, tx pgx.Tx, teamName string) error
}

type UserRepository interface {
	CreateUser(ctx context.Context, tx pgx.Tx, user *User) (*User, error)
	GetUserByID(ctx context.Context, userID string) (*User, error)
	GetUsersByTeam(ctx context.Context, teamID int32) ([]User, error)
	UpdateUser(ctx context.Context, tx pgx.Tx, user *User) (*User, error)
	SetUserActiveStatus(ctx context.Context, tx pgx.Tx, userID string, isActive bool) (*User, error)
	MoveUserToTeam(ctx context.Context, tx pgx.Tx, userID string, newTeamID int32) (*User, error)
	DeactivateUsersByTeam(ctx context.Context, tx pgx.Tx, teamID int32) ([]string, error)
	FindReviewCandidates(ctx context.Context, teamID int32, authorID string, excludeUserIDs []string, limit int) ([]User, error)
}

type PullRequestRepository interface {
	CreatePR(ctx context.Context, tx pgx.Tx, pr *PullRequest) (*PullRequest, error)
	GetPRByID(ctx context.Context, prID string) (*PullRequest, error)
	MergePR(ctx context.Context, tx pgx.Tx, prID string) (*PullRequest, error)
	GetReviewers(ctx context.Context, prID string) ([]User, error)
	RemoveReviewer(ctx context.Context, tx pgx.Tx, prID string, userID string) error
	AssignReviewers(ctx context.Context, tx pgx.Tx, prID string, userIDs []string) error
	GetOpenPRsByReviewer(ctx context.Context, tx pgx.Tx, userID string) ([]PullRequest, error)
}

type StatsRepository interface {
	// TODO: Add stats methods
}
