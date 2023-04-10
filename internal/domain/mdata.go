package domain

import (
	"context"
	"time"
)

type MangaDataRepo interface {
	Store(ctx context.Context, mdata *MangaData) (*MangaData, error)
	Delete(ctx context.Context, id int) error
	Update(ctx context.Context, mdata *MangaData) (*MangaData, error)
	ListMangaData(ctx context.Context, apiKey string) ([]MangaData, error)
	GetMangaDataByApiKey(ctx context.Context, apiKey string) (*MangaData, error)
}

type MangaData struct {
	ID         int         `json:"id,omitempty"`
	Manga      []Manga     `json:"manga,omitempty"`
	Extensions []Extension `json:"extensions,omitempty"`
	Categories []Category  `json:"categories,omitempty"`
	UserApiKey *APIKey     `json:"user_api_key,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

type Manga struct {
	Source       int64     `json:"source,omitempty"`
	URL          string    `json:"url,omitempty"`
	Title        string    `json:"title,omitempty"`
	Artist       string    `json:"artist,omitempty"`
	Author       string    `json:"author,omitempty"`
	Description  string    `json:"description,omitempty"`
	Genre        []string  `json:"genre,omitempty"`
	Status       int       `json:"status,omitempty"`
	ThumbnailURL string    `json:"thumbnailUrl,omitempty"`
	DateAdded    int64     `json:"dateAdded,omitempty"`
	Viewer       int       `json:"viewer,omitempty"`
	Chapters     []Chapter `json:"chapters,omitempty"`
	Categories   []int     `json:"categories,omitempty"`
	ViewerFlags  int       `json:"viewer_flags,omitempty"`
	History      []History `json:"history,omitempty"`
}

type Chapter struct {
	URL           string `json:"url,omitempty"`
	Name          string `json:"name,omitempty"`
	Scanlator     string `json:"scanlator,omitempty"`
	Read          bool   `json:"read,omitempty"`
	DateFetch     int64  `json:"dateFetch,omitempty"`
	DateUpload    int64  `json:"dateUpload,omitempty"`
	ChapterNumber int    `json:"chapterNumber,omitempty"`
	SourceOrder   int    `json:"sourceOrder,omitempty"`
}

type History struct {
	URL          string `json:"url,omitempty"`
	LastRead     int64  `json:"lastRead,omitempty"`
	ReadDuration int    `json:"readDuration,omitempty"`
}

type Extension struct {
	Name     string `json:"name,omitempty"`
	SourceID int64  `json:"sourceId,omitempty"`
}

type Category struct {
	Name  string `json:"name,omitempty"`
	Flags int    `json:"flags,omitempty"`
	Order int    `json:"order,omitempty"`
}
