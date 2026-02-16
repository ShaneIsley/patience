# Documentation Maintenance Guide

This document outlines the processes and standards for maintaining the patience CLI documentation suite.

## Documentation Structure

The patience project maintains several documentation files:

- **README.md** - Main entry point, installation, quick start, and basic usage
- **Architecture.md** - Technical architecture and design decisions
- **Development-Guidelines.md** - Contributor guidelines and development standards
- **DAEMON.md** - Daemon setup, configuration, and API documentation
- **examples.md** - Comprehensive usage examples and real-world scenarios
- **DOCUMENTATION.md** - This file, maintenance processes and standards

## Maintenance Process

### 1. Code-Documentation Synchronization

**When to Update Documentation:**
- New features are added
- Existing features are modified or removed
- Configuration options change
- CLI interface changes
- Performance characteristics change significantly

**Responsibility:**
- Feature developers must update relevant documentation as part of their PR
- Documentation updates should be included in the same commit as code changes
- Reviewers must verify documentation accuracy before approving PRs

### 2. Documentation Review Cycle

**Monthly Review (1st of each month):**
- Review all documentation for accuracy
- Check for broken links and outdated examples
- Verify CLI help text matches documented options
- Update performance benchmarks if needed

**Release Review (Before each release):**
- Comprehensive documentation audit
- Update version-specific information
- Verify all examples work with current codebase
- Update changelog and release notes

### 3. Quality Standards

**Writing Standards:**
- Use clear, concise language
- Include practical examples for all features
- Maintain consistent terminology (see [Terminology](#terminology))
- Follow markdown best practices
- Include cross-references between related sections

**Technical Standards:**
- All code examples must be tested and working
- Include both basic and advanced usage examples
- Provide context for when to use each feature
- Include troubleshooting information for common issues

### 4. Testing Documentation

**Automated Testing:**
- CLI examples in documentation are tested in CI
- Link checking for internal and external references
- Markdown linting for consistency

**Manual Testing:**
- New user walkthrough using only documentation
- Verify examples work on different platforms
- Check that documentation matches actual CLI behavior

## Terminology

**Standardized Terms:**
- **patience** (lowercase) - The CLI tool itself
- **retry** - The action of attempting a command again
- **backoff strategy** - The algorithm determining delay between retries
- **attempt** - A single execution of the command
- **delay** - The time waited between attempts
- **timeout** - Maximum time allowed for a single attempt

**Note:** The CLI output uses a `[retry]` prefix (e.g., `[retry] Attempt 1/5 starting...`). When quoting actual CLI output, preserve this prefix. The terminology guidance below applies to prose in documentation, not to CLI output examples.

**Avoid These Terms in Prose:**
- "retry tool" (use "patience CLI" or just "patience")
- "backoff algorithm" (use "backoff strategy")
- "wait time" (use "delay")

## Cross-Reference Guidelines

**Internal Links:**
- Use relative links for internal documentation: `[Architecture](Architecture.md)`
- Link to specific sections: `[Strategy Details](README.md#strategy-details)`
- Always verify links work after changes

**External Links:**
- Include context for why external resources are relevant
- Prefer stable, authoritative sources
- Check external links during monthly reviews

## Visual Elements

**Diagrams:**
- Use ASCII art for simple diagrams that work in all environments
- Include alt text for accessibility
- Keep diagrams simple and focused

**Code Examples:**
- Use syntax highlighting: ```bash
- Include expected output when helpful
- Show both success and failure cases
- Use realistic examples, not just "foo" and "bar"

## Documentation Workflow

### For New Features

1. **Planning Phase:**
   - Identify documentation requirements
   - Plan where new content fits in existing structure
   - Consider user journey and discoverability

2. **Development Phase:**
   - Write documentation alongside code
   - Include examples and edge cases
   - Update related sections for consistency

3. **Review Phase:**
   - Technical review for accuracy
   - Editorial review for clarity and style
   - User testing with documentation only

4. **Release Phase:**
   - Final verification of all examples
   - Update cross-references
   - Announce documentation updates

### For Documentation-Only Changes

1. **Issue Identification:**
   - User feedback
   - Internal review findings
   - Outdated information

2. **Change Planning:**
   - Assess impact on other documents
   - Plan testing approach
   - Consider backward compatibility

3. **Implementation:**
   - Make changes with clear commit messages
   - Test all affected examples
   - Update cross-references

4. **Review:**
   - Peer review for accuracy
   - User testing if significant changes
   - Merge and announce if needed

## Metrics and Feedback

**Documentation Metrics:**
- User feedback on clarity and completeness
- Support ticket volume for documented features
- Time to productivity for new users
- Documentation coverage of features

**Feedback Channels:**
- GitHub issues for documentation problems
- User surveys for comprehensive feedback
- Developer feedback during code reviews
- Community discussions and questions

## Tools and Automation

**Recommended Tools:**
- Markdown linters for consistency
- Link checkers for broken references
- Spell checkers for quality
- CLI testing for example verification

**Automation:**
- CI checks for documentation quality
- Automated testing of code examples
- Link validation in pull requests
- Spell checking in CI pipeline

## Maintenance Schedule

**Weekly:**
- Monitor user feedback and issues
- Update examples if CLI behavior changes
- Quick review of recent changes

**Monthly:**
- Comprehensive link checking
- Review and update external references
- Check documentation metrics
- Plan improvements based on feedback

**Quarterly:**
- Major documentation structure review
- User journey optimization
- Performance benchmark updates
- Accessibility review

**Annually:**
- Complete documentation audit
- Major restructuring if needed
- Tool and process evaluation
- Long-term roadmap planning

## Contact

For questions about documentation maintenance:
- Create an issue with the `documentation` label
- Contact the maintainers directly
- Discuss in community channels

---

*This document is maintained as part of the patience CLI project. Last updated: 2026-02-15*