package version

import (
	"testing"
)

func TestGitHubReleaseChecker_checkNewVersion(t *testing.T) {
	type fields struct {
		Repo string
	}
	type args struct {
		version string
		release *Release
	}
	tests := []struct {
		name        string
		fields      fields
		args        args
		wantNew     bool
		wantVersion string
		wantErr     bool
	}{
		{
			name:   "outdated new available",
			fields: fields{},
			args: args{
				version: "v0.2.0",
				release: &Release{
					TagName:         "v0.3.0",
					TargetCommitish: "",
				},
			},
			wantNew:     true,
			wantVersion: "0.3.0",
			wantErr:     false,
		},
		{
			name:   "same version",
			fields: fields{},
			args: args{
				version: "v0.2.0",
				release: &Release{
					TagName:         "v0.2.0",
					TargetCommitish: "",
				},
			},
			wantNew:     false,
			wantVersion: "",
			wantErr:     false,
		},
		{
			name:   "no new version",
			fields: fields{},
			args: args{
				version: "v0.3.0",
				release: &Release{
					TagName:         "v0.2.0",
					TargetCommitish: "",
				},
			},
			wantNew:     false,
			wantVersion: "",
			wantErr:     false,
		},
		{
			name:   "new rc available",
			fields: fields{},
			args: args{
				version: "v0.3.0",
				release: &Release{
					TagName:         "v0.3.0-rc1",
					TargetCommitish: "",
				},
			},
			wantNew:     false,
			wantVersion: "",
			wantErr:     false,
		},
		{
			name:   "new rc available",
			fields: fields{},
			args: args{
				version: "v0.3.0-RC1",
				release: &Release{
					TagName:         "v0.3.0-RC2",
					TargetCommitish: "",
				},
			},
			wantNew:     true,
			wantVersion: "0.3.0-RC2",
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &Checker{
				Repo: tt.fields.Repo,
			}
			got, gotVersion, err := g.checkNewVersion(tt.args.version, tt.args.release)
			if tt.wantErr {
				if err == nil {
					t.Error("checkNewVersion() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("checkNewVersion() unexpected error: %v", err)
				}
			}
			if got != tt.wantNew {
				t.Errorf("checkNewVersion() got = %v, want %v", got, tt.wantNew)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("checkNewVersion() gotVersion = %q, want %q", gotVersion, tt.wantVersion)
			}
		})
	}
}

func Test_isDevelop(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{name: "test_1", version: "dev", want: true},
		{name: "test_2", version: "develop", want: true},
		{name: "test_3", version: "master", want: true},
		{name: "test_4", version: "latest", want: true},
		{name: "test_5", version: "v1.0.1", want: false},
		{name: "test_6", version: "1.0.1", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDevelop(tt.version); got != tt.want {
				t.Errorf("isDevelop(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}
