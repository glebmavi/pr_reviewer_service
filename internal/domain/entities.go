package domain

import (
	"errors"
	"time"
)

var (
	ErrInternalError = errors.New("internal Error")
	ErrNoCandidate   = errors.New("no suitable candidate found for assignment")
	ErrNotAssigned   = errors.New("user is not assigned to this PR")
	ErrNotFound      = errors.New("resource not found")
	ErrPRExists      = errors.New("PR already exists")
	ErrPRMerged      = errors.New("operation not allowed on merged PR")
	ErrTeamExists    = errors.New("team already exists")
	ErrValidation    = errors.New("validation failed")
	ErrUserNotActive = errors.New("user is not active")
)

type PRStatus string

const (
	StatusOpen   PRStatus = "OPEN"
	StatusMerged PRStatus = "MERGED"
)

type User struct {
	ID       string
	Username string
	TeamID   int32
	TeamName string
	IsActive bool
}

func (u *User) CanBeMoved() bool {
	return u.IsActive
}

type Team struct {
	ID       int32
	TeamName string
	IsActive bool
	Members  []User
}

func (t *Team) CanBeMoved() bool {
	return t.IsActive
}

type PullRequest struct {
	ID        string
	Name      string
	AuthorID  string
	Status    PRStatus
	Reviewers []Reviewer
	CreatedAt time.Time
	MergedAt  *time.Time
}

type Reviewer struct {
	ID       string
	Username string
}

func (pr *PullRequest) IsOpen() bool {
	return pr.Status != StatusMerged
}

type StatItem struct {
	ReviewCount int64
	UserID      string
}
