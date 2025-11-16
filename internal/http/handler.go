package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/render"

	"github.com/glebmavi/pr_reviewer_service/internal/app"
	"github.com/glebmavi/pr_reviewer_service/internal/domain"
	"github.com/glebmavi/pr_reviewer_service/pkg/api"
)

// Handler implements the api.ServerInterface
type Handler struct {
	teamSvc  *app.TeamService
	prSvc    *app.PullRequestService
	userSvc  *app.UserService
	statsSvc *app.StatsService
	log      *slog.Logger
}

func NewHandler(teamSvc *app.TeamService, prSvc *app.PullRequestService, userSvc *app.UserService, statsSvc *app.StatsService, log *slog.Logger) *Handler {
	return &Handler{
		teamSvc:  teamSvc,
		prSvc:    prSvc,
		userSvc:  userSvc,
		statsSvc: statsSvc,
		log:      log,
	}
}

// --- Health ---

func (h *Handler) GetHealth(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, map[string]string{"status": "ok"})
}

// --- Teams ---

func (h *Handler) PostTeamAdd(w http.ResponseWriter, r *http.Request) {
	var req api.PostTeamAddJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, r, api.VALIDATIONERROR, "invalid request body", http.StatusBadRequest)
		return
	}

	memberNames := make([]string, len(req.Members))
	for i, member := range req.Members {
		memberNames[i] = member.Username
	}

	team, err := h.teamSvc.CreateTeam(r.Context(), req.TeamName, memberNames)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, teamToAPI(team))
}

func (h *Handler) GetTeamGet(w http.ResponseWriter, r *http.Request, params api.GetTeamGetParams) {
	team, err := h.teamSvc.GetTeam(r.Context(), params.TeamName)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(w, r, teamToAPI(team))
}

func (h *Handler) PostTeamEdit(w http.ResponseWriter, r *http.Request) {
	var req api.PostTeamEditJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, r, api.VALIDATIONERROR, "invalid request body", http.StatusBadRequest)
		return
	}

	team, err := h.teamSvc.UpdateTeam(r.Context(), req.OldTeamName, req.NewTeamName)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(w, r, teamToAPI(team))
}

func (h *Handler) PostTeamDeactivate(w http.ResponseWriter, r *http.Request) {
	var req api.PostTeamDeactivateJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, r, api.VALIDATIONERROR, "invalid request body", http.StatusBadRequest)
		return
	}

	deactivatedCount, reassignedCount, err := h.teamSvc.DeactivateTeamAndReassign(r.Context(), req.TeamName)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(w, r, api.TeamDeactivateResponse{
		DeactivatedUsersCount:  &deactivatedCount,
		ReassignedReviewsCount: &reassignedCount,
	})
}

// --- Users ---

func (h *Handler) PostUsersAdd(w http.ResponseWriter, r *http.Request) {
	var req api.PostUsersAddJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, r, api.VALIDATIONERROR, "invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.userSvc.AddUser(r.Context(), req.Username, req.TeamName, req.IsActive)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, userToAPI(user))
}

func (h *Handler) PostUsersEdit(w http.ResponseWriter, r *http.Request) {
	var req api.PostUsersEditJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, r, api.VALIDATIONERROR, "invalid request body", http.StatusBadRequest)
		return
	}

	user := &domain.User{
		ID:       req.UserId,
		Username: req.Username,
		TeamName: req.TeamName,
		IsActive: req.IsActive,
	}

	updatedUser, err := h.userSvc.UpdateUser(r.Context(), user)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(w, r, userToAPI(updatedUser))
}

func (h *Handler) PostUsersMoveToTeam(w http.ResponseWriter, r *http.Request) {
	var req api.PostUsersMoveToTeamJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, r, api.VALIDATIONERROR, "invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.userSvc.MoveUserToTeam(r.Context(), req.UserId, req.NewTeamName)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(w, r, userToAPI(user))
}

func (h *Handler) PostUsersSetIsActive(w http.ResponseWriter, r *http.Request) {
	var req api.PostUsersSetIsActiveJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, r, api.VALIDATIONERROR, "invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.userSvc.SetUserActiveStatus(r.Context(), req.UserId, req.IsActive)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(w, r, userToAPI(user))
}

func (h *Handler) GetUsersGetReview(w http.ResponseWriter, r *http.Request, params api.GetUsersGetReviewParams) {
	prs, err := h.prSvc.GetReviewsForUser(r.Context(), params.UserId)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	shortPRs := make([]api.PullRequestShort, len(prs))
	for i, pr := range prs {
		shortPRs[i] = *prToShortAPI(&pr)
	}

	render.Status(r, http.StatusOK)
	render.JSON(w, r, struct {
		UserId       string                 `json:"user_id"`
		PullRequests []api.PullRequestShort `json:"pull_requests"`
	}{
		UserId:       params.UserId,
		PullRequests: shortPRs,
	})
}

// --- PullRequests ---

func (h *Handler) PostPullRequestCreate(w http.ResponseWriter, r *http.Request) {
	var req api.PostPullRequestCreateJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, r, api.VALIDATIONERROR, "invalid request body", http.StatusBadRequest)
		return
	}

	pr, err := h.prSvc.CreatePR(r.Context(), req.PullRequestName, req.AuthorId)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, prToAPI(pr))
}

func (h *Handler) GetPullRequestGetPullRequestId(w http.ResponseWriter, r *http.Request, pullRequestId api.PullRequestIdParam) {
	pr, err := h.prSvc.GetPR(r.Context(), pullRequestId)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(w, r, prToAPI(pr))
}

func (h *Handler) PostPullRequestMerge(w http.ResponseWriter, r *http.Request) {
	var req api.PostPullRequestMergeJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, r, api.VALIDATIONERROR, "invalid request body", http.StatusBadRequest)
		return
	}

	pr, err := h.prSvc.MergePR(r.Context(), req.PullRequestId)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(w, r, prToAPI(pr))
}

func (h *Handler) PostPullRequestReassign(w http.ResponseWriter, r *http.Request) {
	var req api.PostPullRequestReassignJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, r, api.VALIDATIONERROR, "invalid request body", http.StatusBadRequest)
		return
	}

	pr, newReviewerID, err := h.prSvc.ReassignReviewer(r.Context(), req.PullRequestId, req.OldUserId)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(w, r, struct {
		Pr         *api.PullRequest `json:"pr"`
		ReplacedBy string           `json:"replaced_by"`
	}{
		Pr:         prToAPI(pr),
		ReplacedBy: newReviewerID,
	})
}

func (h *Handler) GetPullRequestOpenWithoutReviewers(w http.ResponseWriter, r *http.Request) {
	prs, err := h.prSvc.GetOpenPRsWithoutReviewers(r.Context())
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	shortPRs := make([]api.PullRequestShort, len(prs))
	for i, pr := range prs {
		shortPRs[i] = *prToShortAPI(&pr)
	}

	render.Status(r, http.StatusOK)
	render.JSON(w, r, shortPRs)
}

// --- Stats ---

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.statsSvc.GetStats(r.Context())
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	apiStats := make([]api.StatItem, len(stats))
	for i, s := range stats {
		apiStats[i] = api.StatItem{
			UserId:      &s.UserID,
			ReviewCount: &s.ReviewCount,
		}
	}

	render.Status(r, http.StatusOK)
	render.JSON(w, r, api.StatsResponse{ReviewStats: &apiStats})
}

func (h *Handler) GetStatsTeamTeamNameOpenReviewCount(w http.ResponseWriter, r *http.Request, teamName api.TeamNameParam) {
	h.getReviewCount(r.Context(), w, r, h.statsSvc.GetOpenReviewCountForTeam, teamName)
}

func (h *Handler) GetStatsTeamTeamNameMergedReviewCount(w http.ResponseWriter, r *http.Request, teamName api.TeamNameParam) {
	h.getReviewCount(r.Context(), w, r, h.statsSvc.GetMergedReviewCountForTeam, teamName)
}

func (h *Handler) GetStatsUserUserIdOpenReviewCount(w http.ResponseWriter, r *http.Request, userId api.UserIdParam) {
	h.getReviewCount(r.Context(), w, r, h.statsSvc.GetOpenReviewCountForUser, userId)
}

func (h *Handler) GetStatsUserUserIdMergedReviewCount(w http.ResponseWriter, r *http.Request, userId api.UserIdParam) {
	h.getReviewCount(r.Context(), w, r, h.statsSvc.GetMergedReviewCountForUser, userId)
}

func (h *Handler) getReviewCount(ctx context.Context, w http.ResponseWriter, r *http.Request, countFn func(context.Context, string) (int, error), param string) {
	count, err := countFn(ctx, param)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, api.CountResponse{Count: count})
}

// --- Error Helpers ---

func (h *Handler) handleServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var code = api.INTERNALERROR
	var httpStatus = http.StatusInternalServerError
	var message = err.Error()

	switch {
	case errors.Is(err, domain.ErrNotFound):
		code = api.NOTFOUND
		httpStatus = http.StatusNotFound
	case errors.Is(err, domain.ErrTeamExists):
		code = api.TEAMEXISTS
		httpStatus = http.StatusConflict
	case errors.Is(err, domain.ErrPRExists):
		code = api.PREXISTS
		httpStatus = http.StatusConflict
	case errors.Is(err, domain.ErrPRMerged):
		code = api.PRMERGED
		httpStatus = http.StatusConflict
	case errors.Is(err, domain.ErrNotAssigned):
		code = api.NOTASSIGNED
		httpStatus = http.StatusConflict
	case errors.Is(err, domain.ErrNoCandidate):
		code = api.NOCANDIDATE
		httpStatus = http.StatusConflict
	case errors.Is(err, domain.ErrValidation):
		code = api.VALIDATIONERROR
		httpStatus = http.StatusBadRequest
	}

	if httpStatus == http.StatusInternalServerError {
		h.log.ErrorContext(r.Context(), "internal server error", slog.String("error", err.Error()))
		message = "internal server error"
	} else {
		h.log.InfoContext(r.Context(), "client error", slog.String("error", err.Error()), "code", string(code))
	}

	h.respondError(w, r, code, message, httpStatus)
}

func (h *Handler) respondError(w http.ResponseWriter, r *http.Request, code api.ErrorResponseErrorCode, message string, httpStatus int) {
	resp := api.ErrorResponse{
		Error: struct {
			Code    api.ErrorResponseErrorCode `json:"code"`
			Message string                     `json:"message"`
		}{
			Code:    code,
			Message: message,
		},
	}

	render.Status(r, httpStatus)
	render.JSON(w, r, resp)
}

// --- Mappers ---

func teamToAPI(team *domain.Team) *api.Team {
	members := make([]api.TeamMember, len(team.Members))
	for i, m := range team.Members {
		members[i] = api.TeamMember{
			IsActive: m.IsActive,
			UserId:   m.ID,
			Username: m.Username,
		}
	}
	return &api.Team{
		TeamName: team.TeamName,
		Members:  members,
	}
}

func userToAPI(user *domain.User) *api.User {
	return &api.User{
		UserId:   user.ID,
		Username: user.Username,
		TeamName: user.TeamName,
		IsActive: user.IsActive,
	}
}

func prToAPI(pr *domain.PullRequest) *api.PullRequest {
	reviewerIDs := make([]string, len(pr.Reviewers))
	for i, r := range pr.Reviewers {
		reviewerIDs[i] = r.ID
	}

	var mergedAt *time.Time
	if pr.MergedAt != nil {
		mergedAt = pr.MergedAt
	}

	return &api.PullRequest{
		PullRequestId:     pr.ID,
		PullRequestName:   pr.Name,
		AuthorId:          pr.AuthorID,
		Status:            api.PullRequestStatus(pr.Status),
		AssignedReviewers: reviewerIDs,
		CreatedAt:         &pr.CreatedAt,
		MergedAt:          mergedAt,
	}
}

func prToShortAPI(pr *domain.PullRequest) *api.PullRequestShort {
	return &api.PullRequestShort{
		PullRequestId:   pr.ID,
		PullRequestName: pr.Name,
		AuthorId:        pr.AuthorID,
		Status:          api.PullRequestShortStatus(pr.Status),
	}
}
