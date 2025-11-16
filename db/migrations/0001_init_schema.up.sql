CREATE TYPE pr_status AS ENUM ('OPEN', 'MERGED');

CREATE TABLE teams (
    team_id SERIAL PRIMARY KEY,
    team_name VARCHAR(100) UNIQUE NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE users (
    user_id VARCHAR(100) PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    team_id INTEGER NOT NULL REFERENCES teams(team_id),
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE pull_requests (
    pr_id VARCHAR(100) PRIMARY KEY,
    pr_name VARCHAR(255) NOT NULL,
    author_id VARCHAR(100) NOT NULL REFERENCES users(user_id),
    status pr_status NOT NULL DEFAULT 'OPEN',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    merged_at TIMESTAMPTZ
);

CREATE TABLE review_assignments (
    pr_id VARCHAR(100) NOT NULL REFERENCES pull_requests(pr_id) ON DELETE CASCADE,
    user_id VARCHAR(100) NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    PRIMARY KEY (pr_id, user_id)
);

CREATE INDEX idx_users_team_id ON users(team_id);
CREATE INDEX idx_pr_author_id ON pull_requests(author_id);
CREATE INDEX idx_pr_status ON pull_requests(status);
CREATE INDEX idx_assignments_user_id ON review_assignments(user_id);