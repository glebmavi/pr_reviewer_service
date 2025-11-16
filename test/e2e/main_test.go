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
	if body != nil {
		jsonData, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewBuffer(jsonData)
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
			{UserID: "", Username: "A"},
			{UserID: "", Username: "B"},
			{UserID: "", Username: "C"},
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
		"author_id":         createdTeam.Members[0].UserID,
	}
	resp, body = doRequest(t, "POST", "/pullRequest/create", prPayload)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdPR PullRequest
	unmarshalResponse(t, body, &createdPR)
	prID := createdPR.PullRequestID

	assert.Equal(t, "feat: new feature", createdPR.PullRequestName)
	assert.Equal(t, createdTeam.Members[0].UserID, createdPR.AuthorID)
	assert.Equal(t, "OPEN", createdPR.Status)
	assert.Len(t, createdPR.AssignedReviewers, 2)
	assert.NotContains(t, createdPR.AssignedReviewers, createdTeam.Members[0].UserID) // Author should not be a reviewer
	assert.Contains(t, createdPR.AssignedReviewers, createdTeam.Members[1].UserID)
	assert.Contains(t, createdPR.AssignedReviewers, createdTeam.Members[2].UserID)

	// 3. Get the PR and verify its state
	resp, body = doRequest(t, "GET", "/pullRequest/get/"+prID, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var fetchedPR PullRequest
	unmarshalResponse(t, body, &fetchedPR)
	assert.Equal(t, prID, fetchedPR.PullRequestID)
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
		"old_user_id":     createdTeam.Members[2].UserID,
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
			{UserID: "", Username: "D"},
			{UserID: "", Username: "E"},
		},
	}
	resp, body := doRequest(t, "POST", "/team/add", teamPayload)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdTeam Team
	unmarshalResponse(t, body, &createdTeam)

	// 2. Create a PR by D, should assign only E
	prPayload := map[string]string{
		"pull_request_name": "fix: css bug",
		"author_id":         createdTeam.Members[0].UserID,
	}
	resp, body = doRequest(t, "POST", "/pullRequest/create", prPayload)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdPR PullRequest
	unmarshalResponse(t, body, &createdPR)
	assert.Len(t, createdPR.AssignedReviewers, 1)
	assert.Equal(t, createdTeam.Members[1].UserID, createdPR.AssignedReviewers[0])

	// 3. Create a team with 1 member
	teamPayloadSolo := Team{
		TeamName: "solo-squad",
		Members:  []TeamMember{{UserID: "", Username: "F"}},
	}
	resp, body = doRequest(t, "POST", "/team/add", teamPayloadSolo)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdTeamSolo Team
	unmarshalResponse(t, body, &createdTeamSolo)

	// 4. Create a PR by F, should assign 0 reviewers
	prPayloadSolo := map[string]string{
		"pull_request_name": "docs: update readme",
		"author_id":         createdTeamSolo.Members[0].UserID,
	}
	resp, body = doRequest(t, "POST", "/pullRequest/create", prPayloadSolo)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdPRSolo PullRequest
	unmarshalResponse(t, body, &createdPRSolo)
	assert.Len(t, createdPRSolo.AssignedReviewers, 0)
}
