package model

import (
	"time"
)

// DBRepo represents a repository record in the database
type DBRepo struct {
	ID         int64     `db:"id"`
	Name       string    `db:"name"`
	URL        string    `db:"url"`
	Tag        string    `db:"tag"`
	LastSync   time.Time `db:"last_sync"`
	CommitHash string    `db:"commit_hash"`
	ZipFile    string    `db:"zip_file"`
	Size       int64     `db:"size"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

// DBVersion represents a version record in the database
type DBVersion struct {
	ID         int64     `db:"id"`
	RepoID     int64     `db:"repo_id"`
	Tag        string    `db:"tag"`
	CommitHash string    `db:"commit_hash"`
	ZipFile    string    `db:"zip_file"`
	Size       int64     `db:"size"`
	CreatedAt  time.Time `db:"created_at"`
	Deleted    int       `db:"deleted"`
}

// Schema contains the SQL schema for the database
const Schema = `
CREATE TABLE IF NOT EXISTS repos (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    url TEXT NOT NULL,
    tag TEXT,
    last_sync TIMESTAMP NOT NULL,
    commit_hash TEXT NOT NULL,
    zip_file TEXT NOT NULL,
    size INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id INTEGER NOT NULL,
    tag TEXT NOT NULL,
    commit_hash TEXT NOT NULL,
    zip_file TEXT NOT NULL,
    size INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE,
    UNIQUE(repo_id, tag)
);

CREATE INDEX IF NOT EXISTS idx_repos_name ON repos(name);
CREATE INDEX IF NOT EXISTS idx_versions_repo_id ON versions(repo_id);
CREATE INDEX IF NOT EXISTS idx_versions_tag ON versions(tag);
`