package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/compose"
)

const (
	baseURL      = "http://localhost:8080"
	startTimeout = 60 * time.Second
)

var (
	composeStack compose.ComposeStack
	client       *http.Client
)

func TestMain(m *testing.M) {
	var exitCode int
	defer func() {
		os.Exit(exitCode)
	}()

	ctx := context.Background()

	composeFilePath, err := filepath.Abs("../../docker-compose.test.yml")
	if err != nil {
		log.Printf("ERROR: failed to get absolute path: %v\n", err)
		exitCode = 1
		return
	}

	if _, err := os.Stat(composeFilePath); os.IsNotExist(err) {
		log.Printf("ERROR: docker-compose.test.yml not found at %s\n", composeFilePath)
		exitCode = 1
		return
	}

	log.Printf("Using docker-compose file: %s\n", composeFilePath)

	composeStack, err = compose.NewDockerComposeWith(
		compose.WithStackFiles(composeFilePath),
		compose.StackIdentifier("prreviewer_test"),
	)
	if err != nil {
		log.Printf("ERROR: failed to create compose stack: %v\n", err)
		exitCode = 1
		return
	}

	composeStack = composeStack.WithEnv(map[string]string{
		"POSTGRES_USER":     "testuser",
		"POSTGRES_PASSWORD": "testpassword",
		"POSTGRES_DB":       "testdb",
		"APP_DB_URL":        "postgres://testuser:testpassword@postgres:5432/testdb?sslmode=disable",
	})

	log.Println("Starting Docker Compose stack...")
	err = composeStack.Up(ctx, compose.Wait(true))
	if err != nil {
		log.Printf("ERROR: failed to start compose stack: %v\n", err)
		_ = composeStack.Down(ctx, compose.RemoveOrphans(true), compose.RemoveVolumes(true))
		exitCode = 1
		return
	}
	log.Println("Docker Compose stack started successfully")

	client = &http.Client{Timeout: 10 * time.Second}

	log.Println("Waiting for service to be ready...")
	if err := waitForService(baseURL+"/health", startTimeout); err != nil {
		log.Printf("ERROR: service not ready: %v\n", err)
		_ = composeStack.Down(ctx, compose.RemoveOrphans(true), compose.RemoveVolumes(true))
		exitCode = 1
		return
	}
	log.Println("Service is ready, starting tests...")

	exitCode = m.Run()

	log.Println("Cleaning up Docker Compose stack...")
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := composeStack.Down(cleanupCtx, compose.RemoveOrphans(true), compose.RemoveVolumes(true)); err != nil {
		log.Printf("WARNING: failed to stop compose stack: %v\n", err)
		// Don't fail tests due to cleanup issues
	} else {
		log.Println("Cleanup completed successfully")
	}
}

func waitForService(url string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for i := 1; ; i++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("service not ready after %s", timeout)
		case <-ticker.C:
			resp, err := client.Get(url)
			if err == nil && resp.StatusCode == http.StatusOK {
				_ = resp.Body.Close()
				log.Printf("✓ Service ready after %d attempts\n", i)
				return nil
			}
			if resp != nil {
				_ = resp.Body.Close()
			}
		}
	}
}

func doRequest(t *testing.T, method, path string, body interface{}) (*http.Response, []byte) {
	t.Helper()

	var bodyReader io.Reader
	var jsonData []byte
	if body != nil {
		var err error
		jsonData, err = json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewBuffer(jsonData)
	}

	t.Logf("Request: %s %s", method, path)
	if body != nil {
		t.Logf("Request body: %s", string(jsonData))
	}

	req, err := http.NewRequest(method, baseURL+path, bodyReader)
	require.NoError(t, err)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	require.NoError(t, err)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	logResponse(t, respBody)

	return resp, respBody
}

func logResponse(t *testing.T, body []byte) {
	t.Helper()
	t.Logf("Response body: %s", string(body))
}

func unmarshalResponse(t *testing.T, data []byte, v interface{}) {
	t.Helper()
	err := json.Unmarshal(data, v)
	require.NoError(t, err, "response body: %s", string(data))
}

func assertErrorCode(t *testing.T, body []byte, expectedCode string) {
	t.Helper()
	var errResp ErrorResponse
	unmarshalResponse(t, body, &errResp)
	assert.Equal(t, expectedCode, errResp.Error.Code)
}

// Tests
func TestHealth(t *testing.T) {
	resp, _ := doRequest(t, "GET", "/health", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestPRReviewCycle(t *testing.T) {
	// 1. Create a team with 3 members
	teamName := "backend-squad"
	teamPayload := Team{
		TeamName: teamName,
		Members: []TeamMember{
			{UserId: "", Username: "A"},
			{UserId: "", Username: "B"},
			{UserId: "", Username: "C"},
		},
	}
	resp, body := doRequest(t, "POST", "/team/add", teamPayload)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdTeam Team
	unmarshalResponse(t, body, &createdTeam)
	assert.Equal(t, teamName, createdTeam.TeamName)
	assert.Len(t, createdTeam.Members, 3)

	// 2. Create a PR by A, should assign B and C
	prPayload := map[string]string{
		"pull_request_name": "feat: new feature",
		"author_id":         createdTeam.Members[0].UserId,
	}
	resp, body = doRequest(t, "POST", "/pullRequest/create", prPayload)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdPR PullRequest
	unmarshalResponse(t, body, &createdPR)
	prID := createdPR.PullRequestId

	assert.Equal(t, "feat: new feature", createdPR.PullRequestName)
	assert.Equal(t, createdTeam.Members[0].UserId, createdPR.AuthorId)
	assert.Equal(t, "OPEN", createdPR.Status)
	assert.Len(t, createdPR.AssignedReviewers, 2)
	assert.NotContains(t, createdPR.AssignedReviewers, createdTeam.Members[0].UserId) // Author should not be a reviewer
	assert.Contains(t, createdPR.AssignedReviewers, createdTeam.Members[1].UserId)
	assert.Contains(t, createdPR.AssignedReviewers, createdTeam.Members[2].UserId)

	// 3. Get the PR and verify its state
	resp, body = doRequest(t, "GET", "/pullRequest/get/"+prID, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var fetchedPR PullRequest
	unmarshalResponse(t, body, &fetchedPR)
	assert.Equal(t, prID, fetchedPR.PullRequestId)
	assert.Len(t, fetchedPR.AssignedReviewers, 2)

	// 4. Merge the PR
	mergePayload := map[string]string{"pull_request_id": prID}
	resp, body = doRequest(t, "POST", "/pullRequest/merge", mergePayload)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var mergedPR PullRequest
	unmarshalResponse(t, body, &mergedPR)
	assert.Equal(t, "MERGED", mergedPR.Status)

	// 5. Try to reassign a reviewer on a merged PR (should fail WITH PR_MERGED)
	reassignPayload := map[string]string{
		"pull_request_id": prID,
		"old_user_id":     createdTeam.Members[2].UserId,
	}
	resp, body = doRequest(t, "POST", "/pullRequest/reassign", reassignPayload)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	assertErrorCode(t, body, "PR_MERGED")
}

func TestPRReviewWithNotEnoughReviewers(t *testing.T) {
	// 1. Create a team with 2 members
	teamName := "frontend-squad"
	teamPayload := Team{
		TeamName: teamName,
		Members: []TeamMember{
			{UserId: "", Username: "D"},
			{UserId: "", Username: "E"},
		},
	}
	resp, body := doRequest(t, "POST", "/team/add", teamPayload)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdTeam Team
	unmarshalResponse(t, body, &createdTeam)

	// 2. Create a PR by D, should assign only E
	prPayload := map[string]string{
		"pull_request_name": "fix: css bug",
		"author_id":         createdTeam.Members[0].UserId,
	}
	resp, body = doRequest(t, "POST", "/pullRequest/create", prPayload)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdPR PullRequest
	unmarshalResponse(t, body, &createdPR)
	assert.Len(t, createdPR.AssignedReviewers, 1)
	assert.Equal(t, createdTeam.Members[1].UserId, createdPR.AssignedReviewers[0])

	// 3. Create a team with 1 member
	teamPayloadSolo := Team{
		TeamName: "solo-squad",
		Members:  []TeamMember{{UserId: "", Username: "F"}},
	}
	resp, body = doRequest(t, "POST", "/team/add", teamPayloadSolo)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdTeamSolo Team
	unmarshalResponse(t, body, &createdTeamSolo)

	// 4. Create a PR by F, should assign 0 reviewers
	prPayloadSolo := map[string]string{
		"pull_request_name": "docs: update readme",
		"author_id":         createdTeamSolo.Members[0].UserId,
	}
	resp, body = doRequest(t, "POST", "/pullRequest/create", prPayloadSolo)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdPRSolo PullRequest
	unmarshalResponse(t, body, &createdPRSolo)
	assert.Len(t, createdPRSolo.AssignedReviewers, 0)
}

func TestUserDeactivationAndReassignment(t *testing.T) {
	// 1. Create a team with 3 members
	teamName := "deactivation-test-squad"
	teamPayload := Team{
		TeamName: teamName,
		Members: []TeamMember{
			{Username: "UserX"},
			{Username: "UserY"},
			{Username: "UserZ"},
		},
	}
	resp, body := doRequest(t, "POST", "/team/add", teamPayload)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	var createdTeam Team
	unmarshalResponse(t, body, &createdTeam)
	require.Len(t, createdTeam.Members, 3)
	author := createdTeam.Members[0]
	reviewer1 := createdTeam.Members[1]
	reviewer2 := createdTeam.Members[2]

	// 2. Create a PR by UserX, assigning UserY and UserZ
	prPayload := map[string]string{
		"pull_request_name": "feat: user deactivation test",
		"author_id":         author.UserId,
	}
	resp, body = doRequest(t, "POST", "/pullRequest/create", prPayload)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	var createdPR PullRequest
	unmarshalResponse(t, body, &createdPR)
	prID := createdPR.PullRequestId
	require.Contains(t, createdPR.AssignedReviewers, reviewer1.UserId)
	require.Contains(t, createdPR.AssignedReviewers, reviewer2.UserId)

	// 3. Deactivate UserY
	deactivatePayload := PostUsersSetIsActiveJSONBody{
		UserId:   reviewer1.UserId,
		IsActive: false,
	}
	resp, body = doRequest(t, "POST", "/users/setIsActive", deactivatePayload) // will also remove UserY from any PR
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var userResponse User
	unmarshalResponse(t, body, &userResponse)
	assert.Equal(t, reviewer1.UserId, userResponse.UserId)
	assert.False(t, userResponse.IsActive, "UserY should be deactivated")

	// 4. Try to reassign UserZ. It should fail with NO_CANDIDATE because UserZ is the only other
	// reviewer and there are no other active users in the team.
	reassignPayload := map[string]string{
		"pull_request_id": prID,
		"old_user_id":     reviewer2.UserId,
	}
	resp, body = doRequest(t, "POST", "/pullRequest/reassign", reassignPayload)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	assertErrorCode(t, body, "NO_CANDIDATE")
}

func TestTeamDeactivation(t *testing.T) {
	// 1. Create two teams
	teamToDeactivateName := "deactivation-squad"
	teamToDeactivatePayload := Team{
		TeamName: teamToDeactivateName,
		Members: []TeamMember{
			{Username: "Reviewer1"},
			{Username: "Reviewer2"},
			{Username: "Reviewer3"},
		},
	}
	resp, body := doRequest(t, "POST", "/team/add", teamToDeactivatePayload)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	var teamToDeactivate Team
	unmarshalResponse(t, body, &teamToDeactivate)
	reviewerToDeactivate := teamToDeactivate.Members[0]

	reassignTeamName := "reassign-squad"
	reassignTeamPayload := Team{
		TeamName: reassignTeamName,
		Members: []TeamMember{
			{Username: "Author"},
			{Username: "NewReviewerCandidate"},
		},
	}
	resp, body = doRequest(t, "POST", "/team/add", reassignTeamPayload)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	var reassignTeam Team
	unmarshalResponse(t, body, &reassignTeam)
	author := reassignTeam.Members[0]

	// 2. Create a PR by a user from the second team
	prPayload := map[string]string{
		"pull_request_name": "feat: team deactivation test",
		"author_id":         author.UserId,
	}
	resp, body = doRequest(t, "POST", "/pullRequest/create", prPayload)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	var createdPR PullRequest
	unmarshalResponse(t, body, &createdPR)
	prID := createdPR.PullRequestId

	// 3. Manually assign a reviewer from the team that will be deactivated
	assignPayload := map[string]string{
		"pull_request_id": prID,
		"user_id":         reviewerToDeactivate.UserId,
	}
	resp, body = doRequest(t, "POST", "/pullRequest/assign", assignPayload)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var updatedPR PullRequest
	unmarshalResponse(t, body, &updatedPR)
	require.Contains(t, updatedPR.AssignedReviewers, reviewerToDeactivate.UserId)

	// 4. Deactivate the first team
	deactivateTeamPayload := map[string]string{
		"team_name": teamToDeactivateName,
	}
	resp, body = doRequest(t, "POST", "/team/deactivate", deactivateTeamPayload)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 5. Check the response
	var deactivateResp TeamDeactivateResponse
	unmarshalResponse(t, body, &deactivateResp)
	assert.Equal(t, 3, *deactivateResp.DeactivatedUsersCount, "Should deactivate all 3 users in the team")
	assert.Equal(t, 0, *deactivateResp.ReassignedReviewsCount, "Has 1 reviewer which is enough, so it didn't reassign")
}

func TestUserManagement(t *testing.T) {
	// 1. Create a team
	team1Name := "rangers"
	team1Payload := Team{
		TeamName: team1Name,
		Members:  []TeamMember{},
	}
	resp, _ := doRequest(t, "POST", "/team/add", team1Payload)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// 2. Add a new user to the team
	addUserPayload := map[string]string{
		"username":  "Zordon",
		"team_name": team1Name,
	}
	resp, body := doRequest(t, "POST", "/users/add", addUserPayload)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var addedUser User
	unmarshalResponse(t, body, &addedUser)
	assert.Equal(t, "Zordon", addedUser.Username)
	assert.Equal(t, team1Name, addedUser.TeamName)
	assert.True(t, addedUser.IsActive)
	userID := addedUser.UserId

	// 3. Create another team
	team2Name := "paladins"
	team2Payload := Team{
		TeamName: team2Name,
		Members:  []TeamMember{},
	}
	resp, _ = doRequest(t, "POST", "/team/add", team2Payload)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// 4. Move the user to the new team
	moveUserPayload := map[string]string{
		"user_id":       userID,
		"new_team_name": team2Name,
	}
	resp, body = doRequest(t, "POST", "/users/moveToTeam", moveUserPayload)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var movedUser User
	unmarshalResponse(t, body, &movedUser)
	assert.Equal(t, userID, movedUser.UserId)
	assert.Equal(t, team2Name, movedUser.TeamName)

	// 5. Get user and verify team change
	resp, body = doRequest(t, "GET", "/users/get/"+userID, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var fetchedUser User
	unmarshalResponse(t, body, &fetchedUser)
	assert.Equal(t, team2Name, fetchedUser.TeamName)
}

func TestStats(t *testing.T) {
	// 1. Create teams and users
	teamAvengersName := "avengers"
	teamAvengersPayload := Team{
		TeamName: teamAvengersName,
		Members: []TeamMember{
			{Username: "ironman"},
			{Username: "captain"},
			{Username: "thor"},
		},
	}
	resp, body := doRequest(t, "POST", "/team/add", teamAvengersPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var avengersTeam Team
	unmarshalResponse(t, body, &avengersTeam)
	ironman := avengersTeam.Members[0]
	captain := avengersTeam.Members[1]
	thor := avengersTeam.Members[2]

	teamGuardiansName := "guardians"
	teamGuardiansPayload := Team{
		TeamName: teamGuardiansName,
		Members: []TeamMember{
			{Username: "starlord"},
			{Username: "gamora"},
		},
	}
	resp, body = doRequest(t, "POST", "/team/add", teamGuardiansPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var guardiansTeam Team
	unmarshalResponse(t, body, &guardiansTeam)
	starlord := guardiansTeam.Members[0]
	gamora := guardiansTeam.Members[1]

	// 2. Create PRs
	pr1Payload := map[string]string{"pull_request_name": "feat: infinity stones", "author_id": ironman.UserId}
	resp, body = doRequest(t, "POST", "/pullRequest/create", pr1Payload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var pr1 PullRequest
	unmarshalResponse(t, body, &pr1)

	pr2Payload := map[string]string{"pull_request_name": "feat: awesome mix vol. 1", "author_id": starlord.UserId}
	resp, body = doRequest(t, "POST", "/pullRequest/create", pr2Payload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	pr3Payload := map[string]string{"pull_request_name": "refactor: suit v42", "author_id": ironman.UserId}
	resp, body = doRequest(t, "POST", "/pullRequest/create", pr3Payload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// 3. Merge one PR
	mergePayload := map[string]string{"pull_request_id": pr1.PullRequestId}
	resp, _ = doRequest(t, "POST", "/pullRequest/merge", mergePayload)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// 4. Check stats
	// Global stats
	resp, body = doRequest(t, "GET", "/stats", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var stats StatsResponse
	unmarshalResponse(t, body, &stats)

	// We expect stats for captain, thor and gamora.
	// captain: 1 merged (pr1), 1 open (pr3)
	// thor: 1 merged (pr1), 1 open (pr3)
	// gamora: 1 open (pr2)
	// The endpoint /stats returns total review count (open + merged)
	expectedStats := map[string]int64{
		captain.UserId: 2,
		thor.UserId:    2,
		gamora.UserId:  1,
	}
	for _, stat := range *stats.ReviewStats {
		if count, ok := expectedStats[*stat.UserId]; ok {
			assert.Equal(t, count, *stat.ReviewCount, "user %s review count mismatch", stat.UserId)
			delete(expectedStats, *stat.UserId)
		}
	}
	assert.Empty(t, expectedStats, "some users were not found in stats response")

	// Team open review count
	resp, body = doRequest(t, "GET", "/stats/team/"+teamAvengersName+"/open-review-count", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var avengersOpenCount CountResponse
	unmarshalResponse(t, body, &avengersOpenCount)
	assert.Equal(t, 2, avengersOpenCount.Count) // pr3 has 2 reviewers from avengers

	// Team merged review count
	resp, body = doRequest(t, "GET", "/stats/team/"+teamAvengersName+"/merged-review-count", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var avengersMergedCount CountResponse
	unmarshalResponse(t, body, &avengersMergedCount)
	assert.Equal(t, 2, avengersMergedCount.Count) // pr1 has 2 reviewers from avengers

	// User open review count
	resp, body = doRequest(t, "GET", "/stats/user/"+captain.UserId+"/open-review-count", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var userOpenCount CountResponse
	unmarshalResponse(t, body, &userOpenCount)
	assert.Equal(t, 1, userOpenCount.Count) // captain on pr3

	// User merged review count
	resp, body = doRequest(t, "GET", "/stats/user/"+captain.UserId+"/merged-review-count", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var userMergedCount CountResponse
	unmarshalResponse(t, body, &userMergedCount)
	assert.Equal(t, 1, userMergedCount.Count) // captain on pr1
}
