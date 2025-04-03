package enterprisereports

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/gofri/go-github-pagination/githubpagination"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_primary_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_secondary_ratelimit"
	"github.com/google/go-github/v70/github"
)

// Config encapsulates all configuration from CLI flags and Viper.
type Config struct {
	Organizations           bool
	Repositories            bool
	Teams                   bool
	Collaborators           bool
	Users                   bool
	AuthMethod              string
	Token                   string
	GithubAppID             int64
	GithubAppPrivateKey     string
	GithubAppInstallationID int64
	EnterpriseSlug          string
}

// Validate checks for required flags based on the chosen authentication method.
func (c *Config) Validate() error {
	if c.EnterpriseSlug == "" {
		return fmt.Errorf("enterprise flag is required")
	}

	switch c.AuthMethod {
	case "token":
		if c.Token == "" {
			return fmt.Errorf("token is required when using token authentication")
		}
	case "app":
		if c.GithubAppID == 0 || c.GithubAppPrivateKey == "" || c.GithubAppInstallationID == 0 {
			return fmt.Errorf("app-id, app-private-key, and app-installation-id are required when using GitHub App authentication")
		}
	default:
		return fmt.Errorf("unknown auth-method %q; please use 'token' or 'app'", c.AuthMethod)
	}
	return nil
}

// InitializeFlags configures the CLI flags and binds them to Viper.
func InitializeFlags(rootCmd *cobra.Command, config *Config) {
	// Report flags.
	rootCmd.Flags().BoolVar(&config.Organizations, "organizations", false, "Run Organizations report")
	rootCmd.Flags().BoolVar(&config.Repositories, "repositories", false, "Run Repositories report")
	rootCmd.Flags().BoolVar(&config.Teams, "teams", false, "Run Teams report")
	rootCmd.Flags().BoolVar(&config.Collaborators, "collaborators", false, "Run Collaborators report")
	rootCmd.Flags().BoolVar(&config.Users, "users", false, "Run Users report")

	// Authentication flags.
	rootCmd.Flags().StringVar(&config.AuthMethod, "auth-method", "token", "Authentication method: token or app")
	rootCmd.Flags().StringVar(&config.EnterpriseSlug, "enterprise", "", "Enterprise slug (required)")
	rootCmd.Flags().StringVar(&config.Token, "token", "", "Authentication token (required if auth-method is token)")
	rootCmd.Flags().Int64Var(&config.GithubAppID, "app-id", 0, "GitHub App ID (required if auth-method is app)")
	rootCmd.Flags().StringVar(&config.GithubAppPrivateKey, "app-private-key-file", "", "GitHub App private key file path (required if auth-method is app)")
	rootCmd.Flags().Int64Var(&config.GithubAppInstallationID, "app-installation-id", 0, "GitHub App installation ID (required if auth-method is app)")

	// Bind flags to Viper
	viper.BindPFlags(rootCmd.Flags())

	// Optionally read in config file and environment variables:
	viper.SetConfigName("config") // name of config file (without extension)
	viper.AddConfigPath(".")      // look for config in the working directory
	viper.AutomaticEnv()          // read in environment variables that match
	if err := viper.ReadInConfig(); err == nil {
		log.Info().Str("Config File", viper.ConfigFileUsed()).Msg("Using config file")
	}
}

// NewRESTClient creates a new REST client configured based on the chosen authentication method.
func NewRESTClient(ctx context.Context, conf *Config) (*github.Client, error) {
	switch conf.AuthMethod {
	case "token":
		rateLimiter := github_ratelimit.New(nil,
			github_primary_ratelimit.WithLimitDetectedCallback(func(ctx *github_primary_ratelimit.CallbackContext) {
				log.Info().Str("category", string(ctx.Category)).Time("resetTime", *ctx.ResetTime).Msg("Primary rate limit detected")
			}),
			github_secondary_ratelimit.WithLimitDetectedCallback(func(ctx *github_secondary_ratelimit.CallbackContext) {
				log.Info().Time("resetTime", *ctx.ResetTime).Dur("totalSleepTime", *ctx.TotalSleepTime).Msg("Secondary rate limit detected")
			}),
		)

		paginator := githubpagination.NewClient(rateLimiter,
			githubpagination.WithPerPage(100), // default to 100 results per page
		)

		client := github.NewClient(paginator).WithAuthToken(conf.Token)
		return client, nil
	case "app":
		itr, err := ghinstallation.NewKeyFromFile(
			http.DefaultTransport,
			conf.GithubAppID,
			conf.GithubAppInstallationID,
			conf.GithubAppPrivateKey,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create installation transport: %w", err)
		}
		client := github.NewClient(&http.Client{Transport: itr})
		return client, nil
	default:
		return nil, fmt.Errorf("unsupported auth-method %q", conf.AuthMethod)
	}
}

// NewGraphQLClient creates a new GraphQL client configured based on the chosen authentication method.
func NewGraphQLClient(ctx context.Context, conf *Config) (*githubv4.Client, error) {
	switch conf.AuthMethod {
	case "token":
		src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: conf.Token})
		httpClient := oauth2.NewClient(ctx, src)
		client := githubv4.NewClient(httpClient)
		return client, nil
	case "app":
		itr, err := ghinstallation.NewKeyFromFile(
			http.DefaultTransport,
			conf.GithubAppID,
			conf.GithubAppInstallationID,
			conf.GithubAppPrivateKey,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create installation transport: %w", err)
		}
		httpClient := &http.Client{Transport: itr}
		client := githubv4.NewClient(httpClient)
		return client, nil
	default:
		return nil, fmt.Errorf("unsupported auth-method %q", conf.AuthMethod)
	}
}

// RunReports executes the selected report logic.
func RunReports(ctx context.Context, conf *Config, restClient *github.Client, graphQLClient *githubv4.Client) {
	if conf.Organizations {
		start := time.Now()
		log.Info().Msg("Running Organizations Report...")
		// TODO: Add Organizations report logic here.
		log.Info().Dur("Duration", time.Since(start)).Msg("Organizations Report completed")
	}
	if conf.Repositories {
		start := time.Now()
		log.Info().Msg("Running Repositories Report...")
		// TODO: Add Repositories report logic here.
		log.Info().Dur("Duration", time.Since(start)).Msg("Repositories Report completed")
	}
	if conf.Teams {
		start := time.Now()
		log.Info().Msg("Running Teams Report...")
		// TODO: Add Teams report logic here.
		log.Info().Dur("Duration", time.Since(start)).Msg("Teams Report completed")
	}
	if conf.Collaborators {
		start := time.Now()
		log.Info().Msg("Running Collaborators Report...")
		// TODO: Add Collaborators report logic here.
		log.Info().Dur("Duration", time.Since(start)).Msg("Collaborators Report completed")
	}
	if conf.Users {
		log.Info().Msg("Running Users Report...")
		currentTime := time.Now()
		// Format the current time to create a unique file name
		formattedTime := currentTime.Format("20060102150405")
		// Create a file name using the enterprise slug and formatted time
		fileName := fmt.Sprintf("%s_users_report_%s.csv", conf.EnterpriseSlug, formattedTime)
		runUsersReport(ctx, restClient, graphQLClient, conf.EnterpriseSlug, fileName)
	}
}
