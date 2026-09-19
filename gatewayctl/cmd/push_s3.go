package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
)

var (
	psBucket string
	psPrefix string
)

var pushS3Cmd = &cobra.Command{
	Use:   "push-s3",
	Short: "Upload rendered cds.yaml, lds.yaml, and runtime.yaml to S3 for the in-container fetcher to pick up",
	Long: `Upload rendered cds.yaml, lds.yaml, and runtime.yaml to S3 for the in-container
fetcher to pick up (CONFIG_SOURCE_KIND=s3). Regenerates first.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		bucket := psBucket
		if !cmd.Flags().Changed("bucket") {
			bucket = resolveS3Bucket()
		}
		prefix := psPrefix
		if !cmd.Flags().Changed("prefix") {
			prefix = resolveS3Prefix()
		}
		if bucket == "" {
			return fmt.Errorf(`required flag "bucket" not set (and no s3.bucket in the settings file)`)
		}
		if _, err := regenerate(); err != nil {
			return err
		}
		return pushS3Files(bucket, prefix)
	},
}

// pushS3Files uploads the already-rendered cds.yaml/lds.yaml/runtime.yaml. Callers that
// need a fresh render first (the push-s3 command, invoked standalone) call
// regenerate() themselves before this.
func pushS3Files(bucket, prefix string) error {
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}
	client := s3.NewFromConfig(cfg)
	prefix = strings.TrimLeft(prefix, "/")

	for _, filename := range []string{"cds.yaml", "lds.yaml", "runtime.yaml"} {
		localPath := filepath.Join(renderedDir, filename)
		f, err := os.Open(localPath)
		if err != nil {
			return err
		}
		key := prefix + filename
		_, err = client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
			Body:   f,
		})
		f.Close()
		if err != nil {
			return fmt.Errorf("uploading %s: %w", localPath, err)
		}
		fmt.Printf("Uploaded %s -> s3://%s/%s\n", localPath, bucket, key)
	}
	return nil
}

func init() {
	// Actual resolution (CLI flag > env var > settings file) happens in
	// RunE via resolveS3Bucket()/resolveS3Prefix() - see push_http.go's
	// init() for why it can't happen here.
	pushS3Cmd.Flags().StringVar(&psBucket, "bucket", "",
		"S3 bucket. Falls back to CONFIG_S3_BUCKET, then the settings file's s3.bucket.")
	pushS3Cmd.Flags().StringVar(&psPrefix, "prefix", "",
		"Key prefix, e.g. 'envoy-perf-gateway/'. Falls back to CONFIG_S3_PREFIX, then the settings file's s3.prefix.")
	rootCmd.AddCommand(pushS3Cmd)
}
