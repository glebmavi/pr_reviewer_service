-- name: CreateUser :one
INSERT INTO users (user_id, username, team_id, is_active)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserWithTeam :one
SELECT u.user_id, u.username, u.is_active, t.team_id, t.team_name, t.is_active as team_is_active
FROM users u
JOIN teams t ON u.team_id = t.team_id
WHERE u.user_id = $1;

-- name: ListUsers :many
SELECT * FROM users;

-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: GetUsersByIDs :many
SELECT * FROM users
WHERE user_id = ANY($1::varchar[]);

-- name: GetActiveUsersFromTeamExcluding :many
SELECT *
FROM users
WHERE team_id = $1
  AND is_active = true
  AND user_id != ALL ($2::varchar[]);

-- name: GetTeamMembers :many
SELECT * FROM users
WHERE team_id = $1;

-- name: UpdateUser :one
UPDATE users
SET username = $2,
    team_id = $3,
    is_active = $4
WHERE user_id = $1
RETURNING *;

-- name: SetUserActiveStatus :one
UPDATE users
SET is_active = $2
WHERE user_id = $1
RETURNING *;

-- name: MoveUserToTeam :one
UPDATE users
SET team_id = $2
WHERE user_id = $1
RETURNING *;

-- name: DeactivateUsersByTeam :many
UPDATE users
SET is_active = false
WHERE team_id = $1
  AND is_active = true
RETURNING user_id;
