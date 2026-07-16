package database

import (
	"testing"
)

func TestDataSourceName(t *testing.T) {
	tests := []struct {
		name       string
		configPath string
		dbName     string
		want       string
	}{
		{
			name:       "default",
			configPath: "",
			dbName:     "syncyomi.db",
			want:       "syncyomi.db",
		},
		{
			name:       "path_1",
			configPath: "/config",
			dbName:     "syncyomi.db",
			want:       "/config/syncyomi.db",
		},
		{
			name:       "path_2",
			configPath: "/config/",
			dbName:     "syncyomi.db",
			want:       "/config/syncyomi.db",
		},
		{
			name:       "path_3",
			configPath: "/config//",
			dbName:     "syncyomi.db",
			want:       "/config/syncyomi.db",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dataSourceName(tt.configPath, tt.dbName)
			if got != tt.want {
				t.Errorf("dataSourceName() = %q, want %q", got, tt.want)
			}
		})
	}
}
