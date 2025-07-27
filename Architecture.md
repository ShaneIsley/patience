# **Architecture: patience CLI**

This document outlines the architecture for the modern, strategy-based patience command-line utility, written in Go.

## **1\. Core Philosophy**

The patience CLI is designed to be:

* **Strategy-Driven:** Each backoff strategy is a first-class citizen with its own subcommand and specialized configuration
* **HTTP-Intelligent:** Built-in HTTP response parsing for adaptive retry timing based on server hints
* **Reliable:** A trustworthy tool for wrapping and retrying critical commands in production environments
* **Observant:** Provides clear, actionable feedback and metrics with real-time status reporting
* **Flexible:** Supports a wide range of use cases, from simple scripts to complex distributed systems
* **Modern:** Leverages modern CLI patterns (subcommands) and system capabilities
* **Self-contained:** Core functionality works flawlessly without external dependencies. The daemon is optional.

## **2\. High-Level Components**

The system consists of three main components:

1. **patience CLI:** The primary executable with strategy-based subcommand architecture. Each strategy is a dedicated subcommand with specialized configuration and behavior.
2. **HTTP-Aware Intelligence:** Built-in HTTP response parsing that extracts retry timing from server responses (`Retry-After` headers, JSON fields).
3. **patienced Daemon (Optional):** A background process that collects, aggregates, and exposes long-term metrics. Not required for core functionality.

## **3\. patience CLI \- Detailed Architecture**

The CLI uses a modern subcommand-based architecture where each backoff strategy is a dedicated subcommand with specialized configuration.

### **3.1. Subcommand Architecture Design**

**Design Decision: Subcommands vs. Flags**

We chose subcommands over a flag-based approach for several key reasons:

1. **Strategy-Specific Configuration:** Each strategy has unique parameters (e.g., HTTP-aware has `--fallback`, exponential has `--multiplier`)
2. **Discoverability:** Users can explore strategies with `patience --help` and get strategy-specific help with `patience STRATEGY --help`
3. **Clarity:** `patience exponential --base-delay 1s` is clearer than `patience --backoff exponential --delay 1s`
4. **Extensibility:** Adding new strategies doesn't create flag conflicts or confusion
5. **Industry Standard:** Follows patterns used by `git`, `docker`, `kubectl`, and other modern CLI tools

**Available Strategies:**
- `http-aware` (`ha`) - HTTP response-aware delays
- `exponential` (`exp`) - Exponentially increasing delays  
- `linear` (`lin`) - Linearly increasing delays
- `fixed` (`fix`) - Fixed delay between retries
- `jitter` (`jit`) - Random jitter around base delay
- `decorrelated-jitter` (`dj`) - AWS-style decorrelated jitter
- `fibonacci` (`fib`) - Fibonacci sequence delays
- `polynomial` (`poly`) - Polynomial growth with configurable exponent
- `adaptive` (`adapt`) - Machine learning adaptive strategy

### **3.2. Execution Flow**

1. **Strategy Selection:** User selects strategy via subcommand (e.g., `patience exponential`)
2. **Configuration Parsing:** Strategy-specific flags are parsed along with common flags (attempts, timeout, patterns). Configuration follows precedence: CLI flags > environment variables > config file > defaults
3. **Strategy Initialization:** The appropriate backoff strategy is created with strategy-specific parameters
4. **Executor Creation:** An Executor is created with the strategy, common configuration, and optional condition checker
5. **Attempt Loop:** The executor enters a loop for each attempt:
   * Spawns the wrapped command as a subprocess
   * Captures stdout, stderr, and exit code
   * Monitors against configured timeouts
6. **Intelligent Condition Checking:** After each attempt:
   * **HTTP-Aware Processing:** For HTTP-aware strategy, parses response for `Retry-After` headers or JSON retry fields
   * **Pattern Matching:** Checks output against success/failure regex patterns
   * **Exit Code Evaluation:** Falls back to standard exit code checking
7. **Adaptive Delay Calculation:** If retry is needed:
   * **HTTP-Aware:** Uses server-provided timing or falls back to configured strategy
   * **Adaptive:** Records outcome and adjusts future delays based on learning algorithm
   * **Mathematical Strategies:** Use algorithm-specific calculations (exponential, linear, polynomial, etc.)
8. **Termination:** Loop terminates on success or max attempts reached
9. **Status Reporting:** Real-time attempt status and final summary via UI reporter
10. **Metrics Dispatch (Async):** Optional fire-and-forget metrics to daemon

### **3.2. Status and Output (Daemon Inactive)**

When patienced is not running, the CLI is the sole source of information. It provides detailed real-time and summary output directly to the terminal.

#### **Real-time (Per-Attempt) Output:**

* **Attempt Start:** A clear message indicating which attempt is starting.  
  * Example: \[retry\] Attempt 1/5 starting...  
* **Command Output:** The stdout and stderr from the wrapped command are streamed directly to the terminal by default.  
* **Attempt Failure:** When an attempt fails, retry immediately reports **why** it failed and the **delay** before the next attempt.  
  * Example (exit code): \[retry\] Attempt 1/5 failed (exit code: 1). Retrying in 2.1s.  
  * Example (timeout): \[retry\] Attempt 2/5 failed (timeout: 10s). Retrying in 4.5s.

#### **Final Summary Output:**

* **Final Status:** A conclusive message indicating success or failure.  
  * Success: ✅ \[retry\] Command succeeded after 3 attempts.  
  * Failure: ❌ \[retry\] Command failed after 5 attempts.  
* **Run Statistics:** A brief report detailing the execution:  
  * **Total Attempts:** The total number of times the command was run.  
  * **Successful Runs:** The number of successful attempts (will be 1 on success).  
  * **Failed Runs:** The number of failed attempts.  
  * **Total Duration:** The total time from the start of the first attempt to the end of the final attempt, including all delays.  
  * **Reason for Final State:** The specific reason for the final success or failure (e.g., Success on exit code: 0 or Failure due to max retries reached).

### **3.3. HTTP-Aware Intelligence**

The HTTP-aware strategy represents a significant architectural innovation:

**HTTP Response Parsing:**
- Extracts `Retry-After` headers from HTTP responses
- Parses JSON responses for retry timing fields (`retry_after`, `retryAfter`, `retry_in`)
- Handles both absolute timestamps and relative delays
- Gracefully falls back to configured strategy when no HTTP information is available

**Real-World Validation:**
- Tested against 7 major APIs: GitHub, Twitter, AWS, Stripe, Discord, Reddit, Slack
- Handles various HTTP response formats and timing conventions
- Respects server load and rate limiting automatically

**Fallback Strategy Integration:**
- Seamlessly integrates with any mathematical strategy as fallback
- Allows users to specify fallback behavior: `--fallback exponential`
- Maintains consistent behavior when HTTP information is unavailable

### **3.4. Go Package Structure**

/patience  
├── /cmd  
│   ├── /patience      \# Main package with subcommand architecture (Cobra)  
│   │   ├── main.go           \# Root command and strategy registration
│   │   └── subcommands.go    \# All strategy subcommand implementations
│   └── /patienced     \# Optional daemon for metrics aggregation
├── /pkg  
│   ├── /executor      \# Core logic for running and managing commands  
│   ├── /config        \# Configuration loading and validation  
│   ├── /backoff       \# All backoff strategies including HTTP-aware intelligence  
│   │   ├── strategy.go       \# Base strategy interface and core strategies
│   │   ├── http_aware.go     \# HTTP response parsing and adaptive timing
│   │   ├── adaptive.go       \# Machine learning adaptive strategy with EMA
│   │   └── polynomial.go     \# Polynomial growth strategy
│   ├── /conditions    \# Logic for checking success/failure conditions  
│   ├── /metrics       \# Structs and client for sending data to patienced  
│   ├── /ui            \# Terminal output and status reporting  
│   ├── /storage       \# Configuration and state persistence
│   └── /monitoring    \# Resource monitoring and performance tracking
├── /benchmarks        \# Performance testing infrastructure
├── /examples          \# Real-world usage examples and integration tests
└── /scripts           \# Installation, testing, and deployment scripts

## **4\. patienced Daemon \- Detailed Architecture**

The daemon is a long-running, optional background service for metrics aggregation.

### **4.1. Core Responsibilities**

* **Listen for Metrics:** Listens on a Unix socket (/tmp/patienced.sock or similar) for incoming metrics payloads from patience CLI instances.  
* **In-Memory Aggregation:** Stores and aggregates metrics in memory. This includes counters for successes/failures per command, histograms for durations, etc.  
* **Expose Metrics:** Exposes the aggregated metrics via an HTTP endpoint in a standard format (e.g., Prometheus).  
* **Persistence (Future):** May periodically flush metrics to disk to survive restarts.

### **4.2. Metrics Collection (Daemon Active)**

When the daemon is active, the following metrics are collected and aggregated:

* **Counters:**  
  * retry\_runs\_total{command, final\_status}: Total number of retry executions, labeled by the command hash and final status (succeeded, failed).  
  * retry\_attempts\_total{command, exit\_code}: Total number of individual command attempts, labeled by command hash and exit code.  
* **Histograms:**  
  * retry\_run\_duration\_seconds{command}: Histogram of total execution time for a retry run (including delays).  
  * retry\_attempt\_duration\_seconds{command}: Histogram of execution time for individual command attempts.  
* **Gauges:**  
  * patienced\_active: A gauge set to 1 indicating the daemon is running.

### **4.3. Communication**

* **CLI \-\> Daemon:** The patience CLI will send a UDP packet or a quick stream message containing a JSON or Protobuf payload to the Unix socket. This is connectionless and non-blocking to prevent slowing down the CLI.  
* **Scraper \-\> Daemon:** A monitoring system (like Prometheus) scrapes the /metrics HTTP endpoint exposed by patienced.

## **5\. Architectural Benefits**

### **5.1. Subcommand Architecture Benefits**

**User Experience:**
- **Intuitive Discovery:** `patience --help` shows all available strategies
- **Strategy-Specific Help:** `patience exponential --help` shows relevant options only
- **Shorter Commands:** `patience exp -b 1s` vs `patience --backoff exponential --delay 1s` (58% shorter)
- **No Flag Conflicts:** Each strategy can have unique flags without namespace pollution

**Developer Experience:**
- **Clean Separation:** Each strategy is self-contained with its own configuration struct
- **Easy Extension:** Adding new strategies doesn't affect existing code
- **Type Safety:** Strategy-specific configuration is strongly typed
- **Testability:** Each subcommand can be tested independently

**Maintenance Benefits:**
- **Modular Design:** Changes to one strategy don't affect others
- **Clear Ownership:** Each strategy has dedicated code and tests
- **Documentation:** Strategy-specific help and examples are co-located
- **Backwards Compatibility:** New strategies can be added without breaking changes

### **5.2. HTTP-Aware Strategy Benefits**

**Adaptive Intelligence:**
- **Server-Directed Timing:** Respects server load and rate limiting automatically
- **Optimal Performance:** Reduces unnecessary load on struggling servers
- **Real-World Tested:** Validated against major APIs in production use
- **Graceful Degradation:** Falls back to mathematical strategies when needed

**Production Ready:**
- **Error Handling:** Robust parsing with fallback on malformed responses
- **Performance:** Minimal overhead for HTTP response analysis
- **Compatibility:** Works with any HTTP tool (curl, wget, custom scripts)
- **Observability:** Clear logging of HTTP timing decisions

## **6\. Technology Choices**

* **Language:** Go. Ideal for creating efficient, self-contained, cross-platform CLI tools. Excellent concurrency model for handling subprocesses, timeouts, and HTTP parsing.
* **CLI Framework:** Cobra. Industry-standard library for building modern CLI applications with subcommand support.
* **HTTP Parsing:** Go standard library (`net/http`, `encoding/json`). No external dependencies for HTTP intelligence.
* **Configuration:** Custom TOML-based configuration with precedence handling (file → environment → flags).
* **Testing:** Go standard testing with testify for assertions. Comprehensive test coverage including real HTTP integration tests.
* **Metrics Exposition:** prometheus/client\_golang. Standard Go library for instrumenting applications with Prometheus metrics.