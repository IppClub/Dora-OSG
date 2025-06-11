package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"go.uber.org/zap"
)

// Repo represents a Git repository
type Repo struct {
	Name     string
	URL      string
	Path     string
	LFS      bool
	Logger   *zap.Logger
}

// NewRepo creates a new Repo instance
func NewRepo(name, url, basePath string, lfs bool, logger *zap.Logger) *Repo {
	return &Repo{
		Name:   name,
		URL:    url,
		Path:   filepath.Join(basePath, "repos", name),
		LFS:    lfs,
		Logger: logger,
	}
}

// PullOrClone pulls or clones the repository
func (r *Repo) PullOrClone() error {
	repo, err := r.openOrClone()
	if err != nil {
		return fmt.Errorf("failed to open/clone repo: %w", err)
	}

	// Get the worktree
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Pull the latest changes
	err = worktree.Pull(&git.PullOptions{
		Force: true,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to pull: %w", err)
	}

	err = repo.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		Tags:       git.AllTags,
		Force:      true,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch: %w", err)
	}

	// If LFS is enabled, run 'git lfs pull' after pulling
	if r.LFS {
		gitLFSPath, err := exec.LookPath("git")
		if err != nil {
			r.Logger.Warn("git not found for LFS pull", zap.Error(err))
			return nil // Don't fail if git is not found, just warn
		}
		cmd := exec.Command(gitLFSPath, "lfs", "pull")
		cmd.Dir = r.Path
		output, err := cmd.CombinedOutput()
		if err != nil {
			r.Logger.Warn("git lfs pull failed", zap.Error(err), zap.ByteString("output", output))
			// Don't fail the whole operation, just warn
		} else {
			r.Logger.Info("git lfs pull succeeded", zap.ByteString("output", output))
		}
	}

	return nil
}

// GetLatestCommit returns the latest commit hash and tag
func (r *Repo) GetLatestCommit() (string, string, error) {
	repo, err := git.PlainOpen(r.Path)
	if err != nil {
		return "", "", fmt.Errorf("failed to open repo: %w", err)
	}

	// Get the HEAD reference
	ref, err := repo.Head()
	if err != nil {
		return "", "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Get the commit
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return "", "", fmt.Errorf("failed to get commit: %w", err)
	}

	// Get the tag
	tag := ""
	tags, err := repo.Tags()
	if err != nil {
		return "", "", fmt.Errorf("failed to get tags: %w", err)
	}

	tags.ForEach(func(t *plumbing.Reference) error {
		obj, err := repo.TagObject(t.Hash())
		if err != nil {
			return nil // Skip if not a tag object
		}
		if obj.Target == commit.Hash {
			tag = t.Name().Short()
		}
		return nil
	})

	return commit.Hash.String(), tag, nil
}

// openOrClone opens an existing repository or clones it if it doesn't exist
func (r *Repo) openOrClone() (*git.Repository, error) {
	// Try to open existing repository
	repo, err := git.PlainOpen(r.Path)
	if err == nil {
		return repo, nil
	}

	// If repository doesn't exist, clone it
	if err == git.ErrRepositoryNotExists {
		r.Logger.Info("cloning repository",
			zap.String("name", r.Name),
			zap.String("url", r.URL),
		)

		// Create directory if it doesn't exist
		if err := os.MkdirAll(r.Path, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}

		// Clone the repository
		repo, err = git.PlainClone(r.Path, false, &git.CloneOptions{
			URL:      r.URL,
			Progress: os.Stdout,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to clone: %w", err)
		}

		return repo, nil
	}

	return nil, err
}