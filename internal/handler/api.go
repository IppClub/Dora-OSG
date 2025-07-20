package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/ippclub/dora-osg/internal/config"
	"github.com/ippclub/dora-osg/internal/model"
	"github.com/ippclub/dora-osg/internal/service"
	"github.com/ippclub/dora-osg/internal/store"
	"go.uber.org/zap"
)

// API handles HTTP requests
type API struct {
	cfg         *config.Config
	logger      *zap.Logger
	store       *store.SQLiteStore
	syncService *service.SyncService
	rateLimiter *RateLimiter
	mu          sync.RWMutex
	cache       struct {
		packages     []byte
		version      []byte
		packageInfo  map[string][]byte
	}
}

// NewAPI creates a new API instance
func NewAPI(cfg *config.Config, logger *zap.Logger, syncService *service.SyncService) (*API, error) {
	// Initialize database store
	dbStore, err := store.NewSQLiteStore(cfg.Storage.Path, logger)
	if err != nil {
		return nil, err
	}

	// Initialize rate limiter
	rateLimiter := NewRateLimiter(float64(cfg.RateLimit.RPS), cfg.RateLimit.Burst)

	api := &API{
		cfg:         cfg,
		logger:      logger,
		store:       dbStore,
		syncService: syncService,
		rateLimiter: rateLimiter,
	}
	api.cache.packageInfo = make(map[string][]byte)

	// Initialize cache
	if err := api.UpdateCache(); err != nil {
		logger.Error("failed to initialize cache", zap.Error(err))
	}

	syncService.SetOnSyncCallback(func() {
		if err := api.UpdateCache(); err != nil {
			logger.Error("failed to update cache after sync", zap.Error(err))
		} else {
			logger.Info("cache updated after sync")
		}
	})

	return api, nil
}

// Close closes the API and its resources
func (a *API) Close() error {
	a.rateLimiter.Close()
	return a.store.Close()
}

// RegisterRoutes registers the API routes
func (a *API) RegisterRoutes(r chi.Router) {
	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)

	// API routes with rate limiting
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(a.rateLimiter.RateLimit)
		r.Get("/packages", a.listPackages)
		r.Get("/packages/{name}", a.getPackageVersions)
		r.Get("/packages/{name}/latest", a.getLatestPackage)
		r.Get("/package-list-version", a.getPackageListVersion)
	})

	// Admin routes (localhost only)
	r.Route("/admin", func(r chi.Router) {
		r.Use(LocalOnly)
		r.Post("/sync", a.triggerSync)
	})

	// Static file server for downloads with rate limiting
	zipDir := filepath.Join(a.cfg.Storage.Path, "zips")
	fileServer := http.FileServer(http.Dir(zipDir))
	r.Handle("/zips/*", a.rateLimiter.RateLimit(SecureFileServer(http.StripPrefix("/zips/", fileServer))))

	// Assets file server for JSON and JPG files
	assetsDir := filepath.Join(a.cfg.Storage.Path, "assets")
	assetsServer := http.FileServer(http.Dir(assetsDir))
	r.Handle("/assets/*", a.rateLimiter.RateLimit(SecureAssetsServer(http.StripPrefix("/assets/", assetsServer))))
}

// UpdateCache updates all cached responses
func (a *API) UpdateCache() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Update packages cache
	repos, err := a.store.GetAllRepos()
	if err != nil {
		return fmt.Errorf("failed to get repos: %w", err)
	}

	packages := make([]model.PackageInfo, 0, len(repos))
	for _, repo := range repos {
		versions, err := a.store.GetVersionsByRepoID(repo.ID, 3)
		if err != nil {
			a.logger.Error("failed to get versions",
				zap.String("repo", repo.Name),
				zap.Error(err),
			)
			continue
		}

		packageInfo := model.PackageInfo{
			Name:    repo.Name,
			URL:     repo.URL,
			Versions: make([]*model.Version, 0),
		}

		// Add all versions
		for _, v := range versions {
			packageInfo.Versions = append(packageInfo.Versions, &model.Version{
				File:      v.ZipFile,
				Size:      v.Size,
				Tag:       v.Tag,
				Commit:    v.CommitHash,
				Download:  a.getDownloadURL(v.ZipFile),
				UpdatedAt: v.CreatedAt.Unix(),
			})
		}

		packages = append(packages, packageInfo)

		// Cache individual package info
		packageInfoBytes, err := json.Marshal(packageInfo)
		if err != nil {
			a.logger.Error("failed to marshal package info",
				zap.String("repo", repo.Name),
				zap.Error(err),
			)
			continue
		}
		a.cache.packageInfo[repo.Name] = packageInfoBytes
	}

	// Cache all packages
	packagesBytes, err := json.Marshal(packages)
	if err != nil {
		return fmt.Errorf("failed to marshal packages: %w", err)
	}
	a.cache.packages = packagesBytes

	// Update version cache
	version, err := a.store.GetLatestPackageListVersion()
	if err != nil {
		return fmt.Errorf("failed to get package list version: %w", err)
	}

	versionResponse := struct {
		Version   int64 `json:"version"`
		UpdatedAt int64 `json:"updatedAt"`
	}{
		Version:   version.Version,
		UpdatedAt: version.UpdatedAt.Unix(),
	}

	versionBytes, err := json.Marshal(versionResponse)
	if err != nil {
		return fmt.Errorf("failed to marshal version: %w", err)
	}
	a.cache.version = versionBytes

	return nil
}

// listPackages returns a list of all packages
func (a *API) listPackages(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.cache.packages == nil {
		http.Error(w, "Cache not initialized", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(a.cache.packages)
}

// getPackageVersions returns all versions of a package
func (a *API) getPackageVersions(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "package name is required", http.StatusBadRequest)
		return
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	if cached, ok := a.cache.packageInfo[name]; ok {
		w.Header().Set("Content-Type", "application/json")
		w.Write(cached)
		return
	}

	http.Error(w, "package not found", http.StatusNotFound)
}

// getLatestPackage redirects to the latest version of a package
func (a *API) getLatestPackage(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "package name is required", http.StatusBadRequest)
		return
	}

	repo, err := a.store.GetRepoByName(name)
	if err != nil {
		a.logger.Error("failed to get repo",
			zap.String("name", name),
			zap.Error(err),
		)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if repo == nil {
		http.Error(w, "package not found", http.StatusNotFound)
		return
	}

	versions, err := a.store.GetVersionsByRepoID(repo.ID, 3)
	if err != nil {
		a.logger.Error("failed to get versions",
			zap.String("repo", name),
			zap.Error(err),
		)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if len(versions) == 0 {
		http.Error(w, "no versions found", http.StatusNotFound)
		return
	}

	// Redirect to the latest version
	http.Redirect(w, r, a.getDownloadURL(versions[0].ZipFile), http.StatusFound)
}

// triggerSync triggers a manual sync of all repositories
func (a *API) triggerSync(w http.ResponseWriter, r *http.Request) {
	a.logger.Info("manual sync triggered")

	// Start sync in a goroutine to avoid blocking
	go func() {
		if err := a.syncService.SyncAll(); err != nil {
			a.logger.Error("manual sync failed", zap.Error(err))
		} else {
			a.logger.Info("manual sync completed successfully")
		}
	}()

	// Return immediately with a 202 Accepted status
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "sync started",
		"message": "Repository synchronization has been triggered",
	})
}

// getDownloadURL returns the full download URL for a file
func (a *API) getDownloadURL(filename string) string {
	return a.cfg.Download.BaseURL + "/zips/" + filename
}

// getPackageListVersion returns the current version of the package list
func (a *API) getPackageListVersion(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.cache.version == nil {
		http.Error(w, "Cache not initialized", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(a.cache.version)
}