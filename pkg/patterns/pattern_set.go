package patterns

import (
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed predefined/*.yaml
var predefinedPatterns embed.FS

// Priority represents the priority level of a pattern
type Priority int

const (
	PriorityLow Priority = iota
	PriorityMedium
	PriorityHigh
	PriorityCritical
)

// String returns the string representation of the priority
func (p Priority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityMedium:
		return "medium"
	case PriorityHigh:
		return "high"
	case PriorityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// UnmarshalYAML implements custom YAML unmarshaling for Priority
func (p *Priority) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}

	switch strings.ToLower(s) {
	case "low":
		*p = PriorityLow
	case "medium":
		*p = PriorityMedium
	case "high":
		*p = PriorityHigh
	case "critical":
		*p = PriorityCritical
	default:
		return fmt.Errorf("invalid priority: %s", s)
	}

	return nil
}

// Category represents the category of a pattern
type Category int

const (
	CategorySuccess Category = iota
	CategoryError
	CategoryWarning
	CategoryInfo
	CategoryRetry
)

// String returns the string representation of the category
func (c Category) String() string {
	switch c {
	case CategorySuccess:
		return "success"
	case CategoryError:
		return "error"
	case CategoryWarning:
		return "warning"
	case CategoryInfo:
		return "info"
	case CategoryRetry:
		return "retry"
	default:
		return "unknown"
	}
}

// UnmarshalYAML implements custom YAML unmarshaling for Category
func (c *Category) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}

	switch strings.ToLower(s) {
	case "success":
		*c = CategorySuccess
	case "error":
		*c = CategoryError
	case "warning":
		*c = CategoryWarning
	case "info":
		*c = CategoryInfo
	case "retry":
		*c = CategoryRetry
	default:
		return fmt.Errorf("invalid category: %s", s)
	}

	return nil
}

// PatternDefinition defines a single pattern within a pattern set
type PatternDefinition struct {
	Pattern     string                 `yaml:"pattern" json:"pattern"`
	Description string                 `yaml:"description" json:"description"`
	Priority    Priority               `yaml:"priority" json:"priority"`
	Category    Category               `yaml:"category" json:"category"`
	Tags        []string               `yaml:"tags,omitempty" json:"tags,omitempty"`
	Metadata    map[string]interface{} `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// PatternSet represents a collection of related patterns
type PatternSet struct {
	Name        string                       `yaml:"name" json:"name"`
	Version     string                       `yaml:"version" json:"version"`
	Description string                       `yaml:"description" json:"description"`
	Author      string                       `yaml:"author,omitempty" json:"author,omitempty"`
	Patterns    map[string]PatternDefinition `yaml:"patterns" json:"patterns"`
	Metadata    map[string]interface{}       `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// MatchResult represents the result of pattern matching against a pattern set
type MatchResult struct {
	Matched     bool                   `json:"matched"`
	PatternName string                 `json:"pattern_name,omitempty"`
	Pattern     *PatternDefinition     `json:"pattern,omitempty"`
	MatchTime   time.Duration          `json:"match_time"`
	Context     map[string]interface{} `json:"context,omitempty"`
}

// LoadPatternSetFromYAML loads a pattern set from YAML data
func LoadPatternSetFromYAML(data []byte) (*PatternSet, error) {
	var patternSet PatternSet

	if err := yaml.Unmarshal(data, &patternSet); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate the loaded pattern set
	if err := patternSet.Validate(); err != nil {
		return nil, fmt.Errorf("pattern set validation failed: %w", err)
	}

	return &patternSet, nil
}

// LoadPredefinedPatternSet loads a predefined pattern set by name
func LoadPredefinedPatternSet(name string) (*PatternSet, error) {
	filename := fmt.Sprintf("predefined/%s.yaml", name)

	data, err := predefinedPatterns.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("predefined pattern set '%s' not found: %w", name, err)
	}

	return LoadPatternSetFromYAML(data)
}

// ListAvailablePatternSets returns a list of available predefined pattern sets
func ListAvailablePatternSets() []string {
	entries, err := predefinedPatterns.ReadDir("predefined")
	if err != nil {
		return []string{}
	}

	var sets []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			name := strings.TrimSuffix(entry.Name(), ".yaml")
			sets = append(sets, name)
		}
	}

	sort.Strings(sets)
	return sets
}

// Match attempts to match the input against all patterns in the set
func (ps *PatternSet) Match(input string) (*MatchResult, error) {
	return ps.MatchWithContext(input, nil)
}

// MatchWithContext attempts to match the input with context variables
func (ps *PatternSet) MatchWithContext(input string, context map[string]interface{}) (*MatchResult, error) {
	start := time.Now()

	// Track matches by priority (higher priority = higher number)
	var bestMatch *MatchResult
	var bestPriority Priority = -1

	// Try each pattern in the set
	for name, pattern := range ps.Patterns {
		matcher, err := NewJSONPatternMatcher(pattern.Pattern)
		if err != nil {
			continue // Skip invalid patterns
		}

		matched, err := matcher.MatchWithContext(input, context)
		if err != nil {
			continue // Skip patterns that error
		}

		if matched {
			// If this is a higher priority match, or first match, use it
			if pattern.Priority > bestPriority {
				bestMatch = &MatchResult{
					Matched:     true,
					PatternName: name,
					Pattern:     &pattern,
					MatchTime:   time.Since(start),
					Context:     context,
				}
				bestPriority = pattern.Priority
			}
		}
	}

	// If we found a match, return it
	if bestMatch != nil {
		bestMatch.MatchTime = time.Since(start)
		return bestMatch, nil
	}

	// No match found
	return &MatchResult{
		Matched:   false,
		MatchTime: time.Since(start),
		Context:   context,
	}, nil
}

// Validate validates the pattern set structure and all patterns
func (ps *PatternSet) Validate() error {
	if ps.Name == "" {
		return fmt.Errorf("pattern set name is required")
	}

	if ps.Version == "" {
		return fmt.Errorf("pattern set version is required")
	}

	if len(ps.Patterns) == 0 {
		return fmt.Errorf("pattern set must contain at least one pattern")
	}

	// Validate each pattern
	for name, pattern := range ps.Patterns {
		if pattern.Pattern == "" {
			return fmt.Errorf("pattern '%s' has empty pattern string", name)
		}

		if pattern.Description == "" {
			return fmt.Errorf("pattern '%s' has empty description", name)
		}

		// Validate pattern syntax by trying to create a matcher
		_, err := NewJSONPatternMatcher(pattern.Pattern)
		if err != nil {
			return fmt.Errorf("pattern '%s' has invalid pattern syntax: %w", name, err)
		}
	}

	return nil
}

// GetPatternsByCategory returns all patterns in a specific category
func (ps *PatternSet) GetPatternsByCategory(category Category) map[string]PatternDefinition {
	result := make(map[string]PatternDefinition)
	for name, pattern := range ps.Patterns {
		if pattern.Category == category {
			result[name] = pattern
		}
	}
	return result
}

// GetPatternsByPriority returns all patterns with a specific priority or higher
func (ps *PatternSet) GetPatternsByPriority(minPriority Priority) map[string]PatternDefinition {
	result := make(map[string]PatternDefinition)
	for name, pattern := range ps.Patterns {
		if pattern.Priority >= minPriority {
			result[name] = pattern
		}
	}
	return result
}

// GetPatternsByTag returns all patterns that have a specific tag
func (ps *PatternSet) GetPatternsByTag(tag string) map[string]PatternDefinition {
	result := make(map[string]PatternDefinition)
	for name, pattern := range ps.Patterns {
		for _, t := range pattern.Tags {
			if t == tag {
				result[name] = pattern
				break
			}
		}
	}
	return result
}
