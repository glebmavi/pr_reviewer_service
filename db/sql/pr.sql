-- name: CreatePR :one
INSERT INTO pull_requests (pr_id, pr_name, author_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPRByID :one
SELECT * FROM pull_requests
WHERE pr_id = $1;

-- name: ListPRs :many
SELECT * FROM pull_requests;

-- name: CountPRs :one
SELECT count(*) FROM pull_requests;

-- name: AddReviewerToPR :exec
INSERT INTO review_assignments (pr_id, user_id)
VALUES ($1, $2);

-- name: GetReviewersForPR :many
SELECT u.*
FROM users u
JOIN review_assignments ra ON u.user_id = ra.user_id
WHERE ra.pr_id = $1;

-- name: MergePR :one
UPDATE pull_requests
SET status = 'MERGED',
    merged_at = NOW()
WHERE pr_id = $1
RETURNING *;

-- name: FindReplacementCandidates :many
SELECT u.*
FROM users u
WHERE u.team_id = $1      -- Команда, в которой ищем
  AND u.is_active = true    -- Только активные
  AND u.user_id != $2       -- Не автор PR
  AND u.user_id != ALL($3::varchar[]) -- Не те, кто уже назначен
ORDER BY random()
LIMIT $4;

-- name: RemoveReviewerFromPR :exec
DELETE FROM review_assignments
WHERE pr_id = $1 AND user_id = $2;

-- name: RemoveAllReviewersFromPR :exec
DELETE FROM review_assignments
WHERE pr_id = $1;

-- name: GetPRsForReviewer :many
SELECT pr.pr_id, pr.pr_name, pr.author_id, pr.status
FROM pull_requests pr
JOIN review_assignments ra ON pr.pr_id = ra.pr_id
WHERE ra.user_id = $1;

-- name: GetOpenReviewsForUsers :many
-- Находим все ОТКРЫТЫЕ ревью для списка пользователей
SELECT ra.pr_id, ra.user_id, pr.author_id
FROM review_assignments ra
JOIN pull_requests pr ON ra.pr_id = pr.pr_id
WHERE pr.status = 'OPEN'
  AND ra.user_id = ANY($1::text[]);

-- name: GetReviewStats :many
SELECT user_id, COUNT(*) AS review_count
FROM review_assignments
GROUP BY user_id
ORDER BY review_count DESC;

-- name: GetAuthorTeamByPR :one
SELECT t.*
FROM teams t
JOIN users u ON t.team_id = u.team_id
JOIN pull_requests pr ON u.user_id = pr.author_id
WHERE pr.pr_id = $1;

-- name: GetOpenPRsWithoutReviewers :many
SELECT pr.*
FROM pull_requests pr
LEFT JOIN review_assignments ra ON pr.pr_id = ra.pr_id
WHERE pr.status = 'OPEN'
GROUP BY pr.pr_id
HAVING COUNT(ra.user_id) = 0;

-- name: CountOpenReviewsByTeam :one
SELECT COUNT(pr.pr_id)
FROM pull_requests pr
JOIN review_assignments ra ON pr.pr_id = ra.pr_id
JOIN users u ON ra.user_id = u.user_id
WHERE u.team_id = $1 AND pr.status = 'OPEN';

-- name: CountOpenReviewsByUser :one
SELECT COUNT(pr.pr_id)
FROM pull_requests pr
JOIN review_assignments ra ON pr.pr_id = ra.pr_id
WHERE ra.user_id = $1 AND pr.status = 'OPEN';

-- name: CountMergedReviewsByTeam :one
SELECT COUNT(pr.pr_id)
FROM pull_requests pr
JOIN review_assignments ra ON pr.pr_id = ra.pr_id
JOIN users u ON ra.user_id = u.user_id
WHERE u.team_id = $1 AND pr.status = 'MERGED';

-- name: CountMergedReviewsByUser :one
SELECT COUNT(pr.pr_id)
FROM pull_requests pr
JOIN review_assignments ra ON pr.pr_id = ra.pr_id
WHERE ra.user_id = $1 AND pr.status = 'MERGED';
