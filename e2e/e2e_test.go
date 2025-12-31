//go:build e2e

// Package e2e contains end-to-end tests for the suve CLI.
//
// These tests run against a real AWS-compatible service (localstack) and verify
// the complete workflow of each command. They require Docker to be running and
// localstack to be started via `make up`.
//
// Run with: make e2e
//
// Environment variables:
//   - SUVE_LOCALSTACK_EXTERNAL_PORT: Custom localstack port (default: 4566)
//
// Note: Secrets Manager tests require localstack Pro for full functionality.
package e2e

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/mpyw/suve/internal/version/smversion"
	"github.com/mpyw/suve/internal/version/ssmversion"

	ssmcat "github.com/mpyw/suve/internal/cli/ssm/cat"
	ssmdiff "github.com/mpyw/suve/internal/cli/ssm/diff"
	ssmlog "github.com/mpyw/suve/internal/cli/ssm/log"
	ssmls "github.com/mpyw/suve/internal/cli/ssm/ls"
	ssmrm "github.com/mpyw/suve/internal/cli/ssm/rm"
	ssmset "github.com/mpyw/suve/internal/cli/ssm/set"
	ssmshow "github.com/mpyw/suve/internal/cli/ssm/show"

	smcat "github.com/mpyw/suve/internal/cli/sm/cat"
	smcreate "github.com/mpyw/suve/internal/cli/sm/create"
	smdiff "github.com/mpyw/suve/internal/cli/sm/diff"
	smlog "github.com/mpyw/suve/internal/cli/sm/log"
	smls "github.com/mpyw/suve/internal/cli/sm/ls"
	smrestore "github.com/mpyw/suve/internal/cli/sm/restore"
	smrm "github.com/mpyw/suve/internal/cli/sm/rm"
	smupdate "github.com/mpyw/suve/internal/cli/sm/update"
	smshow "github.com/mpyw/suve/internal/cli/sm/show"
)

func getEndpoint() string {
	port := os.Getenv("SUVE_LOCALSTACK_EXTERNAL_PORT")
	if port == "" {
		port = "4566"
	}
	return fmt.Sprintf("http://127.0.0.1:%s", port)
}

func newSSMClient(t *testing.T) *ssm.Client {
	t.Helper()
	endpoint := getEndpoint()

	cfg, err := config.LoadDefaultConfig(t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatalf("failed to load AWS config: %v", err)
	}

	return ssm.NewFromConfig(cfg, func(o *ssm.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

func newSMClient(t *testing.T) *secretsmanager.Client {
	t.Helper()
	endpoint := getEndpoint()

	cfg, err := config.LoadDefaultConfig(t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatalf("failed to load AWS config: %v", err)
	}

	return secretsmanager.NewFromConfig(cfg, func(o *secretsmanager.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// TestSSM_FullWorkflow tests the complete SSM Parameter Store workflow:
// set → show → cat → update → log → diff → ls → rm → verify deletion
//
// This test creates a parameter, updates it, verifies version history,
// compares versions using diff, and cleans up by deleting.
func TestSSM_FullWorkflow(t *testing.T) {
	ctx := t.Context()
	client := newSSMClient(t)
	paramName := "/suve-e2e-test/param"

	// Cleanup function
	cleanup := func() {
		_, _ = client.DeleteParameter(ctx, &ssm.DeleteParameterInput{
			Name: aws.String(paramName),
		})
	}

	// Clean up before and after test
	cleanup()
	t.Cleanup(cleanup)

	// 1. Set parameter
	t.Run("set", func(t *testing.T) {
		var buf bytes.Buffer
		err := ssmset.Run(ctx, client, &buf, paramName, "initial-value", "String", "")
		if err != nil {
			t.Fatalf("ssmset.Run() error: %v", err)
		}
		t.Logf("set output: %s", buf.String())
	})

	// 2. Show parameter
	t.Run("show", func(t *testing.T) {
		var buf, errBuf bytes.Buffer
		spec := &ssmversion.Spec{Name: paramName}
		err := ssmshow.Run(ctx, client, &buf, &errBuf, spec, true, false)
		if err != nil {
			t.Fatalf("ssmshow.Run() error: %v", err)
		}
		output := buf.String()
		if !bytes.Contains([]byte(output), []byte("initial-value")) {
			t.Errorf("expected output to contain 'initial-value', got: %s", output)
		}
		t.Logf("show output: %s", output)
	})

	// 3. Cat parameter (raw output without trailing newline)
	t.Run("cat", func(t *testing.T) {
		var buf, warnBuf bytes.Buffer
		spec := &ssmversion.Spec{Name: paramName}
		err := ssmcat.Run(ctx, client, &buf, &warnBuf, spec, true, false)
		if err != nil {
			t.Fatalf("ssmcat.Run() error: %v", err)
		}
		output := buf.String()
		if output != "initial-value" {
			t.Errorf("expected 'initial-value', got: %q", output)
		}
	})

	// 4. Update parameter
	t.Run("update", func(t *testing.T) {
		var buf bytes.Buffer
		err := ssmset.Run(ctx, client, &buf, paramName, "updated-value", "String", "")
		if err != nil {
			t.Fatalf("ssmset.Run() error: %v", err)
		}
	})

	// 5. Log (without patch)
	t.Run("log", func(t *testing.T) {
		var buf, errBuf bytes.Buffer
		err := ssmlog.Run(ctx, client, &buf, &errBuf, paramName, ssmlog.Options{MaxResults: 10})
		if err != nil {
			t.Fatalf("ssmlog.Run() error: %v", err)
		}
		t.Logf("log output: %s", buf.String())
	})

	// 6. Diff - Compare version 1 with version 2 using legacy format
	// This tests the Run() function which uses the legacy 3-argument format.
	// The diff should show "initial-value" as removed (-) and "updated-value" as added (+).
	t.Run("diff", func(t *testing.T) {
		var buf bytes.Buffer
		err := ssmdiff.Run(ctx, client, &buf, paramName, "#1", "#2")
		if err != nil {
			t.Fatalf("ssmdiff.Run() error: %v", err)
		}
		output := buf.String()
		t.Logf("diff output: %s", output)
		if !bytes.Contains([]byte(output), []byte("-initial-value")) {
			t.Errorf("expected diff to contain '-initial-value'")
		}
		if !bytes.Contains([]byte(output), []byte("+updated-value")) {
			t.Errorf("expected diff to contain '+updated-value'")
		}
	})

	// 7. List
	t.Run("ls", func(t *testing.T) {
		var buf bytes.Buffer
		err := ssmls.Run(ctx, client, &buf, "/suve-e2e-test/", false)
		if err != nil {
			t.Fatalf("ssmls.Run() error: %v", err)
		}
		output := buf.String()
		if !bytes.Contains([]byte(output), []byte(paramName)) {
			t.Errorf("expected ls output to contain %s, got: %s", paramName, output)
		}
		t.Logf("ls output: %s", output)
	})

	// 8. Delete
	t.Run("rm", func(t *testing.T) {
		var buf bytes.Buffer
		err := ssmrm.Run(ctx, client, &buf, paramName)
		if err != nil {
			t.Fatalf("ssmrm.Run() error: %v", err)
		}
	})

	// 9. Verify deletion
	t.Run("verify-deleted", func(t *testing.T) {
		var buf, errBuf bytes.Buffer
		spec := &ssmversion.Spec{Name: paramName}
		err := ssmshow.Run(ctx, client, &buf, &errBuf, spec, true, false)
		if err == nil {
			t.Error("expected error after deletion, got nil")
		}
	})
}

// TestSM_FullWorkflow tests the complete Secrets Manager workflow:
// create → show → cat → update → log → diff → ls → rm → restore → verify → force-rm
//
// This test creates a secret, updates it, verifies version history using labels,
// compares versions using diff, tests soft delete with recovery, and cleans up
// with force delete.
func TestSM_FullWorkflow(t *testing.T) {
	ctx := t.Context()
	client := newSMClient(t)
	secretName := "suve-e2e-test/secret"

	// Cleanup function
	cleanup := func() {
		_, _ = client.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
			SecretId:                   aws.String(secretName),
			ForceDeleteWithoutRecovery: aws.Bool(true),
		})
	}

	// Clean up before and after test
	cleanup()
	t.Cleanup(cleanup)

	// 1. Create secret
	t.Run("create", func(t *testing.T) {
		var buf bytes.Buffer
		err := smcreate.Run(ctx, client, &buf, secretName, "initial-secret", "E2E test secret")
		if err != nil {
			t.Fatalf("smcreate.Run() error: %v", err)
		}
		t.Logf("create output: %s", buf.String())
	})

	// 2. Show secret
	t.Run("show", func(t *testing.T) {
		var buf, errBuf bytes.Buffer
		spec := &smversion.Spec{Name: secretName}
		err := smshow.Run(ctx, client, &buf, &errBuf, spec, false)
		if err != nil {
			t.Fatalf("smshow.Run() error: %v", err)
		}
		output := buf.String()
		if !bytes.Contains([]byte(output), []byte("initial-secret")) {
			t.Errorf("expected output to contain 'initial-secret', got: %s", output)
		}
		t.Logf("show output: %s", output)
	})

	// 3. Cat secret (raw output without trailing newline)
	t.Run("cat", func(t *testing.T) {
		var buf, warnBuf bytes.Buffer
		spec := &smversion.Spec{Name: secretName}
		err := smcat.Run(ctx, client, &buf, &warnBuf, spec, false)
		if err != nil {
			t.Fatalf("smcat.Run() error: %v", err)
		}
		output := buf.String()
		if output != "initial-secret" {
			t.Errorf("expected 'initial-secret', got: %q", output)
		}
	})

	// 4. Update secret
	t.Run("update", func(t *testing.T) {
		var buf bytes.Buffer
		err := smupdate.Run(ctx, client, &buf, secretName, "updated-secret")
		if err != nil {
			t.Fatalf("smupdate.Run() error: %v", err)
		}
	})

	// 5. Log (without patch)
	t.Run("log", func(t *testing.T) {
		var buf, errBuf bytes.Buffer
		err := smlog.Run(ctx, client, &buf, &errBuf, secretName, smlog.Options{MaxResults: 10})
		if err != nil {
			t.Fatalf("smlog.Run() error: %v", err)
		}
		t.Logf("log output: %s", buf.String())
	})

	// 6. Diff - Compare AWSPREVIOUS with AWSCURRENT using legacy format
	// This tests the Run() function which uses the legacy 3-argument format with labels.
	// After update: AWSPREVIOUS = "initial-secret", AWSCURRENT = "updated-secret"
	// The diff should show "initial-secret" as removed (-) and "updated-secret" as added (+).
	t.Run("diff", func(t *testing.T) {
		var buf bytes.Buffer
		err := smdiff.Run(ctx, client, &buf, secretName, ":AWSPREVIOUS", ":AWSCURRENT")
		if err != nil {
			t.Fatalf("smdiff.Run() error: %v", err)
		}
		output := buf.String()
		t.Logf("diff output: %s", output)
		if !bytes.Contains([]byte(output), []byte("-initial-secret")) {
			t.Errorf("expected diff to contain '-initial-secret'")
		}
		if !bytes.Contains([]byte(output), []byte("+updated-secret")) {
			t.Errorf("expected diff to contain '+updated-secret'")
		}
	})

	// 7. List
	t.Run("ls", func(t *testing.T) {
		var buf bytes.Buffer
		err := smls.Run(ctx, client, &buf, "")
		if err != nil {
			t.Fatalf("smls.Run() error: %v", err)
		}
		output := buf.String()
		if !bytes.Contains([]byte(output), []byte(secretName)) {
			t.Errorf("expected ls output to contain %s, got: %s", secretName, output)
		}
		t.Logf("ls output: %s", output)
	})

	// 8. Delete (with recovery window)
	t.Run("rm", func(t *testing.T) {
		var buf bytes.Buffer
		err := smrm.Run(ctx, client, &buf, secretName, false, 7)
		if err != nil {
			t.Fatalf("smrm.Run() error: %v", err)
		}
	})

	// 9. Restore
	t.Run("restore", func(t *testing.T) {
		var buf bytes.Buffer
		err := smrestore.Run(ctx, client, &buf, secretName)
		if err != nil {
			t.Fatalf("smrestore.Run() error: %v", err)
		}
	})

	// 10. Verify restored
	t.Run("verify-restored", func(t *testing.T) {
		var buf, errBuf bytes.Buffer
		spec := &smversion.Spec{Name: secretName}
		err := smshow.Run(ctx, client, &buf, &errBuf, spec, false)
		if err != nil {
			t.Fatalf("smshow.Run() after restore error: %v", err)
		}
	})

	// 11. Final cleanup (force delete)
	t.Run("force-rm", func(t *testing.T) {
		var buf bytes.Buffer
		err := smrm.Run(ctx, client, &buf, secretName, true, 0)
		if err != nil {
			t.Fatalf("smrm.Run() force delete error: %v", err)
		}
	})
}
