package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataSourceName(t *testing.T) {
	type args struct {
		configPath string
		name       string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "default",
			args: args{
				configPath: "",
				name:       "tachi-sync-server.db",
			},
			want: "tachi-sync-server.db",
		},
		{
			name: "path_1",
			args: args{
				configPath: "/config",
				name:       "tachi-sync-server.db",
			},
			want: "/config/tachi-sync-server.db",
		},
		{
			name: "path_2",
			args: args{
				configPath: "/config/",
				name:       "tachi-sync-server.db",
			},
			want: "/config/tachi-sync-server.db",
		},
		{
			name: "path_3",
			args: args{
				configPath: "/config//",
				name:       "tachi-sync-server.db",
			},
			want: "/config/tachi-sync-server.db",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dataSourceName(tt.args.configPath, tt.args.name)
			assert.Equal(t, tt.want, got)
		})
	}
}
