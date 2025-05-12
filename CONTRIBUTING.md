# Contributing to GitHub Enterprise Reports

Thank you for your interest in contributing to GitHub Enterprise Reports! This document provides guidelines and instructions for contributing to this project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
  - [Project Setup](#project-setup)
  - [Development Workflow](#development-workflow)
- [How to Contribute](#how-to-contribute)
  - [Reporting Issues](#reporting-issues)
  - [Feature Requests](#feature-requests)
  - [Pull Requests](#pull-requests)
- [Project Structure](#project-structure)
  - [Package Overview](#package-overview)
- [Coding Standards](#coding-standards)
  - [Go Conventions](#go-conventions)
  - [Commenting Style](#commenting-style)
  - [Testing](#testing)
- [Review Process](#review-process)
- [Release Process](#release-process)

## Code of Conduct

This project adheres to a code of conduct. By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

## Getting Started

### Project Setup

1. Fork the repository
2. Clone your fork: `git clone https://github.com/your-username/gh-enterprise-reports.git`
3. Change to the project directory: `cd gh-enterprise-reports`
4. Add the upstream repository as a remote: `git remote add upstream https://github.com/kuhlman-labs/gh-enterprise-reports.git`
5. Install dependencies: `go mod download`

### Development Workflow

1. Create a new branch for your feature or bugfix: `git checkout -b feature/your-feature-name`
2. Make your changes
3. Run tests: `go test ./...`
4. Commit your changes: `git commit -am "Add some feature"`
5. Push to your fork: `git push origin feature/your-feature-name`
6. Create a pull request

## How to Contribute

### Reporting Issues

When reporting issues, please include:

- A clear and descriptive title
- A detailed description of the issue
- Steps to reproduce the behavior
- Expected behavior
- Screenshots or logs (if applicable)
- Environment details (OS, Go version, etc.)

### Feature Requests

Feature requests are welcome. Please provide:

- A clear and detailed description of the proposed feature
- The motivation behind the feature
- Example use cases

### Pull Requests

For pull requests:

1. Update documentation as needed
2. Add tests for new features
3. Ensure all tests pass
4. Update the README.md if necessary
5. Follow the coding standards (see below)

## Project Structure

GitHub Enterprise Reports is organized into a modular structure to maintain separation of concerns and facilitate testing. The main entry point is `main.go`, which initializes the application and delegates to the `cmd` package for command-line interface handling.

### Package Overview

The project is structured into the following packages:

- **cmd/**: Contains command-line interface handling logic
  - `root.go`: Implements the root command and base CLI functionality
  - `init.go`: Implements the init command for creating configuration files
  - `logging.go`: Manages application logging setup

- **enterprise-reports/**: Contains the core application packages
  - **api/**: Provides functionality for interacting with GitHub's REST and GraphQL APIs
    - `graphql.go`: Handles GraphQL API operations
    - `rest.go`: Handles REST API operations
    - `rate-limits.go`: Manages API rate limiting
    - `retry.go`: Implements retry logic for API calls

  - **config/**: Manages configuration for the application
    - `config.go`: Defines the configuration structure
    - `provider.go`: Interface for configuration providers
    - `standard_provider.go`: Standard implementation of the configuration provider
    - `manager_provider.go`: Manages multiple configuration providers

  - **logging/**: Provides logging functionality
    - `logging.go`: Configures and manages structured logging

  - **report/**: Core report generation functionality
    - `executor.go`: Executes report generation workflows

  - **reports/**: Implements specific report types
    - `collaborators.go`: Generates reports on repository collaborators
    - `organizations.go`: Generates reports on enterprise organizations
    - `repositories.go`: Generates reports on repositories
    - `teams.go`: Generates reports on teams
    - `users.go`: Generates reports on users
    - `formats.go`: Handles output formatting
    - `runner.go`: Orchestrates the execution of multiple reports

  - **utils/**: Utility functions and helpers
    - `cache.go`: Provides caching mechanisms for API data
    - `concurrent.go`: Utilities for concurrent operations
    - `errors.go`: Error handling utilities
    - `file.go`: File system operations
    - `github.go`: GitHub-specific utility functions

- **utils/**: Utility functions and helpers
  - `cache.go`: Provides caching mechanisms for API data
  - `concurrent.go`: Utilities for concurrent operations
  - `errors.go`: Error handling utilities
  - `file.go`: File system operations
  - `github.go`: GitHub-specific utility functions

Each package contains appropriate test files with the `_test.go` suffix to ensure functionality is correctly implemented and maintained.

## Coding Standards

### Go Conventions

- Follow the [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Use `gofmt` or `goimports` to format your code
- Follow [Effective Go](https://golang.org/doc/effective_go) guidelines

### Commenting Style

- All exported functions, types, and packages must have comments
- Package comments should appear immediately before the package clause:
  ```go
  // Package name provides a description of the package.
  package name
  ```
- Function and type comments should begin with the name being described:
  ```go
  // FunctionName does something specific.
  func FunctionName() {}
  
  // TypeName represents something.
  type TypeName struct {}
  ```
- Comments should be complete sentences, starting with a capital letter and ending with a period
- Use proper grammar and spelling

### Testing

- Write unit tests for all new functionality
- Ensure that existing tests pass
- Aim for high test coverage of all critical paths
- Use table-driven tests when appropriate

## Review Process

All submissions require review. We use GitHub pull requests for this purpose.

1. Submit a pull request with a clear description of the changes
2. Address any feedback or requested changes
3. Once approved, a maintainer will merge your changes

## Release Process

This project uses semantic versioning. The process for releasing a new version is:

1. Update the version number in relevant files
2. Update the CHANGELOG.md file
3. Create a tag for the new version
4. Push the tag to GitHub
5. GitHub Actions will build and publish the release

Thank you for contributing!
