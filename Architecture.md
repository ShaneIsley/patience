# **Architecture: patience CLI**

This document outlines the architecture for a modern, robust patience command-line utility, written in Go.

## **1\. Core Philosophy**

The patience CLI is designed to be:

* **Reliable:** It should be a trustworthy tool for wrapping and retrying critical commands.  
* **Observant:** It must provide clear, actionable feedback and metrics.  
* **Flexible:** It should support a wide range of use cases, from simple scripts to complex CI/CD pipelines.  
* **Modern:** It will leverage modern CLI and system capabilities.  
* **Self-contained:** The core functionality must work flawlessly without any external dependencies or daemons. The daemon is an enhancement, not a requirement.

## **2\. High-Level Components**

The system consists of two main components:

1. **patience CLI:** The primary executable that users interact with. It wraps a given command and implements the retry logic.  
2. **patienced Daemon (Optional):** A background process that collects, aggregates, and exposes long-term metrics. It is not required for the patience command to function.

## **3\. patience CLI \- Detailed Architecture**

The CLI is the core of the project and is responsible for all command execution and retry logic.

### **3.1. Execution Flow**

1. **Parsing & Configuration:** On launch, the CLI parses all command-line arguments, flags, and potentially a configuration file (\~/.config/retry/config.toml). This configuration object dictates the entire behavior of the run.  
2. **Executor Initialization:** An Executor is created based on the configuration. The executor is responsible for managing the command lifecycle.  
3. **Attempt Loop:** The executor enters a loop for each attempt.  
   * It spawns the wrapped command as a subprocess.  
   * It captures stdout, stderr, and the exit code.  
   * It monitors the command against configured timeouts.  
4. **Condition Checking:** After each attempt, the executor checks the result against the success/failure conditions.  
   * **Success Conditions:** (e.g., exit code 0, stdout matches regex). If met, the loop terminates.  
   * **Failure Conditions:** (e.g., non-zero exit code, timeout, stderr matches regex). If met, it calculates the delay for the next attempt.  
5. **Delay Strategy:** If a retry is needed, the executor uses the configured backoff strategy (fixed, exponential, jitter, linear, decorrelated-jitter, or fibonacci) to calculate and wait for the appropriate delay.  
6. **Termination:** The loop terminates when a success condition is met or the maximum number of attempts is reached.  
7. **Status Reporting:** A final summary of the execution is printed to the console.  
8. **Metrics Dispatch (Async):** If the patienced daemon is active and configured, the CLI asynchronously sends the final run metrics to the daemon via a Unix socket. This is a non-blocking, "fire-and-forget" operation to ensure the CLI's exit is not delayed.

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

### **3.3. Go Package Structure**

/retry  
├── /cmd  
│   └── /retry      \# Main package, CLI parsing (Cobra)  
├── /pkg  
│   ├── /executor   \# Core logic for running and managing commands  
│   ├── /config     \# Configuration loading and validation  
│   ├── /backoff    \# Backoff strategies (fixed, exponential, jitter, linear, decorrelated-jitter, fibonacci)  
│   ├── /conditions \# Logic for checking success/failure conditions  
│   ├── /metrics    \# Structs and client for sending data to patienced  
│   └── /ui         \# Handles terminal output and status reporting  
└── /internal  
    └── /daemon     \# (If patienced is in the same repo) Daemon-specific logic

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

## **5\. Technology Choices**

* **Language:** Go. Ideal for creating efficient, self-contained, cross-platform CLI tools. Its concurrency model is perfect for handling subprocesses and timeouts.  
* **CLI Framework:** cobra. A robust library for building modern CLI applications in Go.  
* **Configuration:** viper. To handle configuration from files, environment variables, and flags.  
* **Metrics Exposition:** prometheus/client\_golang. The standard Go library for instrumenting applications with Prometheus metrics.