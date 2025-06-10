package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ippclub/dora-osg/internal/config"
	"github.com/ippclub/dora-osg/internal/model"
	"github.com/ippclub/dora-osg/internal/store"
	"github.com/ippclub/dora-osg/pkg/git"
	"github.com/ippclub/dora-osg/pkg/zip"
	"go.uber.org/zap"
)

// SyncService handles repository synchronization and packaging
type SyncService struct {
	cfg    *config.Config
	logger *zap.Logger
	store  *store.SQLiteStore
	repos  map[string]*git.Repo
	mu     sync.RWMutex
}

// NewSyncService creates a new SyncService instance
func NewSyncService(cfg *config.Config, logger *zap.Logger) (*SyncService, error) {
	// Initialize database store
	dbStore, err := store.NewSQLiteStore(cfg.Storage.Path, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create store: %w", err)
	}

	s := &SyncService{
		cfg:    cfg,
		logger: logger,
		store:  dbStore,
		repos:  make(map[string]*git.Repo),
	}

	// Initialize repositories
	for _, repo := range cfg.Repos {
		s.repos[repo.Name] = git.NewRepo(repo.Name, repo.URL, cfg.Storage.Path, repo.LFS, logger)
	}

	return s, nil
}

// Close closes the service and its resources
func (s *SyncService) Close() error {
	return s.store.Close()
}

// SyncAll synchronizes all repositories
func (s *SyncService) SyncAll() error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(s.repos))

	for _, repo := range s.repos {
		wg.Add(1)
		go func(r *git.Repo) {
			defer wg.Done()
			if err := s.syncRepo(r); err != nil {
				errChan <- fmt.Errorf("failed to sync repo %s: %w", r.Name, err)
			}
		}(repo)
	}

	wg.Wait()
	close(errChan)

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("sync errors: %v", errs)
	}

	return nil
}

// syncRepo synchronizes a single repository
func (s *SyncService) syncRepo(r *git.Repo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	latestCommitHash := ""
	{
		dbRepo, err := s.store.GetRepoByName(r.Name)
		if err == nil {
			latestCommitHash = dbRepo.CommitHash
		}
	}

	// Pull or clone the repository
	if err := r.PullOrClone(); err != nil {
		return err
	}

	// Get the latest commit and tag
	commitHash, tag, err := r.GetLatestCommit()
	if err != nil {
		return err
	}

	if len(latestCommitHash) >= 7 && latestCommitHash != commitHash {
		// Delete the latest zip file
		zipFilePath := filepath.Join(s.cfg.Storage.Path, "zips", fmt.Sprintf("%s-%s.zip", r.Name, latestCommitHash[:7]))
		s.logger.Info("deleting zip file", zap.String("repo", r.Name), zap.String("zip", zipFilePath))
		os.Remove(zipFilePath)
	}

	// Create zip file
	zipFileName := fmt.Sprintf("%s-%s.zip", r.Name, commitHash[:7])
	zipFilePath := filepath.Join(s.cfg.Storage.Path, "zips", zipFileName)

	if _, err := os.Stat(zipFilePath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := zip.CreateZip(r.Path, zipFilePath); err != nil {
			return err
		}
	} else {
		s.logger.Info("zip file already exists", zap.String("repo", r.Name), zap.String("zip", zipFileName))
		return nil
	}

	// Get file size
	size, err := zip.GetFileSize(zipFilePath)
	if err != nil {
		return err
	}

	// Update repository metadata
	dbRepo := &model.DBRepo{
		Name:       r.Name,
		URL:        r.URL,
		Tag:        tag,
		LastSync:   time.Now(),
		CommitHash: commitHash,
		ZipFile:    zipFileName,
		Size:       size,
	}

	if err := s.store.UpsertRepo(dbRepo); err != nil {
		return fmt.Errorf("failed to update repo metadata: %w", err)
	}

	// Add version record
	version := &model.DBVersion{
		RepoID:     dbRepo.ID,
		Tag:        tag,
		CommitHash: commitHash,
		ZipFile:    zipFileName,
		Size:       size,
		CreatedAt:  time.Now(),
	}

	if err := s.store.AddVersion(version); err != nil {
		return fmt.Errorf("failed to add version: %w", err)
	}

	s.logger.Info("repository synchronized",
		zap.String("name", r.Name),
		zap.String("commit", commitHash[:7]),
		zap.String("tag", tag),
	)

	// Delete older than 3 latest undeleted versions
	versions, err := s.store.GetOlderThan3LatestUnDeletedVersions(dbRepo.ID)
	if err != nil {
		return fmt.Errorf("failed to get older than 3 latest un deleted versions: %w", err)
	}
	for _, version := range versions {
		// Delete zip file
		zipFilePath := filepath.Join(s.cfg.Storage.Path, "zips", version.ZipFile)
		os.Remove(zipFilePath)
		// Mark version as deleted
		if err := s.store.MarkVersionAsDeleted(version.ID); err != nil {
			return fmt.Errorf("failed to mark version as deleted: %w", err)
		}
	}

	return nil
}
