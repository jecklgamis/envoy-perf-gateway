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
	Short: "Upload rendered cds.yaml and lds.yaml to S3 for the in-container fetcher to pick up",
	Long: `Upload rendered cds.yaml and lds.yaml to S3 for the in-container
fetcher to pick up (CONFIG_SOURCE_KIND=s3). Regenerates first.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if psBucket == "" {
			return fmt.Errorf(`required flag "bucket" not set`)
		}
		if _, err := regenerate(); err != nil {
			return err
		}

		ctx := context.Background()
		cfg, err := awsconfig.LoadDefaultConfig(ctx)
		if err != nil {
			return fmt.Errorf("loading AWS config: %w", err)
		}
		client := s3.NewFromConfig(cfg)
		prefix := strings.TrimLeft(psPrefix, "/")

		for _, filename := range []string{"cds.yaml", "lds.yaml"} {
			localPath := filepath.Join(renderedDir, filename)
			f, err := os.Open(localPath)
			if err != nil {
				return err
			}
			key := prefix + filename
			_, err = client.PutObject(ctx, &s3.PutObjectInput{
				Bucket: aws.String(psBucket),
				Key:    aws.String(key),
				Body:   f,
			})
			f.Close()
			if err != nil {
				return fmt.Errorf("uploading %s: %w", localPath, err)
			}
			fmt.Printf("Uploaded %s -> s3://%s/%s\n", localPath, psBucket, key)
		}
		return nil
	},
}

func init() {
	pushS3Cmd.Flags().StringVar(&psBucket, "bucket", os.Getenv("CONFIG_S3_BUCKET"), "")
	pushS3Cmd.Flags().StringVar(&psPrefix, "prefix", envOr("CONFIG_S3_PREFIX", ""), "Key prefix, e.g. 'envoy-perf-gateway/'")
	rootCmd.AddCommand(pushS3Cmd)
}
