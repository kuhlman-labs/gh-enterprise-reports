# gh-enterprise-reports

[![Build Status](https://github.com/kuhlman-labs/gh-enterprise-reports/actions/workflows/go.yml/badge.svg)](https://github.com/kuhlman-labs/gh-enterprise-reports/actions/workflows/go.yml)
![Go version](https://img.shields.io/badge/go-%3E=1.24-blue?logo=go)
![License: MIT](https://img.shields.io/badge/license-MIT-green)

A GitHub CLI extension to help administrators of a GitHub Enterprise Cloud environment run reports against their enterprise.

## Table of Contents
- [Features](#%F0%9F%93%8B-features)
- [Prerequisites](#%E2%9A%99%EF%B8%8F-prerequisites)
- [Install manually](#%F0%9F%9A%80-install-manually)
- [Install with GitHub CLI](#%F0%9F%9A%80-install-with-github-cli)
- [Usage](#%F0%9F%9A%99%EF%B8%8F-usage)
  - [Flags](#-flags)
  - [Sample Output](#%F0%9F%93%8A-sample-output)
- [Getting Help](#-%E2%9F%99%CB%86-getting-help)
- [Contributing](#%E2%9D%A4%EF%B8%8F-contributing)
- [Acknowledgments](#%F0%9F%93%9A-acknowledgments)
- [License](#%F0%9F%93%9C-license)

---

## 📋 Features

- **Organizations Report**: Lists all organizations in the enterprise, their details, and memberships.
- **Repositories Report**: Provides information about repositories, including topics, teams, and custom properties.
- **Teams Report**: Details teams, their members, and external groups.
- **Collaborators Report**: Lists collaborators for repositories with their permissions.
- **Users Report**: Identifies users, their activity, and dormant status.

---

## ⚙️ Prerequisites

Before using this tool, ensure you have the following:

- **Go**: Install Go (version 1.24 or later) from [golang.org](https://golang.org/).
- **GitHub Token**: A personal access token with the necessary permissions:
  - `read:org` for organization details.
  - `repo` for repository details.
  - `audit_log` for user login events.
  - `user` for user details.
  - `read:enterprise` for enterprise details.
  
---

## 🚀 Install manually

1. Clone the repository:
   ```bash
   git clone https://github.com/kuhlman-labs/gh-enterprise-reports.git
   cd gh-enterprise-reports
   ```

2. Build the CLI:
   ```bash
   go build -o gh-enterprise-reports main.go
   ```

3. Add the binary to your PATH or run it directly.

## 🚀 Install with GitHub CLI

1. Install the GitHub CLI from [cli.github.com](https://cli.github.com/).

2. Install the extension:
   ```bash
   gh extension install kuhlman-labs/gh-enterprise-reports
   ```

---

## 🛠️ Usage

Run the CLI with the desired flags to generate reports. For example:
```bash
gh enterprise-reports --token <your-token> --enterprise <enterprise-slug> --organizations
```

### 🔧 Flags

 | Flag                       | Description                                                                 |
 |----------------------------|-----------------------------------------------------------------------------|
 | `--auth-method`            | Authentication method (`token` or `app`, defaults to `token`).             |
 | `--token`                  | Personal access token (required if `auth-method` is `token`).              |
 | `--app-id`                 | GitHub App ID (required if `auth-method` is `app`).                        |
 | `--app-private-key-file`   | Path to the GitHub App private key file (required if `auth-method` is `app`). |
 | `--app-installation-id`    | GitHub App installation ID (required if `auth-method` is `app`).           |
 | `--enterprise`             | Enterprise slug (required).                                                |
 | `--organizations`          | Generate the organizations report.                                         |
 | `--repositories`           | Generate the repositories report.                                          |
 | `--teams`                  | Generate the teams report.                                                 |
 | `--collaborators`          | Generate the collaborators report.                                         |
 | `--users`                  | Generate the users report.                                                 |
 | `--log-level`              | Set log level (`debug`, `info`, `warn`, `error`, `fatal`, `panic`).         |
 | `--workers`                | Number of concurrent workers for fetching data (default 5).                |

**note:** the `--auth-method` flag is required is not required if you are using a token. GitHub App support is experimental and may not work as expected.

### Worker Count Explanation

The `--workers` flag controls the number of concurrent workers used to fetch data for the selected reports. Increasing the worker count can speed up report generation, especially for large enterprises, by making more API requests in parallel.

**When to adjust:**

- **Increase workers:** If reports are running slowly and you are not hitting GitHub API rate limits.
- **Decrease workers:** If you are frequently encountering rate limit errors (HTTP 429 or 403). Reducing the number of workers will slow down the report but make it less likely to hit rate limits.

The optimal number depends on your enterprise size, network conditions, and GitHub API rate limits. Start with the default (5) and adjust as needed.

---

## 📊 Sample Output

<details>
<summary>Organizations Report</summary>

**Command:**
```bash
gh enterprise-reports organizations --token <your-token> --enterprise <enterprise-slug>
```

**Sample Output:**
```csv
Organization,Organization ID,Organization Default Repository Permission,Members,Total Members
org1,123456,read,"[{""login"":""user1"",""id"":1,""name"":""User One"",""roleName"":""admin""}]",1
...
```
</details>

<details>
<summary>Repositories Report</summary>

**Command:**
```bash
gh enterprise-reports repositories --token <your-token> --enterprise <enterprise-slug>
```

**Sample Output:**
```csv
Owner,Repository,Archived,Visibility,Pushed_At,Created_At,Topics,Custom_Properties,Teams
org1,repo1,false,public,2023-01-01T00:00:00Z,2022-01-01T00:00:00Z,[topic1],{key:value},team1
...
```
</details>

<details>
<summary>Teams Report</summary>

**Command:**
```bash
gh enterprise-reports teams --token <your-token> --enterprise <enterprise-slug>
```

**Sample Output:**
```csv
Team ID,Owner,Team Name,Team Slug,External Group,Members
1,org1,team1,team1,[group1],[user1,user2]
...
```
</details>

<details>
<summary>Collaborators Report</summary>

**Command:**
```bash
gh enterprise-reports collaborators --token <your-token> --enterprise <enterprise-slug>
```

**Sample Output:**
```csv
Repository,Collaborators
org1/repo1,{login:user1,id:1,permission:admin}
...
```
</details>

<details>
<summary>Users Report</summary>

**Command:**
```bash
gh enterprise-reports users --token <your-token> --enterprise <enterprise-slug>
```

**Sample Output:**
```csv
ID,Login,Name,Email,Last Login(90 days),Dormant?
1,user1,User One,user1@example.com,2023-01-01T00:00:00Z,false
...
```
</details>

---

## 📝 Logging

Logs are written to `gh-enterprise-reports.log` in the current directory. You can adjust the log level using the `--log-level` flag.

---

## 🛠️ Troubleshooting

- **Authentication Errors**: Ensure your token or GitHub App credentials are correct and have the necessary permissions.
- **Rate Limit Exceeded**: The tool automatically waits for rate limits to reset. If this occurs frequently, consider using a GitHub App.

---

## 🤔 Getting Help

If you encounter issues or have questions, please open an issue:
https://github.com/kuhlman-labs/gh-enterprise-reports/issues

---

## 🤝 Contributing

We welcome contributions! To contribute:

1. Fork the repository.
2. Create a new branch for your feature or bug fix.
3. Submit a pull request with a detailed description of your changes.

---

## 📚 Acknowledgments

This project uses the following libraries:

- [go-github](https://github.com/google/go-github): GitHub REST API client for Go.
- [shurcooL/githubv4](https://github.com/shurcooL/githubv4): GitHub GraphQL API client for Go.
- [cobra](https://github.com/spf13/cobra): CLI framework for Go.
- [viper](https://github.com/spf13/viper): Configuration management for Go.

---

## 📜 License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
