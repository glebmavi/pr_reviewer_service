-- name: CreateTeam :one
INSERT INTO teams (team_name)
VALUES ($1)
RETURNING *;

-- name: GetTeamByID :one
SELECT * FROM teams
WHERE team_id = $1;

-- name: GetTeamByName :one
SELECT * FROM teams
WHERE team_name = $1;

-- name: ListTeams :many
SELECT * FROM teams;

-- name: CountTeams :one
SELECT count(*) FROM teams;

-- name: UpdateTeamName :one
UPDATE teams
SET team_name = $2
WHERE team_id = $1
RETURNING *;

-- name: DeactivateTeam :one
UPDATE teams
SET is_active = false
WHERE team_id = $1
RETURNING *;

-- name: ActivateTeam :one
UPDATE teams
SET is_active = true
WHERE team_id = $1
RETURNING *;
