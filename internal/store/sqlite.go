package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ippclub/dora-osg/internal/model"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

// SQLiteStore implements the store interface using SQLite
type SQLiteStore struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewSQLiteStore creates a new SQLite store
func NewSQLiteStore(dataPath string, logger *zap.Logger) (*SQLiteStore, error) {
	dbPath := filepath.Join(dataPath, "dora-osg.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize schema
	if _, err := db.Exec(model.Schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &SQLiteStore{
		db:     db,
		logger: logger,
	}, nil
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// UpsertRepo updates or inserts a repository record
func (s *SQLiteStore) UpsertRepo(repo *model.DBRepo) error {
	query := `
		INSERT INTO repos (name, url, tag, last_sync, commit_hash, zip_file, size, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			tag = excluded.tag,
			last_sync = excluded.last_sync,
			commit_hash = excluded.commit_hash,
			zip_file = excluded.zip_file,
			size = excluded.size,
			updated_at = excluded.updated_at
		RETURNING id
	`

	repo.UpdatedAt = time.Now()
	err := s.db.QueryRow(
		query,
		repo.Name,
		repo.URL,
		repo.Tag,
		repo.LastSync,
		repo.CommitHash,
		repo.ZipFile,
		repo.Size,
		repo.UpdatedAt,
	).Scan(&repo.ID)

	if err != nil {
		return fmt.Errorf("failed to upsert repo: %w", err)
	}

	return nil
}

// AddVersion adds a new version record
func (s *SQLiteStore) AddVersion(version *model.DBVersion) error {
	query := `
		INSERT INTO versions (repo_id, tag, commit_hash, zip_file, size, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(repo_id, tag) DO UPDATE SET
			commit_hash = excluded.commit_hash,
			zip_file = excluded.zip_file,
			size = excluded.size,
			created_at = excluded.created_at
		RETURNING id
	`

	err := s.db.QueryRow(
		query,
		version.RepoID,
		version.Tag,
		version.CommitHash,
		version.ZipFile,
		version.Size,
		version.CreatedAt,
	).Scan(&version.ID)

	if err != nil {
		return fmt.Errorf("failed to add version: %w", err)
	}

	return nil
}

// GetRepoByName gets a repository by name
func (s *SQLiteStore) GetRepoByName(name string) (*model.DBRepo, error) {
	query := `SELECT * FROM repos WHERE name = ?`
	repo := &model.DBRepo{}
	err := s.db.QueryRow(query, name).Scan(
		&repo.ID,
		&repo.Name,
		&repo.URL,
		&repo.Tag,
		&repo.LastSync,
		&repo.CommitHash,
		&repo.ZipFile,
		&repo.Size,
		&repo.CreatedAt,
		&repo.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("repo not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get repo: %w", err)
	}
	return repo, nil
}

// GetVersionsByRepoID gets all versions of a repository
func (s *SQLiteStore) GetVersionsByRepoID(repoID int64, limit int) ([]*model.DBVersion, error) {
	query := `SELECT * FROM versions WHERE repo_id = ? ORDER BY created_at DESC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.Query(query, repoID)
	if err != nil {
		return nil, fmt.Errorf("failed to query versions: %w", err)
	}
	defer rows.Close()

	var versions []*model.DBVersion
	for rows.Next() {
		version := &model.DBVersion{}
		err := rows.Scan(
			&version.ID,
			&version.RepoID,
			&version.Tag,
			&version.CommitHash,
			&version.ZipFile,
			&version.Size,
			&version.CreatedAt,
			&version.Deleted,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan version: %w", err)
		}
		versions = append(versions, version)
	}

	return versions, nil
}

// GetAllRepos gets all repositories
func (s *SQLiteStore) GetAllRepos() ([]*model.DBRepo, error) {
	query := `SELECT * FROM repos ORDER BY name`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query repos: %w", err)
	}
	defer rows.Close()

	var repos []*model.DBRepo
	for rows.Next() {
		repo := &model.DBRepo{}
		err := rows.Scan(
			&repo.ID,
			&repo.Name,
			&repo.URL,
			&repo.Tag,
			&repo.LastSync,
			&repo.CommitHash,
			&repo.ZipFile,
			&repo.Size,
			&repo.CreatedAt,
			&repo.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan repo: %w", err)
		}
		repos = append(repos, repo)
	}

	return repos, nil
}

// Get older than 3 latest undeleted versions
func (s *SQLiteStore) GetOlderThan3LatestUnDeletedVersions(repoID int64) ([]*model.DBVersion, error) {
	query := `SELECT * FROM versions WHERE repo_id = ? AND deleted = 0 ORDER BY created_at DESC`
	rows, err := s.db.Query(query, repoID)
	if err != nil {
		return nil, fmt.Errorf("failed to query versions: %w", err)
	}
	defer rows.Close()

	var versions []*model.DBVersion
	skip := 0
	for rows.Next() {
		skip++
		if skip <= 3 {
			continue
		}
		version := &model.DBVersion{}
		err := rows.Scan(
			&version.ID,
			&version.RepoID,
			&version.Tag,
			&version.CommitHash,
			&version.ZipFile,
			&version.Size,
			&version.CreatedAt,
			&version.Deleted,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan version: %w", err)
		}
		versions = append(versions, version)
	}

	return versions, nil
}

// Mark version as deleted
func (s *SQLiteStore) MarkVersionAsDeleted(versionID int64) error {
	query := `UPDATE versions SET deleted = 1 WHERE id = ?`
	_, err := s.db.Exec(query, versionID)
	if err != nil {
		return fmt.Errorf("failed to mark version as deleted: %w", err)
	}
	return nil
}

// GetLatestPackageListVersion gets the latest package list version
func (s *SQLiteStore) GetLatestPackageListVersion() (*model.DBPackageListVersion, error) {
	query := `SELECT * FROM package_list_versions ORDER BY id DESC LIMIT 1`
	version := &model.DBPackageListVersion{}
	err := s.db.QueryRow(query).Scan(
		&version.ID,
		&version.Version,
		&version.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		// If no version exists, create initial version
		version.Version = 1
		version.UpdatedAt = time.Now()
		err = s.IncrementPackageListVersion()
		if err != nil {
			return nil, fmt.Errorf("failed to create initial version: %w", err)
		}
		return version, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest package list version: %w", err)
	}
	return version, nil
}

// IncrementPackageListVersion increments the package list version
func (s *SQLiteStore) IncrementPackageListVersion() error {
	query := `
		INSERT INTO package_list_versions (version, updated_at)
		SELECT COALESCE(MAX(version), 0) + 1, CURRENT_TIMESTAMP
		FROM package_list_versions
	`
	_, err := s.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to increment package list version: %w", err)
	}
	return nil
}
