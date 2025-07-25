# **TDD Implementation Plan: patience CLI**

This document outlines the development plan for the patience CLI, following a strict Test-Driven Development (TDD) methodology. Development is broken down into cycles, each focusing on a specific, testable piece of functionality.

## **Cycle 0: Project Setup**

**Goal:** Initialize the project structure and CI pipeline.  
**Tasks:**

1. Initialize Go module: go mod init github.com/shaneisley/patience  
2. Set up directory structure (/cmd, /pkg).  
3. Set up basic CI (e.g., GitHub Actions) to run go build ./... and go test ./... on every push.  
4. Add testify as a dependency.

Test (Red): N/A. This cycle is setup-only.  
Code (Green): Project structure.  
Refactor: N/A.

## **Cycle 1: The Simplest Execution**

**Goal:** Execute a command once and exit. No retries.  
**Test (Red):**

* TestExecutor\_SuccessOnFirstTry:  
  * Given an executor configured for 1 attempt.  
  * And a command that will exit with code 0\.  
  * When Run() is called.  
  * Then the result should indicate success.  
  * And the command should have been executed exactly once.  
* TestExecutor\_FailureWithNoRetries:  
  * Given an executor configured for 1 attempt.  
  * And a command that will exit with code 1\.  
  * When Run() is called.  
  * Then the result should indicate failure.  
  * And the command should have been executed exactly once.

**Code (Green):**

* Create a basic executor package.  
* Implement a Run() function that uses os/exec to run a command.  
* It captures the exit code and returns a simple result struct.

**Refactor:**

* Clean up the Run() function.  
* Ensure the result struct is clear.  
* Introduce an interface for the command execution to facilitate future testing with fakes.

## **Cycle 2: Basic Retry Logic**

**Goal:** Implement a simple retry on failure.  
**Test (Red):**

* TestExecutor\_SucceedsOnSecondAttempt:  
  * Given an executor configured for 3 attempts.  
  * And a fake command that fails on the first call but succeeds on the second.  
  * When Run() is called.  
  * Then the result should be success.  
  * And the command should have been executed twice.  
* TestExecutor\_FailsAfterMaxAttempts:  
  * Given an executor configured for 3 attempts.  
  * And a fake command that always fails.  
  * When Run() is called.  
  * Then the result should be failure.  
  * And the command should have been executed three times.

**Code (Green):**

* Add a loop to the executor.Run() function.  
* Add a maxAttempts field to the configuration.  
* Check the exit code in the loop and break on success.

**Refactor:**

* Extract the command execution logic into a helper function.  
* Improve the clarity of the retry loop.

## **Cycle 3: Backoff Strategy (Fixed Delay)**

**Goal:** Introduce a simple, fixed delay between retries.  
**Test (Red):**

* TestExecutor\_WaitsForFixedDelay:  
  * Given an executor configured with a fixed delay of 50ms.  
  * And a fake command that always fails.  
  * When Run() is called.  
  * Then the total execution time should be slightly more than 100ms (for 3 attempts with 2 delays).  
  * We can test this by injecting a fake time.Sleep function.

**Code (Green):**

* Add a delay field to the configuration.  
* In the retry loop, call time.Sleep() with the fixed delay after a failure.

**Refactor:**

* Abstract the delay logic. Create a backoff package with a Strategy interface and a Fixed implementation. This will make it easy to add more strategies later.  
* The executor will take a backoff.Strategy as a dependency.

## **Cycle 4: Timeout Condition**

**Goal:** Add a per-attempt timeout.  
**Test (Red):**

* TestExecutor\_FailsOnTimeout:  
  * Given an executor configured with a 20ms timeout.  
  * And a command that sleeps for 100ms.  
  * When Run() is called.  
  * Then the result should be a failure due to timeout.  
  * And the attempt should have been terminated.

**Code (Green):**

* In the executor, use exec.CommandContext with a context.WithTimeout.  
* Check for context.DeadlineExceeded error.

**Refactor:**

* Clean up the context management and error handling within the attempt loop.

## **Cycle 5: CLI Integration (Cobra)**

**Goal:** Wire up the executor to a basic CLI.  
**Test (Red):**

* This will be more of an integration test.  
* TestCLI\_RunSimpleSuccessCommand:  
  * Execute the compiled patience binary as a subprocess (go build first).  
  * patience -- /bin/true  
  * Assert that the CLI exits with code 0\.  
* TestCLI\_RunSimpleFailCommand:  
  * patience --attempts 2 \-- /bin/false  
  * Assert that the CLI exits with a non-zero code.

**Code (Green):**

* Create the cmd/patience package.  
* Use cobra to set up the root command.  
* Add flags for \--attempts and \--delay.  
* Parse the command to be executed.  
* Instantiate and run the executor with the parsed configuration.

**Refactor:**

* Organize the Cobra command setup.  
* Separate parsing logic from execution logic.

## **Subsequent Cycles (High-Level)**

* **Cycle 6: Exponential Backoff:** Add an Exponential backoff strategy. Test that the delay increases correctly.  
* **Cycle 7: Output/Error Matching:** Implement success/failure conditions based on regex matching of stdout/stderr.  
* **Cycle 8: Configuration File:** Use viper to add support for a TOML configuration file.  
* **Cycle 9: Status Reporting:** Implement the detailed CLI output as specified in the architecture. Test the output strings.  
* **Cycle 10: Daemon Metrics:** Implement the async client to send metrics to the retryd daemon. Test that the client attempts to connect to the socket.  
* **Cycle 11+:** Implement the retryd daemon itself, following a similar TDD cycle for its components (listener, aggregator, HTTP endpoint).

## **Cycle 13: Project Metadata and Licensing**

**Goal:** Add proper project metadata, licensing, and ownership information.  
**Test (Red):**

* TestProjectMetadata_LicenseExists:
  * Given the project root directory.
  * When checking for LICENSE file.
  * Then MIT license should be present with Shane Isley as copyright holder.
* TestProjectMetadata_GoModuleCorrect:
  * Given the go.mod file.
  * When parsing the module path.
  * Then it should reference github.com/shaneisley/patience.
* TestProjectMetadata_ReadmeHasCorrectLinks:
  * Given the README.md file.
  * When parsing markdown links.
  * Then GitHub repository links should point to github.com/shaneisley/patience.

**Code (Green):**

* Update go.mod module path to github.com/shaneisley/patience.
* Create LICENSE file with MIT license and Shane Isley copyright.
* Update README.md with correct GitHub URLs and project ownership.
* Update all import paths in code to use new module path.
* Add proper project description and contact information.

**Refactor:**

* Ensure all documentation references are consistent.
* Verify all import statements use the correct module path.
* Update example configurations with proper project references.

## **Cycle 14: GitHub Repository Setup**

**Goal:** Initialize GitHub repository and push codebase.  
**Test (Red):**

* TestGitRepository_RemoteConfigured:
  * Given a local git repository.
  * When checking remote configuration.
  * Then origin should point to github.com/shaneisley/patience.
* TestGitRepository_InitialCommitExists:
  * Given the GitHub repository.
  * When checking commit history.
  * Then initial commit should contain all project files.
* TestGitRepository_BranchProtection:
  * Given the GitHub repository settings.
  * When checking branch protection rules.
  * Then main branch should have appropriate protections.

**Code (Green):**

* Create GitHub repository at github.com/shaneisley/patience.
* Configure git remote origin to point to GitHub repository.
* Create comprehensive .gitignore for Go projects.
* Push initial codebase with proper commit message.
* Set up branch protection rules for main branch.
* Configure repository settings (description, topics, etc.).

**Refactor:**

* Organize repository structure for public consumption.
* Ensure sensitive information is not committed.
* Verify all documentation is accurate for public repository.

## **Cycle 15: Distribution and Release Strategy**

**Goal:** Implement modern distribution strategies for the patience project.  
**Test (Red):**

* TestDistribution_GoReleaserConfig:
  * Given a .goreleaser.yaml configuration.
  * When validating the configuration.
  * Then it should build for multiple platforms and architectures.
* TestDistribution_GitHubActionsWorkflow:
  * Given .github/workflows/release.yml.
  * When triggered by a version tag.
  * Then it should create GitHub releases with binaries.
* TestDistribution_HomebrewFormula:
  * Given a Homebrew formula template.
  * When processing the formula.
  * Then it should correctly reference the GitHub repository.

**Code (Green):**

* Create .goreleaser.yaml for cross-platform builds and releases.
* Set up GitHub Actions workflow for automated releases.
* Create Homebrew formula template for macOS/Linux distribution.
* Add Dockerfile for container distribution.
* Create installation scripts for various package managers.
* Document distribution methods in README.md.

**Refactor:**

* Optimize build configurations for size and performance.
* Ensure consistent versioning across all distribution methods.
* Validate all distribution channels work correctly.