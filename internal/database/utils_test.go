package database

import (
	"testing"
)

// The toNull* helpers all treat the zero value as SQL NULL.

func TestToNullString(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		want      string
		wantValid bool
	}{
		{name: "empty is null", in: "", want: "", wantValid: false},
		{name: "value is valid", in: "syncyomi", want: "syncyomi", wantValid: true},
		{name: "whitespace is valid", in: " ", want: " ", wantValid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toNullString(tt.in)
			if got.String != tt.want || got.Valid != tt.wantValid {
				t.Errorf("toNullString(%q) = {String: %q, Valid: %v}, want {String: %q, Valid: %v}",
					tt.in, got.String, got.Valid, tt.want, tt.wantValid)
			}
		})
	}
}

func TestToNullInt32(t *testing.T) {
	tests := []struct {
		name      string
		in        int32
		wantValid bool
	}{
		{name: "zero is null", in: 0, wantValid: false},
		{name: "positive is valid", in: 42, wantValid: true},
		{name: "negative is valid", in: -1, wantValid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toNullInt32(tt.in)
			if got.Int32 != tt.in || got.Valid != tt.wantValid {
				t.Errorf("toNullInt32(%d) = {Int32: %d, Valid: %v}, want {Int32: %d, Valid: %v}",
					tt.in, got.Int32, got.Valid, tt.in, tt.wantValid)
			}
		})
	}
}

func TestToNullInt64(t *testing.T) {
	tests := []struct {
		name      string
		in        int64
		wantValid bool
	}{
		{name: "zero is null", in: 0, wantValid: false},
		{name: "positive is valid", in: 9000000000, wantValid: true},
		{name: "negative is valid", in: -1, wantValid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toNullInt64(tt.in)
			if got.Int64 != tt.in || got.Valid != tt.wantValid {
				t.Errorf("toNullInt64(%d) = {Int64: %d, Valid: %v}, want {Int64: %d, Valid: %v}",
					tt.in, got.Int64, got.Valid, tt.in, tt.wantValid)
			}
		})
	}
}

func TestToNullFloat64(t *testing.T) {
	tests := []struct {
		name      string
		in        float64
		wantValid bool
	}{
		{name: "zero is null", in: 0, wantValid: false},
		{name: "positive is valid", in: 1.5, wantValid: true},
		{name: "negative is valid", in: -1.5, wantValid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toNullFloat64(tt.in)
			if got.Float64 != tt.in || got.Valid != tt.wantValid {
				t.Errorf("toNullFloat64(%v) = {Float64: %v, Valid: %v}, want {Float64: %v, Valid: %v}",
					tt.in, got.Float64, got.Valid, tt.in, tt.wantValid)
			}
		})
	}
}

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
