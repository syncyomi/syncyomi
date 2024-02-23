package domain

import (
	"context"
	"time"
)

type MangaDataRepo interface {
	Store(ctx context.Context, mdata *BackupData) (*BackupData, error)
	Delete(ctx context.Context, id int) error
	Update(ctx context.Context, mdata *BackupData) (*BackupData, error)
	ListMangaData(ctx context.Context, apiKey string) ([]BackupData, error)
	GetMangaDataByApiKey(ctx context.Context, apiKey string) (*BackupData, error)
}

type BackupData struct {
	ID                      int                       `json:"id"`
	BackupMangas            []BackupManga             `json:"backupManga"`
	BackupCategories        []BackupCategories        `json:"backupCategories"`
	BackupSources           []BackupSource            `json:"backupSources"`
	BackupPreferences       []BackupPreference        `json:"backupPreferences"`
	BackupSourcePreferences []BackupSourcePreferences `json:"backupSourcePreferences"`
	BackupSavedSearches     []BackupSavedSearch       `json:"backupSavedSearches"`
	UserApiKey              *APIKey                   `json:"user_api_key,omitempty"`
	CreatedAt               time.Time                 `json:"created_at"`
	UpdatedAt               time.Time                 `json:"updated_at"`
}

type BackupManga struct {
	Source                int64                        `json:"source"`
	URL                   string                       `json:"url"`
	Title                 string                       `json:"title"`
	Artist                *string                      `json:"artist,omitempty"`
	Author                *string                      `json:"author,omitempty"`
	Description           *string                      `json:"description,omitempty"`
	Genre                 []string                     `json:"genre"`
	Status                int                          `json:"status"`
	ThumbnailURL          *string                      `json:"thumbnailUrl,omitempty"`
	DateAdded             int64                        `json:"dateAdded"`
	Viewer                int                          `json:"viewer"`
	Chapters              []BackupChapter              `json:"chapters"`
	Categories            []int64                      `json:"categories"`
	Tracking              []BackupTracking             `json:"tracking"`
	Favorite              bool                         `json:"favorite"`
	ChapterFlags          int                          `json:"chapterFlags"`
	ViewerFlags           *int                         `json:"viewerFlags,omitempty"`
	History               []BackupHistory              `json:"history"`
	UpdateStrategy        UpdateStrategy               `json:"updateStrategy"`
	LastModifiedAt        int64                        `json:"lastModifiedAt"`
	FavoriteModifiedAt    *int64                       `json:"favoriteModifiedAt,omitempty"`
	MergedMangaReferences []BackupMergedMangaReference `json:"mergedMangaReferences"`
	FlatMetadata          *BackupFlatMetadata          `json:"flatMetadata,omitempty"`
	CustomStatus          int                          `json:"customStatus"`
	CustomTitle           *string                      `json:"customTitle,omitempty"`
	CustomArtist          *string                      `json:"customArtist,omitempty"`
	CustomAuthor          *string                      `json:"customAuthor,omitempty"`
	CustomDescription     *string                      `json:"customDescription,omitempty"`
	CustomGenre           []string                     `json:"customGenre,omitempty"`
	FilteredScanlators    *string                      `json:"filteredScanlators,omitempty"`
	Version               int64                        `json:"version"`
	IsSyncing             int64                        `json:"isSyncing"`
}

type BackupChapter struct {
	URL            string  `json:"url"`
	Name           string  `json:"name"`
	Scanlator      *string `json:"scanlator,omitempty"`
	Read           bool    `json:"read"`
	Bookmark       bool    `json:"bookmark"`
	LastPageRead   int64   `json:"lastPageRead"`
	DateFetch      int64   `json:"dateFetch"`
	DateUpload     int64   `json:"dateUpload"`
	ChapterNumber  float32 `json:"chapterNumber"`
	SourceOrder    int64   `json:"sourceOrder"`
	LastModifiedAt int64   `json:"lastModifiedAt"`
	Version        int64   `json:"version"`
	IsSyncing      int64   `json:"isSyncing"`
}

type BackupTracking struct {
	SyncID              int     `json:"syncId"`
	LibraryID           int64   `json:"libraryId"`
	MediaIDInt          int     `json:"mediaIdInt"`
	TrackingURL         string  `json:"trackingUrl"`
	Title               string  `json:"title"`
	LastChapterRead     float32 `json:"lastChapterRead"`
	TotalChapters       int     `json:"totalChapters"`
	Score               float32 `json:"score"`
	Status              int     `json:"status"`
	StartedReadingDate  int64   `json:"startedReadingDate"`
	FinishedReadingDate int64   `json:"finishedReadingDate"`
	MediaID             int64   `json:"mediaId"`
}

type BackupHistory struct {
	URL          string `json:"url"`
	LastRead     int64  `json:"lastRead"`
	ReadDuration int64  `json:"readDuration"`
}

type BackupSource struct {
	Name     string `json:"name"`
	SourceID int64  `json:"sourceId"`
}

type BackupCategories struct {
	Name  string `json:"name"`
	Flags int    `json:"flags"`
	Order int    `json:"order"`
}

type UpdateStrategy string

type BackupMergedMangaReference struct {
	IsInfoManga       bool   `json:"isInfoManga"`
	GetChapterUpdates bool   `json:"getChapterUpdates"`
	ChapterSortMode   int    `json:"chapterSortMode"`
	ChapterPriority   int    `json:"chapterPriority"`
	DownloadChapters  bool   `json:"downloadChapters"`
	MergeURL          string `json:"mergeUrl"`
	MangaURL          string `json:"mangaUrl"`
	MangaSourceID     int64  `json:"mangaSourceId"`
}

type BackupFlatMetadata struct {
	SearchMetadata BackupSearchMetadata `json:"searchMetadata"`
	SearchTags     []BackupSearchTag    `json:"searchTags"`
	SearchTitles   []BackupSearchTitle  `json:"searchTitles"`
}

type BackupSearchMetadata struct {
	Uploader     *string `json:"uploader,omitempty"`
	Extra        string  `json:"extra"`
	IndexedExtra *string `json:"indexedExtra,omitempty"`
	ExtraVersion int     `json:"extraVersion"`
}

type BackupSearchTag struct {
	Namespace *string `json:"namespace,omitempty"`
	Name      string  `json:"name"`
	Type      int     `json:"type"`
}

type BackupSearchTitle struct {
	Title string `json:"title"`
	Type  int    `json:"type"`
}

type BackupSourcePreferences struct {
	SourceKey string             `json:"sourceKey"`
	Prefs     []BackupPreference `json:"prefs"`
}

type BackupPreference struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

type BackupSavedSearch struct {
	Name       string `json:"name"`
	Query      string `json:"query"`
	FilterList string `json:"filterList"`
	Source     int64  `json:"source"`
}
