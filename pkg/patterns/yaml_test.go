package patterns

import (
	"testing"
)

func TestLoadHTTPPatternConfigFromYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		want    HTTPPatternConfig
	}{
		{
			name: "valid configuration",
			yaml: `
enable_status_routing: true
enable_header_routing: false
enable_api_detection: true
status_patterns:
  404: "not found"
  500: "internal.*error"
api_patterns:
  github:
    - "api\\.github\\.com"
    - "github\\.com/api"
default_pattern: ".*"
`,
			wantErr: false,
			want: HTTPPatternConfig{
				EnableStatusRouting: true,
				EnableHeaderRouting: false,
				EnableAPIDetection:  true,
				StatusPatterns: map[int]string{
					404: "not found",
					500: "internal.*error",
				},
				APIPatterns: map[APIType][]string{
					"github": {"api\\.github\\.com", "github\\.com/api"},
				},
				DefaultPattern: ".*",
			},
		},
		{
			name: "invalid YAML",
			yaml: `
enable_status_routing: true
invalid_yaml: [
`,
			wantErr: true,
		},
		{
			name: "status routing enabled but no patterns",
			yaml: `
enable_status_routing: true
status_patterns: {}
`,
			wantErr: true,
		},
		{
			name: "invalid regex pattern",
			yaml: `
enable_status_routing: true
status_patterns:
  404: "[invalid regex"
`,
			wantErr: true,
		},
		{
			name: "minimal valid configuration",
			yaml: `
enable_status_routing: false
enable_header_routing: false
enable_api_detection: false
`,
			wantErr: false,
			want: HTTPPatternConfig{
				EnableStatusRouting: false,
				EnableHeaderRouting: false,
				EnableAPIDetection:  false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadHTTPPatternConfigFromYAML([]byte(tt.yaml))

			if tt.wantErr {
				if err == nil {
					t.Error("LoadHTTPPatternConfigFromYAML() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("LoadHTTPPatternConfigFromYAML() unexpected error = %v", err)
				return
			}

			// Compare the configurations
			if got.EnableStatusRouting != tt.want.EnableStatusRouting {
				t.Errorf("EnableStatusRouting = %v, want %v", got.EnableStatusRouting, tt.want.EnableStatusRouting)
			}
			if got.EnableHeaderRouting != tt.want.EnableHeaderRouting {
				t.Errorf("EnableHeaderRouting = %v, want %v", got.EnableHeaderRouting, tt.want.EnableHeaderRouting)
			}
			if got.EnableAPIDetection != tt.want.EnableAPIDetection {
				t.Errorf("EnableAPIDetection = %v, want %v", got.EnableAPIDetection, tt.want.EnableAPIDetection)
			}
			if got.DefaultPattern != tt.want.DefaultPattern {
				t.Errorf("DefaultPattern = %v, want %v", got.DefaultPattern, tt.want.DefaultPattern)
			}

			// Compare status patterns
			if len(got.StatusPatterns) != len(tt.want.StatusPatterns) {
				t.Errorf("StatusPatterns length = %v, want %v", len(got.StatusPatterns), len(tt.want.StatusPatterns))
			}
			for code, pattern := range tt.want.StatusPatterns {
				if got.StatusPatterns[code] != pattern {
					t.Errorf("StatusPatterns[%d] = %v, want %v", code, got.StatusPatterns[code], pattern)
				}
			}

			// Compare API patterns
			if len(got.APIPatterns) != len(tt.want.APIPatterns) {
				t.Errorf("APIPatterns length = %v, want %v", len(got.APIPatterns), len(tt.want.APIPatterns))
			}
			for apiType, patterns := range tt.want.APIPatterns {
				gotPatterns := got.APIPatterns[apiType]
				if len(gotPatterns) != len(patterns) {
					t.Errorf("APIPatterns[%s] length = %v, want %v", apiType, len(gotPatterns), len(patterns))
					continue
				}
				for i, pattern := range patterns {
					if gotPatterns[i] != pattern {
						t.Errorf("APIPatterns[%s][%d] = %v, want %v", apiType, i, gotPatterns[i], pattern)
					}
				}
			}
		})
	}
}
