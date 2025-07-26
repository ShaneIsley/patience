package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPAwareSubcommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedConfig HTTPAwareConfig
		wantErr        bool
	}{
		{
			name: "basic http-aware subcommand",
			args: []string{"http-aware", "--", "curl", "-i", "https://api.github.com"},
			expectedConfig: HTTPAwareConfig{
				Fallback: "exponential",
				MaxDelay: 30 * time.Minute,
			},
		},
		{
			name: "http-aware with custom fallback",
			args: []string{"http-aware", "--fallback", "linear", "--", "curl", "-i", "https://api.github.com"},
			expectedConfig: HTTPAwareConfig{
				Fallback: "linear",
				MaxDelay: 30 * time.Minute,
			},
		},
		{
			name: "http-aware with abbreviation",
			args: []string{"ha", "-f", "exp", "-m", "10m", "--", "curl", "-i", "https://api.github.com"},
			expectedConfig: HTTPAwareConfig{
				Fallback: "exp",
				MaxDelay: 10 * time.Minute,
			},
		},
		{
			name:    "http-aware without command separator",
			args:    []string{"http-aware", "curl", "-i", "https://api.github.com"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new root command for each test
			rootCmd := createTestRootCommand()

			// Set args and execute
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify the parsed configuration
			config := getLastParsedHTTPAwareConfig()
			assert.Equal(t, tt.expectedConfig.Fallback, config.Fallback)
			assert.Equal(t, tt.expectedConfig.MaxDelay, config.MaxDelay)
		})
	}
}

func TestExponentialSubcommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedConfig ExponentialConfig
		wantErr        bool
	}{
		{
			name: "basic exponential subcommand",
			args: []string{"exponential", "--", "curl", "https://api.github.com"},
			expectedConfig: ExponentialConfig{
				BaseDelay:  1 * time.Second,
				Multiplier: 2.0,
				MaxDelay:   60 * time.Second,
			},
		},
		{
			name: "exponential with custom parameters",
			args: []string{"exponential", "--base-delay", "500ms", "--multiplier", "1.5", "--max-delay", "30s", "--", "curl", "https://api.github.com"},
			expectedConfig: ExponentialConfig{
				BaseDelay:  500 * time.Millisecond,
				Multiplier: 1.5,
				MaxDelay:   30 * time.Second,
			},
		},
		{
			name: "exponential with abbreviations",
			args: []string{"exp", "-b", "2s", "-x", "3.0", "-m", "120s", "--", "curl", "https://api.github.com"},
			expectedConfig: ExponentialConfig{
				BaseDelay:  2 * time.Second,
				Multiplier: 3.0,
				MaxDelay:   120 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new root command for each test
			rootCmd := createTestRootCommand()

			// Set args and execute
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify the parsed configuration
			config := getLastParsedExponentialConfig()
			assert.Equal(t, tt.expectedConfig.BaseDelay, config.BaseDelay)
			assert.Equal(t, tt.expectedConfig.Multiplier, config.Multiplier)
			assert.Equal(t, tt.expectedConfig.MaxDelay, config.MaxDelay)
		})
	}
}

func TestArgumentSeparation(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		expectedCommand []string
		wantErr         bool
	}{
		{
			name:            "basic command separation",
			args:            []string{"http-aware", "--", "curl", "-i", "https://api.github.com"},
			expectedCommand: []string{"curl", "-i", "https://api.github.com"},
		},
		{
			name:            "command with flags after separator",
			args:            []string{"exponential", "-b", "1s", "--", "curl", "--fail", "--retry", "0", "https://api.github.com"},
			expectedCommand: []string{"curl", "--fail", "--retry", "0", "https://api.github.com"},
		},
		{
			name:    "missing command separator",
			args:    []string{"http-aware", "curl", "-i", "https://api.github.com"},
			wantErr: true,
		},
		{
			name:    "empty command after separator",
			args:    []string{"http-aware", "--"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new root command for each test
			rootCmd := createTestRootCommand()

			// Set args and execute
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify the parsed command
			command := getLastParsedCommand()
			assert.Equal(t, tt.expectedCommand, command)
		})
	}
}

func TestStrategyFactory(t *testing.T) {
	tests := []struct {
		name         string
		strategyType string
		config       interface{}
		expectedType string
		wantErr      bool
	}{
		{
			name:         "create http-aware strategy",
			strategyType: "http-aware",
			config: HTTPAwareConfig{
				Fallback: "exponential",
				MaxDelay: 30 * time.Minute,
			},
			expectedType: "*backoff.HTTPAware",
		},
		{
			name:         "create exponential strategy",
			strategyType: "exponential",
			config: ExponentialConfig{
				BaseDelay:  1 * time.Second,
				Multiplier: 2.0,
				MaxDelay:   60 * time.Second,
			},
			expectedType: "*backoff.Exponential",
		},
		{
			name:         "invalid strategy type",
			strategyType: "invalid",
			config:       HTTPAwareConfig{},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy, err := createStrategyFromConfig(tt.strategyType, tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, strategy)

			// Check strategy type (this will be implemented in the GREEN phase)
			strategyType := getStrategyTypeName(strategy)
			assert.Contains(t, strategyType, tt.expectedType)
		})
	}
}

func TestIntegrationWithExecutor(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name: "http-aware integration",
			args: []string{"http-aware", "--fallback", "exponential", "--", "echo", "test"},
		},
		{
			name: "exponential integration",
			args: []string{"exponential", "--base-delay", "1s", "--", "echo", "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new root command for each test
			rootCmd := createTestRootCommand()

			// Set args and execute
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test helper functions are now implemented in subcommands.go
