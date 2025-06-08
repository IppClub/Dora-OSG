package model

import "time"

// RepoMeta represents the metadata of a repository
type RepoMeta struct {
	Name       string    `json:"name"`
	URL        string    `json:"url"`
	Tag        string    `json:"tag"`
	LastSync   time.Time `json:"lastSync"`
	CommitHash string    `json:"commitHash"`
	ZipFile    string    `json:"zipFile"`   // 相对路径
	Size       int64     `json:"size"`
}

// PackageInfo represents the package information for API responses
type PackageInfo struct {
	Name    string     `json:"name"`
	Latest  *Version   `json:"latest"`
	Version map[string]*Version `json:"-"` // 按标签索引的版本信息
}

// Version represents a specific version of a package
type Version struct {
	File      string    `json:"file"`
	Size      int64     `json:"size"`
	Tag       string    `json:"tag"`
	Commit    string    `json:"commit"`
	Download  string    `json:"download"`
	UpdatedAt time.Time `json:"updatedAt"`
}