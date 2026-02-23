# Contributing to Patience

This document outlines code quality standards and development practices for the Patience project.

## Code Quality Standards

This project uses automated quality gates to prevent regression of improvements made during TDD development cycles.

### Quality Gates Overview

The CI/CD pipeline enforces these quality standards:

#### 1. Function Complexity (TDD Cycle 1 Fix)
- **Maximum function length**: 120 lines
- **Maximum cyclomatic complexity**: 15
- **Rationale**: Prevents regression of the 216-line `Executor.Run` method that was refactored for maintainability

#### 2. Magic Number Prevention (TDD Cycle 1 Fix)
- **Required**: All numeric constants must be defined in `pkg/executor/constants.go`
- **Detection**: Automated checks ensure constants file exists and is used
- **Rationale**: Prevents hardcoded timeout values and improves maintainability

#### 3. Error Handling (TDD Cycle 2 Fix)
- **Required**: Proper error propagation patterns throughout the codebase
- **Minimum**: 20+ `if err != nil` patterns maintained
- **Rationale**: Ensures robust error handling and prevents silent failures

#### 4. Concurrency Safety (TDD Cycle 3 Fix)
- **Required**: Proper mutex usage for shared state
- **Minimum**: 5+ mutex instances maintained across the codebase
- **Race detection**: All tests run with `-race` flag
- **Rationale**: Prevents data races and ensures thread safety

#### 5. Type Safety (TDD Cycle 4 Fix)
- **Maximum**: 10 `interface{}` usages in non-test code
- **Preferred**: Use concrete types and generics where possible
- **Rationale**: Improves type safety and reduces runtime errors

#### 6. Memory Management (TDD Cycle 4 Fix)
- **Required**: Proper slice preallocation where capacity is known
- **Prohibited**: Manual `runtime.GC()` calls
- **Rationale**: Optimizes memory usage and prevents performance degradation

#### 7. Documentation Standards
- **Required**: All exported functions must have comments
- **Format**: Follow Go documentation conventions
- **Rationale**: Maintains code readability and API documentation

## Development Workflow

### 1. Local Development Setup

```bash
# Install pre-commit hooks (recommended)
pip install pre-commit
pre-commit install

# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### 2. Before Committing

Run these commands before committing:

```bash
# Format code
go fmt ./...

# Run linting (catches all regression patterns)
golangci-lint run --config=.golangci.yml

# Run tests with race detection
go test -race ./...

# Run benchmarks (smoke test)
go test -bench=. -benchtime=1s -run=^$ ./...
```

### 3. Pre-commit Hooks

Pre-commit hooks check for:
- Code formatting and imports
- Function complexity limits
- Magic number detection
- Concurrency safety patterns
- Type safety compliance
- General code quality issues

### 4. CI/CD Pipeline

All pull requests must pass the CI/CD pipeline:

- **Linting**: golangci-lint with comprehensive rules
- **Testing**: Unit tests with race detection on Go 1.21 and 1.22
- **Building**: Cross-platform builds (Linux, macOS, Windows)
- **Security**: Gosec security scanning
- **Coverage**: Code coverage analysis
- **Quality Gate**: All checks must pass before merge

## Code Style Guidelines

### Function Design
- Keep functions under 120 lines
- Limit cyclomatic complexity to 15
- Use descriptive names
- Single responsibility principle

### Error Handling
```go
// Good: Proper error propagation
result, err := someOperation()
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}

// Bad: Ignoring errors
result, _ := someOperation()
```

### Concurrency
```go
// Good: Proper mutex usage
type SafeCounter struct {
    mu    sync.Mutex
    count int
}

func (c *SafeCounter) Increment() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.count++
}

// Bad: Unprotected shared state
type UnsafeCounter struct {
    count int // Race condition risk
}
```

### Type Safety
```go
// Good: Concrete types
func ProcessCommand(cmd Command) error {
    // Implementation
}

// Acceptable: Constrained generics
func ProcessItems[T Processable](items []T) error {
    // Implementation
}

// Avoid: Excessive interface{} usage
func ProcessAnything(data interface{}) error {
    // Avoid this pattern
}
```

## Testing Standards

### Test Coverage
- Maintain high test coverage (aim for >80%)
- Include unit tests for all new functionality
- Add integration tests for complex workflows

### Race Detection
- All tests must pass with `-race` flag
- Test concurrent scenarios explicitly
- Use proper synchronization primitives

### Benchmarks
- Include benchmarks for performance-critical code
- Ensure benchmarks pass in CI (smoke test)
- Document performance expectations

## Pull Request Guidelines

### PR Description Template
```markdown
## Summary
Brief description of changes and motivation.

## Changes Made
- List specific changes
- Reference any issues fixed

## Testing
- [ ] Unit tests added/updated
- [ ] Integration tests pass
- [ ] Benchmarks pass
- [ ] Race detection clean

## Quality Checklist
- [ ] Function complexity under limits
- [ ] No magic numbers introduced
- [ ] Proper error handling
- [ ] Concurrency safety maintained
- [ ] Type safety preserved
- [ ] Documentation updated
```

### Review Process
1. Automated quality checks must pass
2. Code review by maintainer
3. Manual testing if needed
4. Merge after approval

## Quality Metrics

Current quality metrics (maintained by automation):

- **Function Complexity**: ✅ All functions under 120 lines
- **Magic Numbers**: ✅ All constants properly extracted
- **Error Handling**: ✅ Comprehensive error propagation
- **Concurrency Safety**: ✅ Race-free with proper synchronization
- **Type Safety**: ✅ Minimal interface{} usage
- **Test Coverage**: ✅ High coverage with race detection
- **Security**: ✅ No known vulnerabilities

## Getting Help

- **Issues**: Report bugs and feature requests via GitHub Issues
- **Discussions**: Use GitHub Discussions for questions
- **Documentation**: Check the README and inline documentation
- **Code Examples**: See the `examples/` directory

## Regression Prevention

Automated checks prevent regression of these fixes:

1. **TDD Cycle 1**: Function complexity and magic number elimination
2. **TDD Cycle 2**: Error handling improvements
3. **TDD Cycle 3**: Concurrency safety enhancements
4. **TDD Cycle 4**: Type safety and memory optimization

Any changes that would reintroduce these issues will be automatically rejected by our CI/CD pipeline.

---

These guidelines keep the codebase reliable and maintainable.