// Shared test doubles for the staging use-case tests: the strategy mocks
// (service/parser/apply/edit) and the stageEntry helper below are consumed by
// several test files, so the whole file is declared package-wide.
//declscope:package // consumed by the add/apply/edit/status/export/import use-case tests

package staging_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

type mockServiceStrategy struct {
	hasDeleteOptions bool
	//declscope:private
	service staging.Service
	//declscope:private
	serviceName string
	//declscope:private
	itemName string
}

func (m *mockServiceStrategy) Service() staging.Service { return m.service }
func (m *mockServiceStrategy) ServiceName() string      { return m.serviceName }
func (m *mockServiceStrategy) ItemName() string         { return m.itemName }
func (m *mockServiceStrategy) HasDeleteOptions() bool   { return m.hasDeleteOptions }

func newParamStrategy() *mockServiceStrategy {
	return &mockServiceStrategy{
		service:          staging.ServiceParam,
		serviceName:      "Parameter Store",
		itemName:         "parameter",
		hasDeleteOptions: false,
	}
}

func newSecretStrategy() *mockServiceStrategy {
	return &mockServiceStrategy{
		service:          staging.ServiceSecret,
		serviceName:      "Secrets Manager",
		itemName:         "secret",
		hasDeleteOptions: true,
	}
}

type mockParser struct {
	*mockServiceStrategy

	parseErr error
	//declscope:private
	parsedName string
}

func (m *mockParser) ParseName(input string) (string, error) {
	if m.parseErr != nil {
		return "", m.parseErr
	}

	if m.parsedName != "" {
		return m.parsedName, nil
	}

	return input, nil
}

func (m *mockParser) ParseSpec(input string) (string, bool, error) {
	return input, false, nil
}

func newMockParser() *mockParser {
	return &mockParser{
		mockServiceStrategy: newParamStrategy(),
	}
}

type mockApplyStrategy struct {
	*mockServiceStrategy

	applyErrors      map[string]error
	lastModified     map[string]time.Time
	fetchModifiedErr error
}

func (m *mockApplyStrategy) Apply(_ context.Context, name string, _ staging.Entry) error {
	if err, ok := m.applyErrors[name]; ok {
		return err
	}

	return nil
}

func (m *mockApplyStrategy) ApplyTags(_ context.Context, _ string, _ staging.TagEntry) error {
	return nil
}

func (m *mockApplyStrategy) FetchLastModified(_ context.Context, name string) (time.Time, error) {
	if m.fetchModifiedErr != nil {
		return time.Time{}, m.fetchModifiedErr
	}

	if t, ok := m.lastModified[name]; ok {
		return t, nil
	}

	return time.Now(), nil
}

func newMockApplyStrategy() *mockApplyStrategy {
	return &mockApplyStrategy{
		mockServiceStrategy: newParamStrategy(),
		applyErrors:         make(map[string]error),
		lastModified:        make(map[string]time.Time),
	}
}

type mockEditStrategy struct {
	*mockParser

	fetchResult *staging.EditFetchResult
	fetchErr    error
}

func (m *mockEditStrategy) FetchCurrentValue(_ context.Context, _ string) (*staging.EditFetchResult, error) {
	if m.fetchErr != nil {
		return nil, m.fetchErr
	}

	return m.fetchResult, nil
}

func newMockEditStrategy() *mockEditStrategy {
	return &mockEditStrategy{
		mockParser: newMockParser(),
		fetchResult: &staging.EditFetchResult{
			Value:        "aws-value",
			LastModified: time.Now(),
		},
	}
}

// newMockEditStrategyNotFound creates a mock that returns ResourceNotFoundError.
func newMockEditStrategyNotFound() *mockEditStrategy {
	return &mockEditStrategy{
		mockParser: newMockParser(),
		fetchErr:   &staging.ResourceNotFoundError{Err: errors.New("resource not found")},
	}
}

func stageEntry(t *testing.T, s *testutil.MockStore, svc staging.Service, name, value string) {
	t.Helper()

	require.NoError(t, s.StageEntry(t.Context(), svc, staging.EntryKey{Name: name}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new(value),
		StagedAt:  time.Now(),
	}))
}
