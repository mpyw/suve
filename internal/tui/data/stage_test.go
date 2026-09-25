package data_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/file"
	"github.com/mpyw/suve/internal/staging/store/testutil"
	"github.com/mpyw/suve/internal/tui/data"
)

// stagingServiceKey is the all-zero base64 key CI exports; it encrypts the
// working store without touching the keychain.
const stagingServiceKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

// newStagingService stands up a StagingService over the given provider store and
// a temp-home, env-keyed file working store — the same providermock + real
// store/file seam mutate_test.go uses. When useStrategyFor is set, the resolved
// resources carry a non-nil StrategyFor (the Azure App Configuration per-namespace
// branch); otherwise the single Strategy handles every entry. The returned store
// is for pre-staging and read-back assertions.
func newStagingService(
	t *testing.T,
	svcCap capability.ServiceCapability,
	provStore provider.Store,
	useStrategyFor bool,
) (data.StagingService, store.ReadWriteOperator) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUVE_STAGING_KEY", stagingServiceKey)

	st, err := file.NewWorkingStore(provider.AWSScope("123456789012", "ap-northeast-1"))
	require.NoError(t, err)

	resolve := func(context.Context) (data.StagingResources, error) {
		res := data.StagingResources{
			Store:    st,
			Strategy: staging.NewAWSParamStrategy(provStore),
		}
		if useStrategyFor {
			res.StrategyFor = func(string) (staging.FullStrategy, error) {
				return staging.NewAWSParamStrategy(provStore), nil
			}
		}

		return res, nil
	}

	return data.NewStagingService(svcCap, "Parameter Store", resolve), st
}

// stageEntry pre-stages one entry under the param service.
func stageEntry(ctx context.Context, t *testing.T, st store.ReadWriteOperator, key staging.EntryKey, entry staging.Entry) {
	t.Helper()
	require.NoError(t, st.StageEntry(ctx, staging.ServiceParam, key, entry))
}

// TestStagingService_Accessors pins the trivial Service/Label/Capability getters.
//
//nolint:paralleltest // sets HOME / SUVE_STAGING_KEY via t.Setenv (newStagingService)
func TestStagingService_Accessors(t *testing.T) {
	svcCap := awsParamCap(t)

	svc, _ := newStagingService(t, svcCap, &providermock.Store{}, false)

	assert.Equal(t, "param", svc.Service())
	assert.Equal(t, "Parameter Store", svc.Label())
	assert.Equal(t, svcCap, svc.Capability())
}

// TestStagingService_Review drives the concrete Review over a spread of staged
// changes: a normal update, a create, an auto-unstaged no-op update, a delete,
// and a tag change. It asserts the (name, namespace) render order, the
// auto-unstage of a staged value equal to remote, and mapToTags/sortedRemovals
// ordering — plus StagingReview.EntryCount/AutoUnstaged.
//
//nolint:paralleltest // sets HOME / SUVE_STAGING_KEY via t.Setenv (newStagingService)
func TestStagingService_Review(t *testing.T) {
	ctx := context.Background()

	provStore := &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			switch name {
			case "/app/UPDATE":
				return &domain.Entry{Name: name, Value: "old-remote", Type: domain.ValueTypePlaintext, Version: domain.Version{ID: "1"}}, nil
			case "/app/SAME":
				return &domain.Entry{Name: name, Value: "identical", Type: domain.ValueTypePlaintext, Version: domain.Version{ID: "1"}}, nil
			case "/app/DELETE":
				return &domain.Entry{Name: name, Value: "to-delete", Type: domain.ValueTypePlaintext, Version: domain.Version{ID: "1"}}, nil
			case "/app/TAGGED":
				return &domain.Entry{
					Name: name, Value: "v", Type: domain.ValueTypePlaintext, Version: domain.Version{ID: "1"},
					Tags: []domain.Tag{{Key: "dept", Value: "eng"}},
				}, nil
			default:
				return nil, provider.ErrNotFound
			}
		},
	}

	svc, st := newStagingService(t, awsParamCap(t), provStore, false)

	stageEntry(ctx, t, st, staging.EntryKey{Name: "/app/UPDATE"}, staging.Entry{Operation: staging.OperationUpdate, Value: new("new")})
	stageEntry(ctx, t, st, staging.EntryKey{Name: "/app/CREATE"}, staging.Entry{Operation: staging.OperationCreate, Value: new("created")})
	stageEntry(ctx, t, st, staging.EntryKey{Name: "/app/SAME"}, staging.Entry{Operation: staging.OperationUpdate, Value: new("identical")})
	stageEntry(ctx, t, st, staging.EntryKey{Name: "/app/DELETE"}, staging.Entry{Operation: staging.OperationDelete})
	require.NoError(t, st.StageTag(ctx, staging.ServiceParam, staging.EntryKey{Name: "/app/TAGGED"}, staging.TagEntry{
		Add:    map[string]string{"team": "core", "owner": "alice"},
		Remove: maputil.NewSet("dept"),
	}))

	review, err := svc.Review(ctx)
	require.NoError(t, err)

	// Entries render in (name, namespace) order; TAGGED is a tag-only change so it
	// is not an entry row.
	names := lo.Map(review.Entries, func(e data.StagedDiffRow, _ int) string { return e.Name })
	assert.Equal(t, []string{"/app/CREATE", "/app/DELETE", "/app/SAME", "/app/UPDATE"}, names)

	byName := lo.KeyBy(review.Entries, func(e data.StagedDiffRow) string { return e.Name })
	assert.Equal(t, data.StagedDiffCreate, byName["/app/CREATE"].Type)
	assert.Equal(t, "delete", byName["/app/DELETE"].Operation)
	assert.Equal(t, "update", byName["/app/UPDATE"].Operation)
	assert.Equal(t, "old-remote", byName["/app/UPDATE"].RemoteValue)
	assert.Equal(t, "new", byName["/app/UPDATE"].StagedValue)
	assert.Equal(t, data.StagedDiffAutoUnstaged, byName["/app/SAME"].Type)

	// EntryCount excludes the auto-unstaged row; AutoUnstaged reports its key.
	assert.Equal(t, 3, review.EntryCount())
	assert.Equal(t, []data.StagedKey{{Name: "/app/SAME"}}, review.AutoUnstaged())

	// The auto-unstaged entry was actually removed from the store during Review.
	_, err = st.GetEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "/app/SAME"})
	require.ErrorIs(t, err, staging.ErrNotStaged)

	// Tag row: adds sorted by key (mapToTags), removes carry the remote value
	// sorted by key (sortedRemovals).
	require.Len(t, review.Tags, 1)
	tagRow := review.Tags[0]
	assert.Equal(t, "/app/TAGGED", tagRow.Name)
	assert.Equal(t, []data.Tag{{Key: "owner", Value: "alice"}, {Key: "team", Value: "core"}}, tagRow.Adds)
	assert.Equal(t, []data.TagRemoval{{Key: "dept", Value: "eng"}}, tagRow.Removes)
	assert.Equal(t, 1, review.TagCount())
}

// TestStagingService_Review_StrategyForNamespace exercises the App Configuration
// branch: a non-nil StrategyFor resolves a per-namespace strategy, and the same
// name staged under two namespaces sorts by namespace (compareKey's tiebreak).
//
//nolint:paralleltest // sets HOME / SUVE_STAGING_KEY via t.Setenv (newStagingService)
func TestStagingService_Review_StrategyForNamespace(t *testing.T) {
	ctx := context.Background()

	provStore := &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: "remote", Type: domain.ValueTypePlaintext, Version: domain.Version{ID: "1"}}, nil
		},
	}

	svc, st := newStagingService(t, azureParamCap(t), provStore, true)

	update := func(v string) staging.Entry {
		return staging.Entry{Operation: staging.OperationUpdate, Value: new(v)}
	}
	stageEntry(ctx, t, st, staging.EntryKey{Name: "/cfg/X", Namespace: "prod"}, update("staged-p"))
	stageEntry(ctx, t, st, staging.EntryKey{Name: "/cfg/X", Namespace: "dev"}, update("staged-d"))

	review, err := svc.Review(ctx)
	require.NoError(t, err)

	require.Len(t, review.Entries, 2)
	// Same name, so the namespace tiebreak orders dev before prod.
	assert.Equal(t, "dev", review.Entries[0].Namespace)
	assert.Equal(t, "prod", review.Entries[1].Namespace)
}

// TestStagingService_Apply_KeepsEachNamespace pins that one name applied under
// two namespaces yields one result per namespace, each carrying its own
// namespace, instead of collapsing into one result per name.
//
//nolint:paralleltest // sets HOME / SUVE_STAGING_KEY via t.Setenv (newStagingService)
func TestStagingService_Apply_KeepsEachNamespace(t *testing.T) {
	ctx := context.Background()

	provStore := &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: "remote", Type: domain.ValueTypePlaintext, Version: domain.Version{ID: "1"}}, nil
		},
		PutFunc: func(context.Context, string, string, domain.ValueType, string, ...provider.WriteOption) (domain.Version, error) {
			return domain.Version{ID: "2"}, nil
		},
	}

	svc, st := newStagingService(t, azureParamCap(t), provStore, true)

	update := func(v string) staging.Entry {
		return staging.Entry{Operation: staging.OperationUpdate, Value: new(v)}
	}
	stageEntry(ctx, t, st, staging.EntryKey{Name: "/cfg/X", Namespace: "prod"}, update("staged-p"))
	stageEntry(ctx, t, st, staging.EntryKey{Name: "/cfg/X"}, update("staged-null"))

	out, err := svc.Apply(ctx, false)
	require.NoError(t, err)

	require.Len(t, out.Entries, 2)
	assert.Equal(t, "/cfg/X", out.Entries[0].Name)
	assert.Empty(t, out.Entries[0].Namespace, "the null namespace sorts first")
	assert.Equal(t, "/cfg/X", out.Entries[1].Name)
	assert.Equal(t, "prod", out.Entries[1].Namespace)
}

// TestStagingService_Reset covers Reset over both a populated and an empty store,
// pinning stagingResetType's UnstagedAll and NothingStaged mappings and the count.
//
//nolint:paralleltest // sets HOME / SUVE_STAGING_KEY via t.Setenv (newStagingService)
func TestStagingService_Reset(t *testing.T) {
	ctx := context.Background()

	t.Run("unstages everything staged", func(t *testing.T) {
		svc, st := newStagingService(t, awsParamCap(t), &providermock.Store{}, false)

		stageEntry(ctx, t, st, staging.EntryKey{Name: "/app/A"}, staging.Entry{Operation: staging.OperationCreate, Value: new("a")})
		stageEntry(ctx, t, st, staging.EntryKey{Name: "/app/B"}, staging.Entry{Operation: staging.OperationCreate, Value: new("b")})
		require.NoError(t, st.StageTag(ctx, staging.ServiceParam, staging.EntryKey{Name: "/app/C"}, staging.TagEntry{
			Add: map[string]string{"k": "v"},
		}))

		out, err := svc.Reset(ctx)
		require.NoError(t, err)
		assert.Equal(t, data.StagingResetUnstagedAll, out.Type)
		assert.Equal(t, 3, out.Count, "two entries plus one tag change")
		assert.Equal(t, "Parameter Store", out.ServiceLabel)

		_, err = st.GetEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "/app/A"})
		require.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("nothing staged", func(t *testing.T) {
		svc, _ := newStagingService(t, awsParamCap(t), &providermock.Store{}, false)

		out, err := svc.Reset(ctx)
		require.NoError(t, err)
		assert.Equal(t, data.StagingResetNothingStaged, out.Type)
		assert.Zero(t, out.Count)
	})
}

// TestStagingService_Unstage pins the ErrNotStaged-tolerant removal of one item's
// entry and its tags, including the case where neither is staged.
//
//nolint:paralleltest // sets HOME / SUVE_STAGING_KEY via t.Setenv (newStagingService)
func TestStagingService_Unstage(t *testing.T) {
	ctx := context.Background()

	t.Run("removes both the entry and its tags", func(t *testing.T) {
		svc, st := newStagingService(t, awsParamCap(t), &providermock.Store{}, false)

		key := staging.EntryKey{Name: "/app/BOTH"}
		stageEntry(ctx, t, st, key, staging.Entry{Operation: staging.OperationCreate, Value: new("v")})
		require.NoError(t, st.StageTag(ctx, staging.ServiceParam, key, staging.TagEntry{Add: map[string]string{"k": "v"}}))

		require.NoError(t, svc.Unstage(ctx, data.StagedKey{Name: "/app/BOTH"}))

		_, err := st.GetEntry(ctx, staging.ServiceParam, key)
		require.ErrorIs(t, err, staging.ErrNotStaged)
		_, err = st.GetTag(ctx, staging.ServiceParam, key)
		require.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("entry only tolerates a missing tag", func(t *testing.T) {
		svc, st := newStagingService(t, awsParamCap(t), &providermock.Store{}, false)

		key := staging.EntryKey{Name: "/app/ENTRY"}
		stageEntry(ctx, t, st, key, staging.Entry{Operation: staging.OperationCreate, Value: new("v")})

		require.NoError(t, svc.Unstage(ctx, data.StagedKey{Name: "/app/ENTRY"}))

		_, err := st.GetEntry(ctx, staging.ServiceParam, key)
		require.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("nothing staged is a no-op", func(t *testing.T) {
		svc, _ := newStagingService(t, awsParamCap(t), &providermock.Store{}, false)

		require.NoError(t, svc.Unstage(ctx, data.StagedKey{Name: "/app/NONE"}))
	})
}

// TestStagingService_CancelTags covers CancelAddTag / CancelRemoveTag / editStagedTag,
// including the drop-to-empty branch that unstages the tag entry entirely and the
// error surfaced when nothing is staged for the key.
//
//nolint:paralleltest // sets HOME / SUVE_STAGING_KEY via t.Setenv (newStagingService)
func TestStagingService_CancelTags(t *testing.T) {
	ctx := context.Background()

	key := staging.EntryKey{Name: "/app/T"}
	stagedKey := data.StagedKey{Name: "/app/T"}

	seed := func(t *testing.T) (data.StagingService, store.ReadWriteOperator) {
		t.Helper()

		svc, st := newStagingService(t, awsParamCap(t), &providermock.Store{}, false)
		require.NoError(t, st.StageTag(ctx, staging.ServiceParam, key, staging.TagEntry{
			Add:    map[string]string{"a": "1", "b": "2"},
			Remove: maputil.NewSet("x"),
		}))

		return svc, st
	}

	t.Run("CancelAddTag drops one add, keeps the rest staged", func(t *testing.T) {
		svc, st := seed(t)

		require.NoError(t, svc.CancelAddTag(ctx, stagedKey, "a"))

		tag, err := st.GetTag(ctx, staging.ServiceParam, key)
		require.NoError(t, err)
		assert.NotContains(t, tag.Add, "a")
		assert.Contains(t, tag.Add, "b")
		assert.True(t, tag.Remove.Contains("x"))
	})

	t.Run("CancelRemoveTag drops one removal, keeps the rest staged", func(t *testing.T) {
		svc, st := seed(t)

		require.NoError(t, svc.CancelRemoveTag(ctx, stagedKey, "x"))

		tag, err := st.GetTag(ctx, staging.ServiceParam, key)
		require.NoError(t, err)
		assert.False(t, tag.Remove.Contains("x"))
		assert.Len(t, tag.Add, 2)
	})

	t.Run("dropping the last change unstages the tag entry", func(t *testing.T) {
		svc, st := seed(t)

		require.NoError(t, svc.CancelRemoveTag(ctx, stagedKey, "x"))
		require.NoError(t, svc.CancelAddTag(ctx, stagedKey, "a"))
		require.NoError(t, svc.CancelAddTag(ctx, stagedKey, "b"))

		_, err := st.GetTag(ctx, staging.ServiceParam, key)
		require.ErrorIs(t, err, staging.ErrNotStaged, "the emptied tag entry is unstaged")
	})

	t.Run("cancelling a tag that is not staged errors", func(t *testing.T) {
		svc, _ := newStagingService(t, awsParamCap(t), &providermock.Store{}, false)

		err := svc.CancelAddTag(ctx, data.StagedKey{Name: "/app/MISSING"}, "a")
		require.ErrorIs(t, err, staging.ErrNotStaged)
	})
}

// applyAllRemote is a providermock over one existing item whose remote was last
// modified at modified. It records every Put, so a test can tell whether the
// service was written to.
type applyAllRemote struct {
	store *providermock.Store
	puts  []string
	// putErr, when set, fails every Put.
	putErr error
}

func newApplyAllRemote(modified time.Time) *applyAllRemote {
	r := &applyAllRemote{}
	r.store = &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name: name, Value: "remote", Type: domain.ValueTypePlaintext,
				Version: domain.Version{ID: "1"}, Modified: new(modified),
			}, nil
		},
		PutFunc: func(_ context.Context, name, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
			if r.putErr != nil {
				return domain.Version{}, r.putErr
			}

			r.puts = append(r.puts, name)

			return domain.Version{ID: "2"}, nil
		},
	}

	return r
}

// applyAllServices builds the AWS param and secret staging services over one
// in-memory staging store, each staging an update of one item. A remote whose
// last modification is after the staged base time is a conflict.
func applyAllServices(
	t *testing.T, paramConflicts, secretConflicts bool,
) (services []data.StagingService, param, secret *applyAllRemote, st store.ReadWriteOperator) {
	t.Helper()

	base := time.Now().Add(-time.Hour)
	remoteTime := func(conflict bool) time.Time {
		if conflict {
			return time.Now()
		}

		return base.Add(-time.Hour)
	}

	param = newApplyAllRemote(remoteTime(paramConflicts))
	secret = newApplyAllRemote(remoteTime(secretConflicts))
	mem := testutil.NewMockStore()

	stage := func(svc staging.Service, name string) {
		require.NoError(t, mem.StageEntry(t.Context(), svc, staging.EntryKey{Name: name}, staging.Entry{
			Operation: staging.OperationUpdate, Value: new("staged"), StagedAt: time.Now(), BaseModifiedAt: &base,
		}))
	}
	stage(staging.ServiceParam, "/app/p")
	stage(staging.ServiceSecret, "app/s")

	paramSvc := data.NewStagingService(awsParamCap(t), "Parameter Store", func(context.Context) (data.StagingResources, error) {
		return data.StagingResources{Store: mem, Strategy: staging.NewAWSParamStrategy(param.store)}, nil
	})
	secretSvc := data.NewStagingService(awsSecretCap(t), "Secrets Manager", func(context.Context) (data.StagingResources, error) {
		return data.StagingResources{Store: mem, Strategy: staging.NewAWSSecretStrategy(secret.store)}, nil
	})

	return []data.StagingService{paramSvc, secretSvc}, param, secret, mem
}

// requireStaged asserts that both staged updates are still in the store.
func requireStaged(t *testing.T, st store.ReadWriteOperator) {
	t.Helper()

	_, err := st.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/p"})
	require.NoError(t, err, "the param update stays staged")

	_, err = st.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "app/s"})
	require.NoError(t, err, "the secret update stays staged")
}

// TestStagingApplyAll_ConflictInOneServiceAppliesNothing pins #982: a conflict
// in either service rejects the whole apply, in either service order, so the
// conflict-free service is neither written nor unstaged.
func TestStagingApplyAll_ConflictInOneServiceAppliesNothing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		paramConflicts  bool
		secretConflicts bool
		want            []data.StagingApplyResult
	}{
		{
			name: "secret conflicts", secretConflicts: true,
			want: []data.StagingApplyResult{{ServiceLabel: "Secrets Manager", Conflicts: []string{"app/s"}}},
		},
		{
			name: "param conflicts", paramConflicts: true,
			want: []data.StagingApplyResult{{ServiceLabel: "Parameter Store", Conflicts: []string{"/app/p"}}},
		},
		{
			name: "both conflict", paramConflicts: true, secretConflicts: true,
			want: []data.StagingApplyResult{
				{ServiceLabel: "Parameter Store", Conflicts: []string{"/app/p"}},
				{ServiceLabel: "Secrets Manager", Conflicts: []string{"app/s"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			services, param, secret, st := applyAllServices(t, tt.paramConflicts, tt.secretConflicts)

			results, err := data.StagingApplyAll(t.Context(), services, false)
			require.NoError(t, err, "a conflict rejection is reported in the results")
			assert.Equal(t, tt.want, results, "only the conflicting services are reported, with only their conflicts")

			assert.Empty(t, param.puts, "the param service is not written")
			assert.Empty(t, secret.puts, "the secret service is not written")
			requireStaged(t, st)
		})
	}
}

// TestStagingApplyAll_IgnoreConflictsAppliesEveryService pins that with
// conflicts ignored every service is applied, and the results carry each
// target's label in target order.
func TestStagingApplyAll_IgnoreConflictsAppliesEveryService(t *testing.T) {
	t.Parallel()

	services, param, secret, _ := applyAllServices(t, false, true)

	results, err := data.StagingApplyAll(t.Context(), services, true)
	require.NoError(t, err)

	require.Len(t, results, 2)
	assert.Equal(t, "Parameter Store", results[0].ServiceLabel)
	assert.Equal(t, "Secrets Manager", results[1].ServiceLabel)
	assert.Equal(t, []data.ApplyEntryResult{{Name: "/app/p", Status: "updated"}}, results[0].Entries)
	assert.Equal(t, []data.ApplyEntryResult{{Name: "app/s", Status: "updated"}}, results[1].Entries)
	assert.Equal(t, []string{"/app/p"}, param.puts)
	assert.Equal(t, []string{"app/s"}, secret.puts)
}

// TestStagingApplyAll_PartialFailureIsReported pins that a write failure in one
// service is reported in its result rather than as an error, while the other
// service is still applied (the same as the CLI's all-service apply).
func TestStagingApplyAll_PartialFailureIsReported(t *testing.T) {
	t.Parallel()

	services, param, secret, _ := applyAllServices(t, false, false)
	param.putErr = assert.AnError

	results, err := data.StagingApplyAll(t.Context(), services, false)
	require.NoError(t, err, "a per-entry failure is reported in the results")

	require.Len(t, results, 2)
	require.Len(t, results[0].Entries, 1)
	assert.Equal(t, "/app/p", results[0].Entries[0].Name)
	assert.NotEmpty(t, results[0].Entries[0].Error, "the param failure is reported")
	assert.Equal(t, []data.ApplyEntryResult{{Name: "app/s", Status: "updated"}}, results[1].Entries)
	assert.Equal(t, []string{"app/s"}, secret.puts)
}

// TestStagingApplyAll_ResolveErrorIsHard pins that a service whose staging
// resources cannot be resolved fails the apply before anything is written.
func TestStagingApplyAll_ResolveErrorIsHard(t *testing.T) {
	t.Parallel()

	services, param, _, st := applyAllServices(t, false, false)
	broken := data.NewStagingService(awsSecretCap(t), "Secrets Manager", func(context.Context) (data.StagingResources, error) {
		return data.StagingResources{}, assert.AnError
	})

	results, err := data.StagingApplyAll(t.Context(), []data.StagingService{services[0], broken}, false)
	require.ErrorIs(t, err, assert.AnError)
	assert.Nil(t, results)
	assert.Empty(t, param.puts)
	requireStaged(t, st)
}
