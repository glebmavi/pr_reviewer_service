package e2e

type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type TeamMember struct {
	IsActive bool   `json:"is_active"`
	UserId   string `json:"user_id"`
	Username string `json:"username"`
}

type Team struct {
	Members  []TeamMember `json:"members"`
	TeamName string       `json:"team_name"`
}

type User struct {
	IsActive bool   `json:"is_active"`
	TeamName string `json:"team_name"`
	UserId   string `json:"user_id"`
	Username string `json:"username"`
}

type UserAddRequest struct {
	IsActive bool   `json:"is_active"`
	TeamName string `json:"team_name"`
	Username string `json:"username"`
}

type PullRequest struct {
	AssignedReviewers []string `json:"assigned_reviewers"`
	AuthorId          string   `json:"author_id"`
	CreatedAt         *string  `json:"createdAt,omitempty"`
	MergedAt          *string  `json:"mergedAt,omitempty"`
	PullRequestId     string   `json:"pull_request_id"`
	PullRequestName   string   `json:"pull_request_name"`
	Status            string   `json:"status"`
}

type PullRequestShort struct {
	AuthorId        string `json:"author_id"`
	PullRequestId   string `json:"pull_request_id"`
	PullRequestName string `json:"pull_request_name"`
	Status          string `json:"status"`
}

type PullRequestCreateRequest struct {
	AuthorId        string `json:"author_id"`
	PullRequestName string `json:"pull_request_name"`
}

type CountResponse struct {
	Count int `json:"count"`
}

type StatItem struct {
	ReviewCount *int64  `json:"review_count,omitempty"`
	UserId      *string `json:"user_id,omitempty"`
}

type StatsResponse struct {
	ReviewStats *[]StatItem `json:"review_stats,omitempty"`
}

type TeamDeactivateRequest struct {
	TeamName string `json:"team_name"`
}

type TeamDeactivateResponse struct {
	DeactivatedUsersCount  *int `json:"deactivated_users_count,omitempty"`
	ReassignedReviewsCount *int `json:"reassigned_reviews_count,omitempty"`
}

type PostUsersSetIsActiveJSONBody struct {
	UserId   string `json:"user_id"`
	IsActive bool   `json:"is_active"`
}
