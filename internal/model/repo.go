package model

// PackageInfo represents the package information for API responses
type PackageInfo struct {
	Name    string     `json:"name"`
	URL     string     `json:"url"`
	Versions []*Version `json:"versions"` // 按标签索引的版本信息
}

// Version represents a specific version of a package
type Version struct {
	File      string    `json:"file"`
	Size      int64     `json:"size"`
	Tag       string    `json:"tag"`
	Commit    string    `json:"commit"`
	Download  string    `json:"download"`
	UpdatedAt int64     `json:"updatedAt"`
}