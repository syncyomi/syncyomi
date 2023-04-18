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
				name:       "syncyomi.db",
			},
			want: "syncyomi.db",
		},
		{
			name: "path_1",
			args: args{
				configPath: "/config",
				name:       "syncyomi.db",
			},
			want: "/config/syncyomi.db",
		},
		{
			name: "path_2",
			args: args{
				configPath: "/config/",
				name:       "syncyomi.db",
			},
			want: "/config/syncyomi.db",
		},
		{
			name: "path_3",
			args: args{
				configPath: "/config//",
				name:       "syncyomi.db",
			},
			want: "/config/syncyomi.db",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dataSourceName(tt.args.configPath, tt.args.name)
			assert.Equal(t, tt.want, got)
		})
	}
}
