# gh-enterprise-reports

`gh-enterprise-reports` is a GitHub CLI extension designed to generate detailed reports for GitHub Enterprise Cloud EMU environments. It supports various types of reports, including:

- **Organizations**
- **Repositories**
- **Teams**
- **Collaborators**
- **Users**

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
- **GitHub App (optional)**: If using GitHub App authentication, ensure you have:
  - App ID
  - Private key file
  - Installation ID

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

## 🚀 Install with Github CLI

1. Install the GitHub CLI from [cli.github.com](https://cli.github.com/).

2. Install the extension:
   ```bash
   gh extension install kuhlman-labs/gh-enterprise-reports
   ```

---

## 🛠️ Usage

Run the CLI with the desired flags to generate reports. For example:
```bash
./gh-enterprise-reports --auth-method token --token <your-token> --enterprise <enterprise-slug> --organizations
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

---

## 📊 Sample Output

### Organizations Report
**Command:**
```bash
./gh-enterprise-reports --auth-method token --token <your-token> --enterprise <enterprise-slug> --organizations
```
or with GitHub CLI:
```bash
gh enterprise reports organizations --auth-method token --token <your-token> --enterprise <enterprise-slug> --organizations
```

**Sample Output:**
```csv
Organization,Organization ID,Organization Default Repository Permission,Members,Total Members
org1,123456,read,"[{""login"":""user1"",""id"":1,""name"":""User One"",""roleName"":""admin""}]",1
```

### Repositories Report
**Command:**
```bash
./gh-enterprise-reports --auth-method token --token <your-token> --enterprise <enterprise-slug> --repositories
```
or with GitHub CLI:
```bash
gh enterprise reports repositories --auth-method token --token <your-token> --enterprise <enterprise-slug> --repositories
```

**Sample Output:**
```csv
Owner,Repository,Archived,Visibility,Pushed_At,Created_At,Topics,Custom_Properties,Teams
org1,repo1,false,public,2023-01-01T00:00:00Z,2022-01-01T00:00:00Z,[topic1],{key:value},team1
```

### Teams Report
**Command:**
```bash
./gh-enterprise-reports --auth-method token --token <your-token> --enterprise <enterprise-slug> --teams
```

or with GitHub CLI:
```bash
gh enterprise reports teams --auth-method token --token <your-token> --enterprise <enterprise-slug> --teams
```

**Sample Output:**
```csv
Team ID,Owner,Team Name,Team Slug,External Group,Members
1,org1,team1,team1,[group1],[user1,user2]
```

### Collaborators Report
**Command:**
```bash
./gh-enterprise-reports --auth-method token --token <your-token> --enterprise <enterprise-slug> --collaborators
```
or with GitHub CLI:
```bash
gh enterprise reports collaborators --auth-method token --token <your-token> --enterprise <enterprise-slug> --collaborators
```

**Sample Output:**
```csv
Repository,Collaborators
org1/repo1,{login:user1,id:1,permission:admin}
```

### Users Report
**Command:**
```bash
./gh-enterprise-reports --auth-method token --token <your-token> --enterprise <enterprise-slug> --users
```
or with GitHub CLI:
```bash
gh enterprise reports users --auth-method token --token <your-token> --enterprise <enterprise-slug> --users
```

**Sample Output:**
```csv
ID,Login,Name,Email,Last Login(90 days),Dormant?
1,user1,User One,user1@example.com,2023-01-01T00:00:00Z,false
```

---

## 📝 Logging

Logs are written to `gh-enterprise-reports.log` in the current directory. You can adjust the log level using the `--log-level` flag.

---

## 🛠️ Troubleshooting

- **Authentication Errors**: Ensure your token or GitHub App credentials are correct and have the necessary permissions.
- **Rate Limit Exceeded**: The tool automatically waits for rate limits to reset. If this occurs frequently, consider using a GitHub App.

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
