package e2e


// API Models
type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type TeamMember struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

type Team struct {
	TeamName string       `json:"team_name"`
	Members  []TeamMember `json:"members"`
}

type User struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	TeamName string `json:"team_name"`
	IsActive bool   `json:"is_active"`
}

type UserAddRequest struct {
	Username string `json:"username"`
	TeamName string `json:"team_name"`
	IsActive bool   `json:"is_active"`
}

type PullRequest struct {
	PullRequestID     string   `json:"pull_request_id"`
	PullRequestName   string   `json:"pull_request_name"`
	AuthorID          string   `json:"author_id"`
	Status            string   `json:"status"`
	AssignedReviewers []string `json:"assigned_reviewers"`
	CreatedAt         *string  `json:"createdAt,omitempty"`
	MergedAt          *string  `json:"mergedAt,omitempty"`
}

type PullRequestShort struct {
	PullRequestID   string `json:"pull_request_id"`
	PullRequestName string `json:"pull_request_name"`
	AuthorID        string `json:"author_id"`
	Status          string `json:"status"`
}

type PullRequestCreateRequest struct {
	PullRequestName string `json:"pull_request_name"`
	AuthorID        string `json:"author_id"`
}

type CountResponse struct {
	Count int `json:"count"`
}

type StatItem struct {
	UserID      string `json:"user_id"`
	ReviewCount int64  `json:"review_count"`
}

type StatsResponse struct {
	ReviewStats []StatItem `json:"review_stats"`
}

type TeamDeactivateRequest struct {
	TeamName string `json:"team_name"`
}

type TeamDeactivateResponse struct {
	DeactivatedUsersCount  int `json:"deactivated_users_count"`
	ReassignedReviewsCount int `json:"reassigned_reviews_count"`
}

