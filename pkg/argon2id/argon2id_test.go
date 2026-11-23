package argon2id

import "testing"

func TestDefaultParamsFollowRFC9106(t *testing.T) {
	if DefaultParams.Iterations != 3 || DefaultParams.Memory != 64*1024 || DefaultParams.Parallelism != 4 {
		t.Errorf("DefaultParams = %+v, want t=3, 64 MiB, p=4", DefaultParams)
	}
}

func TestLegacyParamsMatchTheOldDefaults(t *testing.T) {
	if LegacyParams.Iterations != 1 || LegacyParams.Memory != 64*1024 || LegacyParams.Parallelism != 2 {
		t.Errorf("LegacyParams = %+v, want t=1, 64 MiB, p=2", LegacyParams)
	}
}

func TestParams_SameCost(t *testing.T) {
	base := Params{Memory: 8, Iterations: 1, Parallelism: 2, SaltLength: 16, KeyLength: 32}

	tests := []struct {
		name  string
		other Params
		want  bool
	}{
		{"identical", base, true},
		{"different salt length only", Params{Memory: 8, Iterations: 1, Parallelism: 2, SaltLength: 8, KeyLength: 32}, true},
		{"different memory", Params{Memory: 16, Iterations: 1, Parallelism: 2, SaltLength: 16, KeyLength: 32}, false},
		{"different iterations", Params{Memory: 8, Iterations: 2, Parallelism: 2, SaltLength: 16, KeyLength: 32}, false},
		{"different parallelism", Params{Memory: 8, Iterations: 1, Parallelism: 4, SaltLength: 16, KeyLength: 32}, false},
		{"different key length", Params{Memory: 8, Iterations: 1, Parallelism: 2, SaltLength: 16, KeyLength: 16}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := base.SameCost(&tt.other); got != tt.want {
				t.Errorf("SameCost() = %v, want %v", got, tt.want)
			}
		})
	}
}
