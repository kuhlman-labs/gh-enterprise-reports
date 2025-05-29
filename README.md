# gh-enterprise-reports

[![Build Status](https://github.com/kuhlman-labs/gh-enterprise-reports/actions/workflows/go.yml/badge.svg)](https://github.com/kuhlman-labs/gh-enterprise-reports/actions/workflows/go.yml)
![Go version](https://img.shields.io/badge/go-%3E=1.24-blue?logo=go)
![License: MIT](https://img.shields.io/badge/license-MIT-green)

A GitHub CLI extension to help administrators of a GitHub Enterprise Cloud environment run reports against their enterprise.

## 🚀 Quick Start

```bash
# Install the extension
gh extension install kuhlman-labs/gh-enterprise-reports

# Generate a configuration template
gh enterprise-reports init

# Edit the config.yml file with your settings
# Then run a report using your config
gh enterprise-reports --profile default
```

## Table of Contents
- [🚀 Quick Start](#-quick-start)
- [📋 Features](#-Features)
- [⚙️ Prerequisites](#-prerequisites)
- [🚀 Install manually](#-install-manually)
- [🚀 Install with GitHub CLI](#-install-with-github-cli)
- [🛠️ Usage](#-usage)
  - [🛠️ Initialization](#-initialization)
  - [🔧 Flags](#-flags)
- [🔄 Output Formats](#-output-formats)
- [📋 Configuration Profiles](#-configuration-profiles)
- [🛠️ Configuration Examples](#-configuration-examples)
- [🔐 GitHub App Authentication](#-github-app-authentication)
- [📊 Sample Output](#-sample-output)
- [📝 Logging](#-logging)
- [🛠️ Troubleshooting](#-troubleshooting)
- [🤔 Getting Help](#-getting-help)
- [🤝 Contributing](#-contributing)
- [📚 Acknowledgments](#-acknowledgments)
- [🔄 Workflow Examples](#-workflow-examples)
- [❓ Frequently Asked Questions](#-frequently-asked-questions)
- [📜 License](#-license)

---

## 📋 Features

- **Organizations Report**: Lists all organizations in the enterprise, their details, and memberships.
- **Repositories Report**: Provides information about repositories, including topics, teams, and custom properties.
- **Teams Report**: Details teams, their members, and external groups.
- **Collaborators Report**: Lists collaborators for repositories with their permissions.
- **Users Report**: Identifies users, their activity, and dormant status.
- **Active Repositories Report**: Identifies repositories with commits in the last 90 days and lists recent contributors.

---

## ⚙️ Prerequisites

Before using this tool, ensure you have the following:

- **Go**: Install Go (version 1.24 or later) from [golang.org](https://golang.org/).
- **GitHub Token**: A personal access token with the necessary permissions:
  - `read:org` for organization details.
  - `repo` for repository details.
  - `audit_log` for user login events.
  - `user` for user details.
  - `admin:enterprise` for enterprise details.
  
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

### 🛠️ Initialization

To get started with a configuration file, you can use the `init` command to generate a template:

```bash
gh enterprise-reports init
```

This will create a `config.yml` file in your current directory with documented options and example profiles. You can then customize this file with your own settings.

The generated template includes:
- Common configuration values (enterprise, token, output format, etc.)
- GitHub App authentication settings
- Example profile configurations for different use cases
- Detailed comments explaining each option

Example of generated template structure:
```yaml
# Common configuration values
enterprise: "fabrikam"           # Required: Your GitHub Enterprise slug
auth-method: "token"             # Authentication method: token or app
token: "your-token-here"         # Required if auth-method is token
output-format: "csv"             # Output format: csv, json, or xlsx
output-dir: "./reports"          # Directory to store report files

# Profile configurations
profiles:
  default:
    organizations: true
    repositories: true
    # ...more settings...
  
  # Additional profiles
  security-audit:
    # ...security audit settings...
```

You can specify a different output location using the `--output` flag:
```bash
gh enterprise-reports init --output custom-config.yml
```

### 🔧 Flags

| Flag                       | Description                                                                 |
|----------------------------|-----------------------------------------------------------------------------|
| Authentication Flags ||
| `--auth-method`            | Authentication method (`token` or `app`, defaults to `token`).             |
| `--token`                  | Personal access token (required if `auth-method` is `token`).              |
| `--app-id`                 | GitHub App ID (required if `auth-method` is `app`).                        |
| `--app-private-key-file`   | Path to the GitHub App private key file (required if `auth-method` is `app`). |
| `--app-installation-id`    | GitHub App installation ID (required if `auth-method` is `app`).           |
| Required Flags ||
| `--enterprise`             | Enterprise slug (required).                                                |
| Report Type Flags ||
| `--organizations`          | Generate the organizations report.                                         |
| `--repositories`           | Generate the repositories report.                                          |
| `--teams`                  | Generate the teams report.                                                 |
| `--collaborators`          | Generate the collaborators report.                                         |
| `--users`                  | Generate the users report.                                                 |
| `--active-repositories`    | Generate the active repositories report.                                   |
| Configuration Flags ||
| `--profile`               | Configuration profile to use (default: "default").                         |
| `--config-file`           | Path to config file (default is ./config.yml).                            |
| Output Flags ||
| `--output-format`         | Output format for reports (`csv`, `json`, or `xlsx`, default `csv`).      |
| `--output-dir`            | Directory where report files will be saved.                               |
| Performance & Debug Flags ||
| `--log-level`             | Set log level (`debug`, `info`, `warn`, `error`, `fatal`, `panic`).       |
| `--workers`               | Number of concurrent workers for fetching data (default 5).                |

**notes:** 
The `--auth-method` flag is required is only required if you are using a GitHub App. GitHub App support is experimental at this time and may not work as expected.

You can que multiple reports in a single command. For example:
```bash
gh enterprise-reports --token <your-token> --enterprise <enterprise-slug> --organizations --repositories
```
This will generate both the organizations and repositories reports.

### Worker Count Explanation

The `--workers` flag controls the number of concurrent workers used to fetch data for the selected reports. Increasing the worker count can speed up report generation, especially for large enterprises, by making more API requests in parallel.

**When to adjust:**

- **Increase workers:** If reports are running slowly and you are not hitting GitHub API rate limits.
- **Decrease workers:** If you are frequently encountering rate limit errors (HTTP 429 or 403). Reducing the number of workers will slow down the report but make it less likely to hit rate limits.

The optimal number depends on your enterprise size, network conditions, and GitHub API rate limits. Start with the default (5) and adjust as needed.


## 🔄 Output Formats

gh-enterprise-reports supports multiple output formats:

- **CSV** (default): Standard comma-separated values format compatible with spreadsheet software
- **JSON**: Structured data format ideal for programmatic processing
- **Excel (XLSX)**: Feature-rich spreadsheet format with styling support

Specify your preferred format using the `--output-format` flag:

```bash
gh enterprise-reports --enterprise <enterprise-slug> --organizations --output-format xlsx
```

## 📋 Configuration Profiles

You can create configuration profiles to easily run different sets of reports with different settings:

1. Create a `config.yml` file with your profiles (you can use `gh enterprise-reports init` to get started):

```yaml
# Common settings
enterprise: "your-enterprise"
token: "your-token"
output_format: "csv"
workers: 5

# Profile configurations
profiles:
  default:
    organizations: true
    repositories: true
    output_dir: "reports/default"
    
  security-audit:
    organizations: true
    teams: true
    collaborators: true
    output_format: "xlsx"
    output_dir: "reports/security"
    
  monthly-review:
    organizations: true
    repositories: true
    users: true
    output_format: "json"
    output_dir: "reports/monthly"
    workers: 10

  inactive-users:
    users: true
    output_dir: "reports/users"
    log_level: "debug"
```

2. Run reports using your profiles:

```bash
# Use the default profile
gh enterprise-reports --profile default

# Run a security audit
gh enterprise-reports --profile security-audit

# Generate monthly review reports
gh enterprise-reports --profile monthly-review

# Check for inactive users
gh enterprise-reports --profile inactive-users
```

3. Override profile settings with command-line flags:

```bash
# Use security-audit profile but change output format
gh enterprise-reports --profile security-audit --output-format csv

# Use monthly-review profile with different worker count
gh enterprise-reports --profile monthly-review --workers 15
```

The command-line flags take precedence over the profile settings, allowing you to easily customize the execution without modifying the config file.

## 🛠️ Configuration Examples

Here are some annotated examples of the `config.yml` file:

<details>
<summary>Basic configuration</summary>

```yaml
# Minimal configuration example
enterprise: "your-enterprise" # Your GitHub Enterprise name
token: "ghp_xxxxxxxxxxxx"     # Personal access token
output-format: "csv"          # Output in CSV format
organizations: true           # Only run the organizations report
```
</details>

<details>
<summary>Advanced configuration with profiles</summary>

```yaml
# Common settings applied to all profiles
enterprise: "your-enterprise"
token: "ghp_xxxxxxxxxxxx"
output-format: "csv"
workers: 5
log-level: "info"

# Profile configurations for different use cases
profiles:
  # Default profile with standard settings
  default:
    organizations: true
    repositories: true
    output-dir: "./reports/default"
    
  # Comprehensive audit with all reports
  full-audit:
    organizations: true
    repositories: true
    teams: true
    collaborators: true
    users: true
    output-format: "xlsx"     # Excel format for better analysis
    output-dir: "./reports/audit"
    workers: 10               # More workers for faster processing
    
  # GitHub App authentication example
  app-auth:
    auth-method: "app"
    app-id: "12345"
    app-private-key-file: "./private-key.pem"
    app-installation-id: "67890"
    organizations: true
    repositories: true
```
</details>

---

## 🔐 GitHub App Authentication

This tool supports GitHub App authentication as an alternative to personal access tokens, offering advantages like higher rate limits and more granular permissions.

<details>
<summary>Benefits of GitHub App Authentication</summary>

- **Higher Rate Limits**: GitHub Apps have higher API rate limits than personal access tokens
- **Fine-grained Permissions**: You can specify exactly what permissions the app needs
- **No User Association**: The app is not tied to a specific user account
- **Automatic Token Refresh**: Tokens are automatically refreshed and rotated
- **Enhanced Security**: No need to store long-lived personal access tokens
</details>

<details>
<summary>Quick Setup Example</summary>

```yaml
# Using GitHub App authentication in config.yml
enterprise: "your-enterprise"
auth-method: "app"
app-id: 12345
app-private-key-file: "./private-key.pem"
app-installation-id: 67890
organizations: true
```

```bash
# Using GitHub App authentication via command line
gh enterprise-reports \
  --auth-method app \
  --app-id 12345 \
  --app-private-key-file ./private-key.pem \
  --app-installation-id 67890 \
  --enterprise your-enterprise \
  --organizations
```
</details>

---

## 📊 Sample Output

<details>
<summary>Organizations Report</summary>

**Command:**
```bash
gh enterprise-reports --organizations --token <your-token> --enterprise <enterprise-slug>
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
gh enterprise-reports --repositories --token <your-token> --enterprise <enterprise-slug>
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
gh enterprise-reports --teams --token <your-token> --enterprise <enterprise-slug>
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
gh enterprise-reports --collaborators --token <your-token> --enterprise <enterprise-slug>
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
gh enterprise-reports --users --token <your-token> --enterprise <enterprise-slug>
```

**Sample Output:**
```csv
ID,Login,Name,Email,Last Login(90 days),Dormant?
1,user1,User One,user1@example.com,2023-01-01T00:00:00Z,false
...
```
</details>

<details>
<summary>Active Repositories Report</summary>

**Command:**
```bash
gh enterprise-reports --active-repositories --token <your-token> --enterprise <enterprise-slug>
```

**Sample Output:**
```csv
Owner,Repository,Pushed_At,Recent_Contributors
org1,active-repo-1,2023-10-15T14:30:00Z,john.doe; jane.smith; alice.dev
org2,busy-project,2023-10-20T09:15:00Z,bob.coder; charlie.dev
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

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to contribute to this project.

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

## 🔄 Workflow Examples

Here are some common workflows that showcase how to use this tool effectively in your enterprise environment:

<div class="workflow-example">
<h3>📊 Monthly Enterprise Audit</h3>

<pre>
┌─────────────────┐    ┌──────────────────┐    ┌──────────────────────┐
│ Generate Reports│    │ Export to Excel  │    │ Share with Leadership│
│ with Profile    │ ──▶│ for Analysis     │ ──▶│ or Security Team     │
└─────────────────┘    └──────────────────┘    └──────────────────────┘
</pre>

<p><strong>Use Case:</strong> Regular enterprise-wide review of organization health, repository metrics, and user activity.</p>

<pre class="command">
# Run a comprehensive audit using the monthly-review profile
gh enterprise-reports --profile monthly-review
</pre>
</div>

<div class="workflow-example">
<h3>👤 Identify Inactive Users</h3>

<pre>
┌────────────────┐    ┌────────────────┐    ┌────────────────────┐
│ Generate Users │    │ Filter         │    │ Take Action on     │
│ Report         │ ──▶│ Dormant Users  │ ──▶│ Inactive Accounts  │
└────────────────┘    └────────────────┘    └────────────────────┘
</pre>

<p><strong>Use Case:</strong> Identify dormant users to optimize license usage and improve security posture.</p>

<pre class="command">
# Run the users report to identify dormant accounts
gh enterprise-reports --profile inactive-users
</pre>
</div>

<div class="workflow-example">
<h3>🔒 Security Assessment Pipeline</h3>

<pre>
┌─────────────────┐    ┌────────────────┐    ┌──────────────────┐    ┌──────────────────┐
│ Generate Reports│    │ Process JSON   │    ┌──────────────────┐    │ Generate Security│
│ in JSON Format  │ ──▶│ with Scripts   │ ──▶│ Compare Against  │ ──▶│ Compliance Report│
└─────────────────┘    └────────────────┘    └──────────────────┘    └──────────────────┘
</pre>

<p><strong>Use Case:</strong> Integrate report data into security tools and compliance workflows.</p>

<pre class="command">
# Generate JSON data for further automated processing
gh enterprise-reports --profile security-audit --output-format json
</pre>
</div>



## ❓ Frequently Asked Questions

<details>
<summary>How do I handle rate limiting?</summary>

The tool automatically handles rate limiting by pausing when limits are reached. You can:
1. Reduce the `--workers` count to make fewer concurrent API calls
2. Switch to GitHub App authentication which has higher rate limits
3. Run during off-hours when there's less API traffic
</details>

<details>
<summary>What permissions does my token need?</summary>

Your token needs these permissions:
- `read:org` for organization details
- `repo` for repository details
- `audit_log` for user login events
- `user` for user details
- `read:enterprise` for enterprise details

For GitHub App authentication, configure the same permission scopes.
</details>

<details>
<summary>Can I automate report generation?</summary>

Yes! You can use GitHub Actions or any scheduler to run reports periodically:

```yaml
# Example GitHub Actions workflow for weekly enterprise reporting
name: Weekly Enterprise Report

# Run every Monday at midnight
on:
  schedule:
    - cron: '0 0 * * MON'

jobs:
  generate-reports:
    name: Generate Enterprise Reports
    runs-on: ubuntu-latest
    
    steps:
      # Check out the repository
      - name: Checkout repository
        uses: actions/checkout@v3
      
      # Install the gh-enterprise-reports extension
      - name: Install Extension
        run: gh extension install kuhlman-labs/gh-enterprise-reports
        env:
          # GitHub CLI is preinstalled on all GitHub-hosted runners
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      
      # Generate the enterprise reports    
      - name: Generate Reports
        run: |
          gh enterprise-reports \
            --token ${{ secrets.ENTERPRISE_TOKEN }} \
            --enterprise your-enterprise \
            --organizations \
            --repositories
      
      # Upload reports as artifacts    
      - name: Upload Reports
        uses: actions/upload-artifact@v3
        with:
          name: enterprise-reports
          path: ./reports/
```
</details>

<details>
<summary>How large are the generated reports?</summary>

Report sizes vary based on your enterprise size:
- Small enterprises (< 50 orgs): Usually < 10MB
- Medium enterprises (50-200 orgs): 10-50MB
- Large enterprises (> 200 orgs): Can exceed 100MB

Consider the Excel format for larger reports as it compresses the data better.
</details>
