# **Go Development & TDD Guidelines**

This document outlines the Test-Driven Development (TDD) philosophy and the general Go development best practices for the patience CLI project. Adhering to these guidelines is mandatory to ensure a high-quality, maintainable, and robust codebase.

## **Part 1: Test-Driven Development (TDD)**

### **1.1. TDD Philosophy: Red, Green, Refactor**

We will strictly follow the "Red, Green, Refactor" cycle for all new feature development.

1. **RED \- Write a Failing Test:** Before writing any implementation code, write a test that describes a small, specific piece of desired functionality. The test must fail because the functionality does not yet exist. This proves that the test is valid and not producing a false positive.  
2. **GREEN \- Write the Simplest Code to Pass the Test:** Write the absolute minimum amount of implementation code required to make the failing test pass. Do not add extra features or optimizations at this stage.  
3. **REFACTOR \- Improve the Code:** With the safety of a passing test suite, refactor the implementation code for clarity, efficiency, and to remove duplication. Ensure that the tests continue to pass after refactoring.

### **1.2. Testing Conventions**

* **File Naming:** Test files must be named \_test.go and reside in the same package as the code they are testing.  
* **Test Functions:** Test functions must start with Test and take t \*testing.T as their only parameter (e.g., func TestMyFunction(t \*testing.T)).  
* **Sub-tests:** Use t.Run() to create sub-tests for grouping related checks and improving output clarity, especially for table-driven tests.  
* **Table-Driven Tests:** Prefer table-driven tests for testing multiple input/output combinations of a single function. This keeps tests DRY (Don't Repeat Yourself).

### **1.3. Testing Tooling**

* **Standard Library:** The testing package is the foundation.  
* **Assertions:** We will use testify/require and testify/assert.  
  * Use require for checks that must pass for the rest of the test to make sense (e.g., error checks, setup). A require failure stops the test immediately.  
  * Use assert for all other checks. An assert failure reports the failure but allows the test function to continue.  
* **Mocks and Fakes:** For dependencies (like command execution or filesystem access), we will use Go interfaces and create hand-written fake implementations for testing. Avoid complex mocking frameworks in favor of clarity and simplicity.

### **1.4. Test Scope**

* **What to Test:**  
  * All exported functions and methods.  
  * Core business logic (retry loops, backoff calculations, condition checking).  
  * Edge cases (zero values, nil inputs, error conditions).  
  * A small number of integration tests to verify the interaction between major components (e.g., CLI parsing \-\> Executor).  
* **What NOT to Test:**  
  * Private functions directly (test them via the public API).  
  * Third-party libraries (assume they work; test that *our* code uses them correctly).  
  * Trivial getters/setters.

## **Part 2: General Go Development Practices**

### **2.1. Code Formatting**

* **gofmt is Law:** All Go code submitted to the repository **must** be formatted with gofmt. This is a non-negotiable standard that eliminates all debates about formatting.  
* **goimports is the Standard:** We will use goimports, a superset of gofmt that also automatically adds and removes import statements. Most Go-compatible editors can be configured to run goimports on save.  
* **CI Enforcement:** The CI pipeline will include a step that fails the build if any committed code is not formatted correctly.

### **2.2. Static Analysis & Linting**

We will use golangci-lint as our standard linter to enforce code quality and catch common bugs before they are committed.

* **Configuration:** A .golangci.yml file will be present in the root of the repository to ensure all developers use the same configuration.  
* **Key Enabled Linters (Example):**  
  * govet: Reports suspicious constructs.  
  * errcheck: Checks for unhandled errors.  
  * staticcheck: A suite of powerful static analysis checks.  
  * unused: Checks for unused code.  
  * ineffassign: Detects ineffectual assignments.  
  * gocritic: Provides diagnostics on style and performance.  
* **CI Enforcement:** The CI pipeline will run golangci-lint run on every commit. A failing lint check will fail the build.

### **2.3. Effective Go & Code Style**

We will adhere to the principles outlined in the official "Effective Go" documentation. Key takeaways for this project include:

* **Simplicity:** Prefer clear, simple code over clever or complex solutions.  
* **Interfaces:** Use interfaces to define behavior, not to describe data. Accept interfaces, return structs. This promotes decoupling and testability.  
* **Error Handling:** Errors are values. Handle errors explicitly; do not discard them. Use the errors package to wrap errors to provide context. Avoid panicking in library code.  
* **Concurrency:** When using goroutines, ensure there is a clear plan for managing their lifecycle. Use channels for communication and synchronization. Be mindful of race conditions (use go test \-race).  
* **Package Design:** Packages should have a clear, singular purpose. Avoid generic utility packages like utils or helpers.

### **2.4. Dependency Management**

* **Go Modules:** This project will use Go Modules for all dependency management. The go.mod file is the source of truth for all dependencies.  
* **Tidy Dependencies:** Before committing, always run go mod tidy to ensure the go.mod and go.sum files are clean and accurate.  
* **Dependency Updates:** Dependencies should be updated deliberately and tested thoroughly. Avoid depending on master branches of other repositories.

### **2.5. Documentation & Commenting**

* **Godoc:** All exported types, functions, and methods **must** have clear, concise comments that conform to the godoc standard. A good comment explains *what* the function does from the caller's perspective.  
* **Clarity over Brevity:** Comments should be complete sentences. For complex, unexported functions, add comments to explain the *why* behind the implementation.  
* **Tests as Documentation:** A well-written test serves as executable documentation. Test function names should be descriptive of the behavior they are testing.

### **2.6. Version Control**

* **Commit Messages:** We will use the [Conventional Commits](https://www.conventionalcommits.org/) specification. This creates an explicit commit history that is easy to read and can be used for automated changelog generation.  
  * Example: feat(executor): add support for stdout regex matching  
  * Example: fix(backoff): correct calculation in exponential strategy  
  * Example: docs(readme): update usage examples  
* **Small, Atomic Commits:** Each commit should represent a single logical change. Avoid large, monolithic commits that mix features, bug fixes, and refactoring.