// Rego Playground Back End Service

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/open-policy-agent/rego-playground/api"
	"github.com/open-policy-agent/rego-playground/utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// cliConfig represents the service config
// Note: After startup use viper.Get() to retrieve
// final configuration!
type cliConfig struct {
	HTTPAddr net.IP
	HTTPPort uint16

	NoPersist bool

	Local          bool   `flag:"aws-local,BoolVar,use local AWS development S3 versions"`
	Region         string `flag:"aws-region,StringVar,AWS region"`
	ResourcePrefix string `flag:"aws-resource-prefix,StringVar,AWS S3 bucket prefix"`
	S3Endpoint     string `flag:"aws-s3-endpoint,StringVar,AWS S3 endpoint"`

	Verbose   bool
	LogFormat string

	UIContentRoot string
	ExternalURL   string

	ConfigFile string
}

const (
	configKeyHTTPAddr       = "addr"
	configKeyHTTPPort       = "port"
	configKeyVerbose        = "verbose"
	configKeyLogFormat      = "log-format"
	configKeyNoPersist      = "no-persist"
	configKeyRegion         = "aws-region"
	configKeyResourcePrefix = "resource-prefix"
	configKeyS3Endpoint     = "s3-endpoint"
	configKeyUIContentRoot  = "ui-content-root"
	configKeyExternalURL    = "external-url"
	configKeyConfigFile     = "config-file"
)

var (
	config        cliConfig
	configLoadErr error
	cmd           *cobra.Command
)

func init() {
	cobra.OnInitialize(initConfig)

	cmd = &cobra.Command{
		Use:   path.Base(os.Args[0]) + " [OPTIONS] [arg...]",
		Short: "Rego Playground",
		Long:  "Rego Playground",
		Run:   run,
	}

	config = cliConfig{
		HTTPAddr: net.IPv4zero,
		HTTPPort: 8181,
	}

	// Setup CLI flags
	cmd.Flags().IPVar(&config.HTTPAddr, configKeyHTTPAddr, config.HTTPAddr, "HTTP bind address.")
	cmd.Flags().Uint16Var(&config.HTTPPort, configKeyHTTPPort, config.HTTPPort, "HTTP bind port.")
	cmd.Flags().BoolVar(&config.Verbose, configKeyVerbose, config.Verbose, "Enable verbose logging.")
	cmd.Flags().StringVar(&config.LogFormat, configKeyLogFormat, "json", "Log format, valid options are 'text', 'json', 'json-pretty'.")
	cmd.Flags().BoolVar(&config.NoPersist, configKeyNoPersist, config.NoPersist, "Disables persistence to S3, in-memory storage only.")
	cmd.Flags().StringVar(&config.Region, configKeyRegion, config.Region, "AWS region")
	cmd.Flags().StringVar(&config.ResourcePrefix, configKeyResourcePrefix, config.ResourcePrefix, "AWS S3 bucket prefix.")
	cmd.Flags().StringVar(&config.S3Endpoint, configKeyS3Endpoint, config.S3Endpoint, "AWS S3 endpoint.")
	cmd.Flags().StringVar(&config.UIContentRoot, configKeyUIContentRoot, "/openpolicyagent/ui", "Root directory of the ui content to be served.")
	cmd.Flags().StringVar(&config.ExternalURL, configKeyExternalURL, "https://play.openpolicyagent.org", "The external URL which the service should be accessed.")
	cmd.Flags().StringVar(&config.ConfigFile, configKeyConfigFile, "", "Config file to use (same options as via CLI or ENV)")

	// Setup config file bindings
	err := viper.BindPFlags(cmd.Flags())
	if err != nil {
		log.Fatalf("Unexpected error: %s", err)
	}
}

func initConfig() {
	// Config options can be overridden from environment
	// with the format PLAYGROUND_<config> (all uppercase
	// and "_" instead of "-")
	// Ex: PLAYGROUND_PORT=8123 sets the "port" option to 8123
	// Ex: PLAYGROUND_NO_PERSIST=1 sets the "no-persist" option to true
	viper.SetEnvPrefix("playground")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	if config.ConfigFile != "" {
		viper.SetConfigFile(config.ConfigFile)
		configLoadErr = viper.ReadInConfig()
	}
}

func main() {
	if err := cmd.Execute(); err != nil {
		log.WithField("error", err).Error("Error running rego-playground")
		os.Exit(1)
	}
	os.Exit(0)
}

func run(cmd *cobra.Command, args []string) {
	if configLoadErr != nil {
		log.Fatal(configLoadErr)
	}

	setupLogging(viper.GetString(configKeyLogFormat))

	var v1Store api.DataRequestStore
	var v2Store api.DataRequestStore

	githubClientID := os.Getenv("PLAYGROUND_GITHUB_ID")
	githubClientSecret := os.Getenv("PLAYGROUND_GITHUB_SECRET")
	if githubClientID == "" || githubClientSecret == "" {
		log.Fatal("failed to find the env variables PLAYGROUND_GITHUB_ID or PLAYGROUND_GITHUB_SECRET")
	}

	if viper.GetBool(configKeyNoPersist) {
		v1Store = api.NewMemoryDataRequestStore()
	} else {
		var client *s3.S3
		var err error
		var bucketName string

		for retries := 0; true; retries++ {
			client, bucketName, err = createTestBucket()
			if err != nil {
				log.Errorf("S3 bucket creation failed: %s", err.Error())
				time.Sleep(utils.Backoff(retries))
				continue
			}
			break
		}

		v1Store = api.NewS3DataRequestStore(client, bucketName)
	}

	if githubClientID != "" {
		v2Store = api.NewGistStore(api.GistStoreExternalURL(viper.GetString(configKeyExternalURL)))
	}

	addr := fmt.Sprintf("%v:%v", viper.GetString(configKeyHTTPAddr), viper.GetString(configKeyHTTPPort))
	apiService := api.NewAPIService(addr, v1Store, v2Store, viper.GetString(configKeyUIContentRoot), viper.GetString(configKeyExternalURL), githubClientID, githubClientSecret)

	ctx := context.Background()
	utils.RunServices(ctx,
		apiService,
	)
}

func createTestBucket() (*s3.S3, string, error) {
	s3conn := s3Client()

	bucketName := fmt.Sprintf("%s-test", viper.GetString(configKeyResourcePrefix))

	input := &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	}

	_, err := s3conn.CreateBucket(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeBucketAlreadyExists:
				log.Infof("Bucket %v already exists", bucketName)
			case s3.ErrCodeBucketAlreadyOwnedByYou:
				log.Infof("Bucket %v already exists", bucketName)
			default:
				log.Fatalf("Error while creating bucket %v", aerr.Error())
				return nil, bucketName, err
			}
		} else {
			log.Fatalf("Error while creating bucket %v", err.Error())
			return nil, bucketName, err
		}
	} else {
		log.Infof("Created bucket %v", bucketName)
	}
	return s3conn, bucketName, nil
}

func awsConfig(region string, service string, endpoint string) *aws.Config {
	if region == "" {
		fmt.Printf("%s: you must specify --aws-region\n", path.Base(os.Args[0]))
		os.Exit(1)
	}

	if viper.GetString(configKeyResourcePrefix) == "" {
		fmt.Printf("%s: you must specify --resource-prefix\n", path.Base(os.Args[0]))
		os.Exit(1)
	}

	if endpoint == "" {
		if re, err := endpoints.AwsPartition().EndpointFor(service, region); err != nil {
			fmt.Printf("%s: you must specify a valid AWS region for --aws-region\n", path.Base(os.Args[0]))
			os.Exit(1)
		} else {
			endpoint = re.URL
		}
	}

	config := aws.NewConfig().WithRegion(region).WithEndpoint(endpoint)

	return config
}

// s3Client returns a S3 client.
func s3Client() *s3.S3 {
	return s3.New(session.New(s3Config()))
}

// S3Config returns a S3 config.
func s3Config() *aws.Config {
	return awsConfig(viper.GetString(configKeyRegion), "s3", viper.GetString(configKeyS3Endpoint)).WithDisableSSL(true).WithS3ForcePathStyle(true)
}

func setupLogging(logformat string) {
	switch logformat {
	case "text":
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	case "json-pretty":
		log.SetFormatter(&log.JSONFormatter{PrettyPrint: true})
	case "json":
		fallthrough
	default:
		log.SetFormatter(&log.JSONFormatter{})
	}

	logLevel := log.InfoLevel
	if viper.GetBool(configKeyVerbose) {
		logLevel = log.DebugLevel
	}
	log.SetLevel(logLevel)
}
