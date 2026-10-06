// White-box tests of doubles.go.
//declscope:namespace doubles

// Shared test doubles for the staging use-case tests: the strategy mocks
// (service/parser/apply/edit) and the stageEntry helper below are consumed by
// several test files, so the whole file is declared package-wide.
//declscope:shared // consumed by the add/apply/edit/status/export/import use-case tests

package staging

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

type doublesMockServiceStrategy struct {
	hasDeleteOptions bool
	//declscope:private
	service staging.Service
	//declscope:private
	serviceName string
	//declscope:private
	itemName string
}

func (m *doublesMockServiceStrategy) Service() staging.Service { return m.service }
func (m *doublesMockServiceStrategy) ServiceName() string      { return m.serviceName }
func (m *doublesMockServiceStrategy) ItemName() string         { return m.itemName }
func (m *doublesMockServiceStrategy) HasDeleteOptions() bool   { return m.hasDeleteOptions }

func doublesNewParamStrategy() *doublesMockServiceStrategy {
	return &doublesMockServiceStrategy{
		service:          staging.ServiceParam,
		serviceName:      "Parameter Store",
		itemName:         "parameter",
		hasDeleteOptions: false,
	}
}

func doublesNewSecretStrategy() *doublesMockServiceStrategy {
	return &doublesMockServiceStrategy{
		service:          staging.ServiceSecret,
		serviceName:      "Secrets Manager",
		itemName:         "secret",
		hasDeleteOptions: true,
	}
}

type doublesMockParser struct {
	*doublesMockServiceStrategy

	parseErr error
	//declscope:private
	parsedName string
}

func (m *doublesMockParser) ParseName(input string) (string, error) {
	if m.parseErr != nil {
		return "", m.parseErr
	}

	if m.parsedName != "" {
		return m.parsedName, nil
	}

	return input, nil
}

func (m *doublesMockParser) ParseSpec(input string) (string, bool, error) {
	return input, false, nil
}

func doublesNewMockParser() *doublesMockParser {
	return &doublesMockParser{
		doublesMockServiceStrategy: doublesNewParamStrategy(),
	}
}

type doublesMockApplyStrategy struct {
	*doublesMockServiceStrategy

	applyErrors      map[string]error
	lastModified     map[string]time.Time
	fetchModifiedErr error
}

func (m *doublesMockApplyStrategy) Apply(_ context.Context, name string, _ staging.Entry) error {
	if err, ok := m.applyErrors[name]; ok {
		return err
	}

	return nil
}

func (m *doublesMockApplyStrategy) ApplyTags(_ context.Context, _ string, _ staging.TagEntry) error {
	return nil
}

func (m *doublesMockApplyStrategy) FetchLastModified(_ context.Context, name string) (time.Time, error) {
	if m.fetchModifiedErr != nil {
		return time.Time{}, m.fetchModifiedErr
	}

	if t, ok := m.lastModified[name]; ok {
		return t, nil
	}

	return time.Now(), nil
}

func doublesNewMockApplyStrategy() *doublesMockApplyStrategy {
	return &doublesMockApplyStrategy{
		doublesMockServiceStrategy: doublesNewParamStrategy(),
		applyErrors:                make(map[string]error),
		lastModified:               make(map[string]time.Time),
	}
}

type doublesMockEditStrategy struct {
	*doublesMockParser

	fetchResult *staging.EditFetchResult
	fetchErr    error
}

func (m *doublesMockEditStrategy) FetchCurrentValue(_ context.Context, _ string) (*staging.EditFetchResult, error) {
	if m.fetchErr != nil {
		return nil, m.fetchErr
	}

	return m.fetchResult, nil
}

func doublesNewMockEditStrategy() *doublesMockEditStrategy {
	return &doublesMockEditStrategy{
		doublesMockParser: doublesNewMockParser(),
		fetchResult: &staging.EditFetchResult{
			Value:        "aws-value",
			LastModified: time.Now(),
		},
	}
}

// newMockEditStrategyNotFound creates a mock that returns ResourceNotFoundError.
func doublesNewMockEditStrategyNotFound() *doublesMockEditStrategy {
	return &doublesMockEditStrategy{
		doublesMockParser: doublesNewMockParser(),
		fetchErr:          &staging.ResourceNotFoundError{Err: errors.New("resource not found")},
	}
}

func doublesStageEntry(t *testing.T, s *testutil.MockStore, svc staging.Service, name, value string) {
	t.Helper()

	require.NoError(t, s.StageEntry(t.Context(), svc, staging.EntryKey{Name: name}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new(value),
		StagedAt:  time.Now(),
	}))
}
