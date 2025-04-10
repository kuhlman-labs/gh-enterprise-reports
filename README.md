# gh-enterprise-reports

`gh-enterprise-reports` is a GitHub CLI extension designed to generate detailed reports for GitHub Enterprise Cloud EMU environments. It supports various types of reports, including organizations, repositories, teams, collaborators, and users.

## Features

- **Organizations Report**: Lists all organizations in the enterprise, their details, and memberships.
- **Repositories Report**: Provides information about repositories, including topics, teams, and custom properties.
- **Teams Report**: Details teams, their members, and external groups.
- **Collaborators Report**: Lists collaborators for repositories with their permissions.
- **Users Report**: Identifies users, their activity, and dormant status.

## Prerequisites

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

## Installation

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

## Usage

Run the CLI with the desired flags to generate reports. For example:
```bash
./gh-enterprise-reports --auth-method token --token <your-token> --enterprise <enterprise-slug> --organizations
```

### Flags

- `--auth-method`: Authentication method (`token` or `app` Defaults to `token`).
- `--token`: Personal access token (required if `auth-method` is `token`).
- `--app-id`: GitHub App ID (required if `auth-method` is `app`).
- `--app-private-key-file`: Path to the GitHub App private key file (required if `auth-method` is `app`).
- `--app-installation-id`: GitHub App installation ID (required if `auth-method` is `app`).
- `--enterprise`: Enterprise slug (required).
- `--organizations`: Generate the organizations report.
- `--repositories`: Generate the repositories report.
- `--teams`: Generate the teams report.
- `--collaborators`: Generate the collaborators report.
- `--users`: Generate the users report.
- `--log-level`: Set log level (`debug`, `info`, `warn`, `error`, `fatal`, `panic`).

## Sample Output

### Organizations Report
Command:
```bash
./gh-enterprise-reports --auth-method token --token <your-token> --enterprise <enterprise-slug> --organizations
```
Sample Output:
```csv
Organization,Organization ID,Organization Default Repository Permission,Members,Total Members
org1,MDQ6VXNlcjE=,read,{user1,MDQ6VXNlcjE=,John Doe,admin},{user2,MDQ6VXNlcjI=,Jane Smith,member},2
```

### Repositories Report
Command:
```bash
./gh-enterprise-reports --auth-method token --token <your-token> --enterprise <enterprise-slug> --repositories
```
Sample Output:
```csv
owner,repository,archived,visibility,pushed_at,created_at,topics,custom_properties,teams
org1,repo1,false,public,2023-01-01T12:00:00Z,2022-01-01T12:00:00Z,[topic1,topic2],{prop1:value1},{Team Name: team1, TeamID: 1, Team Slug: team1, External Group: [group1], Permission: admin}
```

### Teams Report
Command:
```bash
./gh-enterprise-reports --auth-method token --token <your-token> --enterprise <enterprise-slug> --teams
```
Sample Output:
```csv
Team ID,Organization,Team Name,Team Slug,External Group,Members
1,org1,team1,team1,[group1,user-group1],[user1,user2]
```

### Collaborators Report
Command:
```bash
./gh-enterprise-reports --auth-method token --token <your-token> --enterprise <enterprise-slug> --collaborators
```
Sample Output:
```csv
Repository,Collaborators
org1/repo1,{Login: user1, ID: 123, Permission: admin},{Login: user2, ID: 456, Permission: push}
```

### Users Report
Command:
```bash
./gh-enterprise-reports --auth-method token --token <your-token> --enterprise <enterprise-slug> --users
```
Sample Output:
```csv
ID,Login,Name,Email,Last Login,Dormant?
123,user1,John Doe,john.doe@example.com,2023-01-01T12:00:00Z,No
456,user2,Jane Smith,jane.smith@example.com,N/A,Yes
```

## Logging

Logs are written to `gh-enterprise-reports.log` in the current directory. You can adjust the log level using the `--log-level` flag.

## Troubleshooting

- **Authentication Errors**: Ensure your token or GitHub App credentials are correct and have the necessary permissions.
- **Rate Limit Exceeded**: The tool automatically waits for rate limits to reset. If this occurs frequently, consider increasing using a GitHub App.

## Contributing

We welcome contributions! To contribute:

1. Fork the repository.
2. Create a new branch for your feature or bug fix.
3. Submit a pull request with a detailed description of your changes.

## Acknowledgments

This project uses the following libraries:

- [go-github](https://github.com/google/go-github): GitHub REST API client for Go.
- [shurkool](github.com/shurcooL/githubv4): GitHub GraphQL API client for Go.
- [cobra](https://github.com/spf13/cobra): CLI framework for Go.
- [viper](https://github.com/spf13/viper): Configuration management for Go.
- [zerolog](https://github.com/rs/zerolog): High-performance logging for Go.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
