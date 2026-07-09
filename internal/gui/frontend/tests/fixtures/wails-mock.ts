import { Page } from '@playwright/test';

// ============================================================================
// Type Definitions
// ============================================================================

export interface Parameter {
  name: string;
  type: 'String' | 'SecureString' | 'StringList';
  value: string;
  // Azure App Configuration namespace (the axis Azure calls a "label"). Empty /
  // omitted is the null/default namespace. Populated into ParamListEntry.namespace
  // by ParamList so the GUI can filter by namespace client-side (#425).
  namespace?: string;
}

export interface Secret {
  name: string;
  value: string;
  // Optional overrides for SecretShow. When omitted, SecretShow synthesizes an
  // AWS-shaped ARN and an ['AWSCURRENT'] staging label (backward compatible).
  // A provider without these (e.g. an empty arn / empty stagingLabels, as Google
  // Cloud and Azure return) drives the presence-gated rendering in SecretView.
  //
  // stagingLabels and state are the two independent concepts (#419): AWS Secrets
  // Manager populates stagingLabels; Google Cloud + Azure Key Vault populate
  // state (enabled/disabled/destroyed). A version never has both.
  arn?: string;
  stagingLabels?: string[];
  state?: string;
  versionId?: string;
}

export interface Tag {
  key: string;
  value: string;
}

export type StagedOperation = 'create' | 'update' | 'delete';

export type Service = 'param' | 'secret';

export interface StagedEntry {
  name: string;
  operation: StagedOperation;
  value?: string;
  // Optional service classifier. When present, stash drain routes the entry by
  // this field; otherwise it falls back to the AWS name-shape heuristic (a
  // leading '/' means param). Google Cloud/Azure names have no such convention, so
  // provider-aware fixtures should set this explicitly.
  service?: Service;
}

export interface StagedTagEntry {
  name: string;
  addTags: Record<string, string>;
  removeTags: Record<string, string>;
  // See StagedEntry.service.
  service?: Service;
}

export interface ParamLogEntry {
  version: number;
  value: string;
  type: string;
  isCurrent: boolean;
  lastModified: string;
}

export interface SecretLogEntry {
  versionId: string;
  // Two independent concepts (#419): stagingLabels for AWS Secrets Manager,
  // state (enabled/disabled/destroyed) for Google Cloud + Azure Key Vault.
  stagingLabels?: string[];
  state?: string;
  value: string;
  isCurrent: boolean;
  created: string;
  // Per-version tags (Azure Key Vault only).
  tags?: Tag[];
}

export interface AWSIdentity {
  accountId: string;
  region: string;
  profile: string;
}

// ScopeSelection mirrors the Go binding DTO (internal/gui/app.go). Only the
// fields relevant to the chosen provider are meaningful.
export interface ScopeSelection {
  provider: string;
  projectId: string;
  vaultName: string;
  storeName: string;
  namespace: string;
}

// DetectResult mirrors the Go binding DTO (internal/gui/providers.go).
export interface DetectResult {
  param: string;
  secret: string;
  stage: string;
  paramActive: string[];
  secretActive: string[];
  stageActive: string[];
}

// ServiceCapability / ProviderCapability mirror the Go binding DTOs
// (internal/gui/providers.go). The default values in defaultCapabilities MUST
// stay in lockstep with App.Capabilities() there.
export interface ServiceCapability {
  service: string;
  displayName: string;
  hasVersionHistory: boolean;
  hasVersionSpecifiers: boolean;
  hasTags: boolean;
  tagsPerVersion: boolean;
  hasRestore: boolean;
  hasStaging: boolean;
  hasForceDelete: boolean;
  hasRecoveryWindow: boolean;
  hasNamespaces: boolean;
}

export interface ProviderCapability {
  provider: string;
  displayName: string;
  scopeFields: string[];
  services: ServiceCapability[];
}

export interface StashFileState {
  exists: boolean;
  encrypted: boolean;
  // Stored entries (for pop/drain)
  entries: StagedEntry[];
  tags: StagedTagEntry[];
}

export interface MockState {
  params: Parameter[];
  secrets: Secret[];
  stagedParam: StagedEntry[];
  stagedSecret: StagedEntry[];
  stagedParamTags: StagedTagEntry[];
  stagedSecretTags: StagedTagEntry[];
  paramTags: Record<string, Tag[]>;
  secretTags: Record<string, Tag[]>;
  // Advanced: version history for diff testing
  paramVersions: Record<string, ParamLogEntry[]>;
  secretVersions: Record<string, SecretLogEntry[]>;
  // Pagination support
  enablePagination: boolean;
  pageSize: number;
  // AWS Identity
  awsIdentity: AWSIdentity;
  // Stash file state
  stashFile: StashFileState;
  // Provider selection (multi-cloud). Defaults describe an AWS-only environment
  // so existing AWS specs are unaffected.
  initialProvider: string;
  // initialService is the launched service ('param'/'secret', or '' for none),
  // mirroring the Go App.InitialService binding. Drives the initial view.
  initialService: string;
  currentScope: ScopeSelection;
  detectResult: DetectResult;
  capabilities: ProviderCapability[];
  // Error simulation
  simulateError?: {
    operation: string;
    message: string;
  };
}

// ============================================================================
// Factory Functions for Test Data
// ============================================================================

/**
 * Create a parameter entry for mock state
 */
export function createParam(
  name: string,
  value: string = 'test-value',
  type: Parameter['type'] = 'String'
): Parameter {
  return { name, type, value };
}

/**
 * Create a secret entry for mock state
 */
export function createSecret(name: string, value: string = 'secret-value'): Secret {
  return { name, value };
}

/**
 * Create a staged value operation (create/update/delete)
 */
export function createStagedValue(
  name: string,
  operation: StagedOperation,
  value?: string
): StagedEntry {
  return { name, operation, value };
}

/**
 * Create a staged tag operation
 */
export function createStagedTags(
  name: string,
  addTags: Record<string, string> = {},
  removeTags: Record<string, string> = {}
): StagedTagEntry {
  return { name, addTags, removeTags };
}

// ============================================================================
// Preset States for Common Test Scenarios
// ============================================================================

/**
 * Default AWS-only scope selection.
 */
export const awsScopeSelection: ScopeSelection = {
  provider: 'aws',
  projectId: '',
  vaultName: '',
  storeName: '',
  namespace: '',
};

/**
 * Default provider detection result describing an AWS-only environment (the
 * single uniquely-active provider across services).
 */
export const awsOnlyDetectResult: DetectResult = {
  param: 'aws',
  secret: 'aws',
  stage: 'aws',
  paramActive: ['aws'],
  secretActive: ['aws'],
  stageActive: ['aws'],
};

/**
 * Static capability descriptor. This MUST stay a verbatim copy of
 * App.Capabilities() in internal/gui/providers.go — dto_contract guards the
 * DTO shape, not these values, so drift here silently diverges the mock from
 * the backend.
 */
export const defaultCapabilities: ProviderCapability[] = [
  {
    provider: 'aws',
    displayName: 'AWS',
    scopeFields: [],
    services: [
      { service: 'param', displayName: 'Param', hasVersionHistory: true, hasVersionSpecifiers: true, hasTags: true, tagsPerVersion: false, hasRestore: false, hasStaging: true, hasForceDelete: false, hasRecoveryWindow: false, hasNamespaces: false },
      { service: 'secret', displayName: 'Secret', hasVersionHistory: true, hasVersionSpecifiers: true, hasTags: true, tagsPerVersion: false, hasRestore: true, hasStaging: true, hasForceDelete: true, hasRecoveryWindow: true, hasNamespaces: false },
    ],
  },
  {
    provider: 'googlecloud',
    displayName: 'Google Cloud',
    scopeFields: ['project'],
    services: [
      { service: 'secret', displayName: 'Secret', hasVersionHistory: true, hasVersionSpecifiers: true, hasTags: true, tagsPerVersion: false, hasRestore: false, hasStaging: true, hasForceDelete: false, hasRecoveryWindow: false, hasNamespaces: false },
    ],
  },
  {
    provider: 'azure',
    displayName: 'Azure',
    scopeFields: [],
    services: [
      { service: 'param', displayName: 'App Configuration', hasVersionHistory: false, hasVersionSpecifiers: false, hasTags: true, tagsPerVersion: false, hasRestore: false, hasStaging: true, hasForceDelete: false, hasRecoveryWindow: false, hasNamespaces: true },
      { service: 'secret', displayName: 'Key Vault', hasVersionHistory: true, hasVersionSpecifiers: true, hasTags: true, tagsPerVersion: true, hasRestore: false, hasStaging: true, hasForceDelete: false, hasRecoveryWindow: false, hasNamespaces: false },
    ],
  },
];

export const defaultMockState: MockState = {
  params: [
    createParam('/app/config', 'config-value', 'String'),
    createParam('/app/database/url', 'postgres://localhost', 'SecureString'),
    createParam('/app/api/key', 'secret-api-key', 'SecureString'),
  ],
  secrets: [
    createSecret('my-secret', 'secret-value-1'),
    createSecret('api-credentials', '{"key": "value"}'),
    createSecret('database-password', 'super-secret-password'),
  ],
  stagedParam: [],
  stagedSecret: [],
  stagedParamTags: [],
  stagedSecretTags: [],
  paramTags: {
    '/app/config': [{ key: 'env', value: 'production' }],
  },
  secretTags: {
    'my-secret': [{ key: 'team', value: 'backend' }],
  },
  // Default version history for diff testing
  paramVersions: {
    '/app/config': [
      { version: 3, value: 'config-value', type: 'String', isCurrent: true, lastModified: new Date().toISOString() },
      { version: 2, value: 'old-config-value', type: 'String', isCurrent: false, lastModified: new Date(Date.now() - 86400000).toISOString() },
      { version: 1, value: 'initial-config', type: 'String', isCurrent: false, lastModified: new Date(Date.now() - 172800000).toISOString() },
    ],
  },
  secretVersions: {
    'my-secret': [
      { versionId: 'v3-current', stagingLabels: ['AWSCURRENT'], value: 'secret-value-1', isCurrent: true, created: new Date().toISOString() },
      { versionId: 'v2-previous', stagingLabels: ['AWSPREVIOUS'], value: 'secret-value-old', isCurrent: false, created: new Date(Date.now() - 86400000).toISOString() },
      { versionId: 'v1-initial', stagingLabels: [], value: 'secret-value-initial', isCurrent: false, created: new Date(Date.now() - 172800000).toISOString() },
    ],
  },
  enablePagination: false,
  pageSize: 50,
  awsIdentity: {
    accountId: '123456789012',
    region: 'ap-northeast-1',
    profile: 'production',
  },
  stashFile: {
    exists: false,
    encrypted: false,
    entries: [],
    tags: [],
  },
  initialProvider: 'aws',
  initialService: '',
  currentScope: awsScopeSelection,
  detectResult: awsOnlyDetectResult,
  capabilities: defaultCapabilities,
};

/**
 * Empty state - no parameters, secrets, or staged changes
 */
export const emptyMockState: Partial<MockState> = {
  params: [],
  secrets: [],
  stagedParam: [],
  stagedSecret: [],
  stagedParamTags: [],
  stagedSecretTags: [],
  paramTags: {},
  secretTags: {},
};

/**
 * State with Param staged changes only
 */
export function createParamStagedState(entries: StagedEntry[]): Partial<MockState> {
  return {
    stagedParam: entries,
    stagedSecret: [],
  };
}

/**
 * State with Secret staged changes only
 */
export function createSecretStagedState(entries: StagedEntry[]): Partial<MockState> {
  return {
    stagedParam: [],
    stagedSecret: entries,
  };
}

/**
 * State with both Param and Secret staged changes
 */
export function createMixedStagedState(
  paramEntries: StagedEntry[],
  secretEntries: StagedEntry[]
): Partial<MockState> {
  return {
    stagedParam: paramEntries,
    stagedSecret: secretEntries,
  };
}

/**
 * State with tag-only staged changes
 */
export function createTagOnlyStagedState(
  paramTags: StagedTagEntry[],
  secretTags: StagedTagEntry[]
): Partial<MockState> {
  return {
    stagedParam: [],
    stagedSecret: [],
    stagedParamTags: paramTags,
    stagedSecretTags: secretTags,
  };
}

/**
 * State with parameter that has multiple tags
 */
export function createMultiTagState(): Partial<MockState> {
  return {
    paramTags: {
      '/app/config': [
        { key: 'env', value: 'production' },
        { key: 'team', value: 'backend' },
        { key: 'project', value: 'suve' },
      ],
    },
    secretTags: {
      'my-secret': [
        { key: 'team', value: 'backend' },
        { key: 'env', value: 'staging' },
      ],
    },
  };
}

/**
 * State with parameters/secrets that have no tags
 */
export function createNoTagsState(): Partial<MockState> {
  return {
    paramTags: {},
    secretTags: {},
  };
}

/**
 * State with multiple parameters for filter testing
 */
export function createFilterTestState(): Partial<MockState> {
  return {
    params: [
      createParam('/prod/app/config', 'prod-config', 'String'),
      createParam('/prod/app/secret', 'prod-secret', 'SecureString'),
      createParam('/prod/database/url', 'prod-db', 'SecureString'),
      createParam('/dev/app/config', 'dev-config', 'String'),
      createParam('/dev/app/secret', 'dev-secret', 'SecureString'),
      createParam('/staging/app/config', 'staging-config', 'String'),
    ],
    secrets: [
      createSecret('prod-api-key', '{"key": "prod-value"}'),
      createSecret('prod-database', 'prod-db-pass'),
      createSecret('dev-api-key', '{"key": "dev-value"}'),
      createSecret('dev-database', 'dev-db-pass'),
      createSecret('staging-api-key', '{"key": "staging-value"}'),
    ],
  };
}

/**
 * State with version history for diff testing
 */
export function createVersionHistoryState(): Partial<MockState> {
  return {
    paramVersions: {
      '/app/config': [
        { version: 3, value: 'current-value-v3', type: 'String', isCurrent: true, lastModified: new Date().toISOString() },
        { version: 2, value: 'previous-value-v2', type: 'String', isCurrent: false, lastModified: new Date(Date.now() - 86400000).toISOString() },
        { version: 1, value: 'initial-value-v1', type: 'String', isCurrent: false, lastModified: new Date(Date.now() - 172800000).toISOString() },
      ],
    },
    secretVersions: {
      'my-secret': [
        { versionId: 'ver-003', stagingLabels: ['AWSCURRENT'], value: '{"current": "v3"}', isCurrent: true, created: new Date().toISOString() },
        { versionId: 'ver-002', stagingLabels: ['AWSPREVIOUS'], value: '{"previous": "v2"}', isCurrent: false, created: new Date(Date.now() - 86400000).toISOString() },
        { versionId: 'ver-001', stagingLabels: [], value: '{"initial": "v1"}', isCurrent: false, created: new Date(Date.now() - 172800000).toISOString() },
      ],
    },
  };
}

/**
 * State with large dataset for pagination testing
 */
export function createPaginationTestState(itemCount: number = 25): Partial<MockState> {
  const params: Parameter[] = [];
  const secrets: Secret[] = [];

  for (let i = 1; i <= itemCount; i++) {
    params.push(createParam(`/app/param-${String(i).padStart(3, '0')}`, `value-${i}`, 'String'));
    secrets.push(createSecret(`secret-${String(i).padStart(3, '0')}`, `secret-value-${i}`));
  }

  return {
    params,
    secrets,
    enablePagination: true,
    pageSize: 10,
  };
}

/**
 * State that simulates API errors
 */
export function createErrorState(operation: string, message: string): Partial<MockState> {
  return {
    simulateError: { operation, message },
  };
}

/**
 * State with existing stash file (unencrypted)
 */
export function createStashFileState(
  entries: StagedEntry[] = [],
  tags: StagedTagEntry[] = [],
  encrypted: boolean = false
): Partial<MockState> {
  return {
    stashFile: {
      exists: true,
      encrypted,
      entries,
      tags,
    },
  };
}

/**
 * State with encrypted stash file
 */
export function createEncryptedStashFileState(
  entries: StagedEntry[] = [],
  tags: StagedTagEntry[] = []
): Partial<MockState> {
  return createStashFileState(entries, tags, true);
}

/**
 * State with no stash file
 */
export function createNoStashFileState(): Partial<MockState> {
  return {
    stashFile: {
      exists: false,
      encrypted: false,
      entries: [],
      tags: [],
    },
  };
}

/**
 * State with staged changes ready to push
 */
export function createStagedForPushState(): Partial<MockState> {
  return {
    stagedParam: [
      createStagedValue('/test/param', 'create', 'new-value'),
    ],
    stagedSecret: [
      createStagedValue('test-secret', 'update', 'updated-value'),
    ],
    stashFile: {
      exists: false,
      encrypted: false,
      entries: [],
      tags: [],
    },
  };
}

/**
 * State with both agent staged and file staged (for merge/overwrite tests)
 */
export function createBothStagedState(): Partial<MockState> {
  return {
    stagedParam: [
      createStagedValue('/agent/param', 'create', 'agent-value'),
    ],
    stagedSecret: [],
    stashFile: {
      exists: true,
      encrypted: false,
      entries: [
        createStagedValue('/file/param', 'update', 'file-value'),
      ],
      tags: [],
    },
  };
}

/**
 * State with custom AWS identity
 */
export function createAWSIdentityState(
  accountId: string,
  region: string,
  profile: string = ''
): Partial<MockState> {
  return {
    awsIdentity: { accountId, region, profile },
  };
}

/**
 * State with no AWS identity (simulates no AWS credentials)
 */
export function createNoAWSIdentityState(): Partial<MockState> {
  return {
    awsIdentity: { accountId: '', region: '', profile: '' },
  };
}

// ---- Provider-selection states (multi-cloud) ------------------------------

function emptyScope(provider: string): ScopeSelection {
  return { provider, projectId: '', vaultName: '', storeName: '', namespace: '' };
}

/**
 * Launched into Google Cloud (as `suve googlecloud --gui` would), project
 * prefilled — so the app resolves a ready scope without a prompt.
 */
export function createGoogleCloudState(overrides: Partial<MockState> = {}): Partial<MockState> {
  return {
    initialProvider: 'googlecloud',
    currentScope: { ...emptyScope('googlecloud'), projectId: 'my-project' },
    detectResult: {
      param: '',
      secret: 'googlecloud',
      stage: '',
      paramActive: [],
      secretActive: ['googlecloud'],
      stageActive: [],
    },
    // Google Cloud secret versions are integers and carry no ARN/staging labels;
    // they carry a per-version state (enabled/disabled/destroyed) instead.
    secrets: [
      { name: 'gcloud-secret-1', value: 'v1', arn: '', stagingLabels: [], state: 'enabled' },
      { name: 'gcloud-secret-2', value: 'v2', arn: '', stagingLabels: [], state: 'enabled' },
    ],
    secretVersions: {
      'gcloud-secret-1': [
        { versionId: '2', stagingLabels: [], state: 'enabled', value: 'v2', isCurrent: true, created: new Date().toISOString() },
        { versionId: '1', stagingLabels: [], state: 'disabled', value: 'v1', isCurrent: false, created: new Date(Date.now() - 86400000).toISOString() },
      ],
    },
    ...overrides,
  };
}

/**
 * Launched into Azure with vault + store present (as env/CLI would supply),
 * so both Key Vault (secret) and App Configuration (param) mount.
 */
export function createAzureState(overrides: Partial<MockState> = {}): Partial<MockState> {
  return {
    initialProvider: 'azure',
    currentScope: {
      ...emptyScope('azure'),
      vaultName: 'my-vault',
      storeName: 'my-store',
    },
    detectResult: {
      param: 'azure',
      secret: 'azure',
      stage: '',
      paramActive: ['azure'],
      secretActive: ['azure'],
      stageActive: [],
    },
    // App Configuration values are untyped/unversioned; Key Vault has no ARN.
    params: [{ name: 'app/config/key', type: 'String', value: 'v' }],
    // App Configuration tags are writable (azappconfig/v2 GET-merge-PUT), so the
    // param carries a tag to exercise the tag UI.
    paramTags: {
      'app/config/key': [{ key: 'env', value: 'prod' }],
    },
    secrets: [{ name: 'kv-secret', value: 'v', arn: '', stagingLabels: [], state: 'enabled', versionId: 'a1b2c3d4e5f6' }],
    // Key Vault versions are opaque hex ids with no AWS staging labels; they
    // carry a per-version enabled/disabled state instead.
    secretVersions: {
      'kv-secret': [
        { versionId: 'a1b2c3d4e5f6', stagingLabels: [], state: 'enabled', value: 'v', isCurrent: true, created: new Date().toISOString() },
      ],
    },
    ...overrides,
  };
}

/**
 * Azure App Configuration with settings spanning multiple namespaces (the axis
 * Azure calls a "label"): some in the null/default namespace, some in `dev`,
 * some in `prd`. The scope's namespace is left empty (the null/default), so the
 * Namespace dropdown defaults to (NULL). Drives the #425 namespace filter +
 * per-entry namespace display specs.
 */
export function createAzureNamespaceState(overrides: Partial<MockState> = {}): Partial<MockState> {
  return createAzureState({
    params: [
      { name: 'app/config', type: 'String', value: 'null-val', namespace: '' },
      { name: 'app/db', type: 'String', value: 'dev-db', namespace: 'dev' },
      { name: 'app/cache', type: 'String', value: 'dev-cache', namespace: 'dev' },
      { name: 'app/queue', type: 'String', value: 'prd-queue', namespace: 'prd' },
    ],
    paramTags: {},
    ...overrides,
  });
}

/**
 * No explicit initial provider and two providers active → the app must show a
 * selector prompt and preselect nothing.
 */
export function createAmbiguousProviderState(): Partial<MockState> {
  return {
    initialProvider: '',
    currentScope: emptyScope(''),
    detectResult: {
      param: '',
      secret: '',
      stage: '',
      paramActive: ['aws'],
      secretActive: ['googlecloud'],
      stageActive: [],
    },
  };
}

/**
 * No explicit initial provider and nothing active → selector prompt, no crash.
 */
export function createNoActiveProviderState(): Partial<MockState> {
  return {
    initialProvider: '',
    currentScope: emptyScope(''),
    detectResult: {
      param: '',
      secret: '',
      stage: '',
      paramActive: [],
      secretActive: [],
      stageActive: [],
    },
  };
}

/**
 * Read the binding invocations recorded by the mock (see __wailsCalls).
 */
export async function getRecordedCalls(page: Page): Promise<string[]> {
  return page.evaluate(() => ((window as any).__wailsCalls as string[]) ?? []);
}

/**
 * Read the exact ScopeSelection payloads passed to SelectScope.
 */
export async function getSelectScopeCalls(page: Page): Promise<ScopeSelection[]> {
  return page.evaluate(() => ((window as any).__selectScopeCalls as ScopeSelection[]) ?? []);
}

// ============================================================================
// Main Mock Setup Function
// ============================================================================

export async function setupWailsMocks(page: Page, customState?: Partial<MockState>) {
  const state = { ...defaultMockState, ...customState };

  await page.addInitScript((mockState: MockState) => {
    // Track state changes during test
    const state = JSON.parse(JSON.stringify(mockState));

    // Records binding invocations so specs can assert e.g. that no
    // GetAWSIdentity / StagingStatus fires under a non-AWS scope (mount gating).
    const calls: string[] = [];
    (window as any).__wailsCalls = calls;

    // Per-provider staged buckets → staging is scope-isolated (the CLI keys
    // on-disk state by provider.Scope.Key()). The initial flat state seeds the
    // AWS bucket; other providers start empty. Every staging method reads/writes
    // currentBucket(), so switching provider swaps the visible staged set.
    const stagedBuckets: Record<string, {
      param: StagedEntry[];
      secret: StagedEntry[];
      paramTags: StagedTagEntry[];
      secretTags: StagedTagEntry[];
    }> = {};
    function currentBucket() {
      const p = state.currentScope?.provider || 'aws';
      if (!stagedBuckets[p]) {
        stagedBuckets[p] = p === 'aws'
          ? {
              param: state.stagedParam,
              secret: state.stagedSecret,
              paramTags: state.stagedParamTags,
              secretTags: state.stagedSecretTags,
            }
          : { param: [], secret: [], paramTags: [], secretTags: [] };
      }
      return stagedBuckets[p];
    }

    const mockApp = {
      // Provider selection / capabilities (multi-cloud)
      DetectProviders: async () => state.detectResult,
      Capabilities: async () => state.capabilities,
      InitialProvider: async () => state.initialProvider,
      InitialService: async () => state.initialService,
      GetCurrentScope: async () => state.currentScope,
      SelectScope: async (sel: any) => {
        calls.push('SelectScope');
        // Record the exact payload so specs can assert it (incl. vault vs store
        // not being swapped). Rejected calls are still recorded (the attempt).
        (window as any).__selectScopeCalls = (window as any).__selectScopeCalls ?? [];
        (window as any).__selectScopeCalls.push(JSON.parse(JSON.stringify(sel)));
        // Runtime-settable failure so a spec can make a *later* SelectScope fail
        // (e.g. after AWS is already active) to test "keep previous scope".
        if (state.simulateError?.operation === 'SelectScope' || (window as any).__forceSelectScopeError) {
          throw new Error(state.simulateError?.message ?? 'scope selection failed');
        }
        // Mirror the backend validation (internal/gui/app.go) so provider-aware
        // specs exercise the same error paths.
        const p = sel?.provider;
        if (p === 'googlecloud') {
          if (!sel.projectId) {
            throw new Error('Google Cloud project ID is required');
          }
        } else if (p === 'azure') {
          if (!sel.vaultName && !sel.storeName) {
            throw new Error('Azure requires a Key Vault name (for secrets) and/or an App Configuration store name (for parameters)');
          }
        } else if (p !== 'aws') {
          throw new Error(`invalid provider: must be 'aws', 'googlecloud', or 'azure': ${JSON.stringify(p)}`);
        }
        // Keep only the provider-relevant fields, matching scopeFromSelection.
        state.currentScope = {
          provider: p,
          projectId: p === 'googlecloud' ? (sel.projectId || '') : '',
          vaultName: p === 'azure' ? (sel.vaultName || '') : '',
          storeName: p === 'azure' ? (sel.storeName || '') : '',
          namespace: p === 'azure' ? (sel.namespace || '') : '',
        };
      },

      // AWS Identity
      GetAWSIdentity: async () => {
        calls.push('GetAWSIdentity');
        if (state.simulateError?.operation === 'GetAWSIdentity') {
          throw new Error(state.simulateError.message);
        }
        return state.awsIdentity;
      },

      // Parameter operations
      ParamList: async (prefix: string, _recursive: boolean, withValue: boolean, filter: string, pageSize?: number, nextToken?: string) => {
        // Simulate error if configured
        if (state.simulateError?.operation === 'ParamList') {
          throw new Error(state.simulateError.message);
        }

        let filtered = state.params;

        // Apply prefix filter
        if (prefix) {
          filtered = filtered.filter((p: any) => p.name.startsWith(prefix));
        }

        // Apply regex filter
        if (filter) {
          try {
            const regex = new RegExp(filter, 'i');
            filtered = filtered.filter((p: any) => regex.test(p.name));
          } catch {
            // Invalid regex, ignore
          }
        }

        // Handle pagination
        if (state.enablePagination && pageSize) {
          const startIndex = nextToken ? parseInt(nextToken) : 0;
          const endIndex = startIndex + pageSize;
          const hasMore = endIndex < filtered.length;
          return {
            entries: filtered.slice(startIndex, endIndex).map((p: any) => ({
              name: p.name,
              type: p.type,
              secret: p.type === 'SecureString',
              value: withValue ? p.value : undefined,
              namespace: p.namespace ?? (state.currentScope?.namespace ?? '')
            })),
            nextToken: hasMore ? String(endIndex) : '',
          };
        }

        return {
          entries: filtered.map((p: any) => ({
            name: p.name,
            type: p.type,
            secret: p.type === 'SecureString',
            value: withValue ? p.value : undefined,
            namespace: p.namespace ?? (state.currentScope?.namespace ?? '')
          })),
          nextToken: '',
        };
      },
      ParamShow: async (name: string, namespace?: string) => {
        // Record the (name, namespace) the frontend asked for so specs can assert
        // a namespaced App Configuration entry is read under its OWN namespace
        // (the label axis), not the shared read scope's.
        const ns = namespace ?? '';
        (window as any).__paramShowCalls = (window as any).__paramShowCalls ?? [];
        (window as any).__paramShowCalls.push({ name, namespace: ns });
        // Match on (name, namespace): a namespaced setting resolves to its OWN
        // value, never the same-key entry in another namespace. If the name
        // exists only under a DIFFERENT namespace, reject like the real provider
        // ("entry not found") — this is exactly the reported bug when the read
        // was issued under the wrong namespace. A name that exists under no
        // namespace falls back to mock-value so non-namespaced specs are
        // unaffected.
        const exact = state.params.find((p: any) => p.name === name && (p.namespace ?? '') === ns);
        const anyNs = state.params.find((p: any) => p.name === name);
        if (!exact && anyNs) {
          throw new Error(`provider: entry not found: ${name}`);
        }
        const param = exact ?? anyNs;
        const tags = state.paramTags[name] || [];
        const versions = state.paramVersions[name];
        const currentVersion = versions ? versions.find((v: any) => v.isCurrent) : null;
        const showType = param?.type || 'String';
        return {
          name,
          value: param?.value || 'mock-value',
          version: currentVersion?.version || 1,
          type: showType,
          secret: showType === 'SecureString',
          description: '',
          lastModified: currentVersion?.lastModified || new Date().toISOString(),
          tags,
        };
      },
      // Only AWS SSM has value types; Azure App Configuration is untyped
      // (empty list hides the frontend Type dropdown).
      ParamTypeOptions: async () =>
        state.currentScope.provider === 'aws' ? ['String', 'SecureString', 'StringList'] : [],
      ParamLog: async (name: string, _limit?: number) => {
        // Azure App Configuration is unversioned — mirror the backend, which
        // returns ErrVersioningUnsupported from History (the value must still
        // be viewable; only history is unavailable).
        if (state.currentScope?.provider === 'azure') {
          throw new Error('the Azure App Configuration store does not support versions (no version history)');
        }
        const versions = state.paramVersions[name] || [
          { version: 1, value: 'current', type: 'String', isCurrent: true, lastModified: new Date().toISOString() },
        ];
        return {
          name,
          entries: versions.map((v: any) => ({ ...v, secret: v.type === 'SecureString' })),
        };
      },
      ParamSet: async (name: string, value: string, _type: string, namespace?: string) => {
        // Simulate error if configured
        if (state.simulateError?.operation === 'ParamSet') {
          throw new Error(state.simulateError.message);
        }
        // App Configuration keys are per-(name, namespace); match on both so a
        // create under a new namespace doesn't collide with the same key
        // elsewhere. Empty namespace is the null/default.
        const ns = namespace ?? '';
        const existing = state.params.find((p: any) => p.name === name && (p.namespace ?? '') === ns);
        if (existing) {
          existing.value = value;
        } else {
          state.params.push({ name, type: 'String', value, namespace: ns });
        }
        return { name, version: 2, isCreated: !existing };
      },
      ParamDelete: async (name: string) => {
        state.params = state.params.filter((p: any) => p.name !== name);
        return { name };
      },
      ParamDiff: async (spec1: string, spec2: string) => {
        // Parse specs like "/app/config#1" and "/app/config#2"
        const parseSpec = (s: string) => {
          const [name, ver] = s.split('#');
          return { name, version: parseInt(ver) };
        };
        const s1 = parseSpec(spec1);
        const s2 = parseSpec(spec2);
        const versions = state.paramVersions[s1.name] || [];
        const v1 = versions.find((v: any) => v.version === s1.version);
        const v2 = versions.find((v: any) => v.version === s2.version);
        return {
          oldName: s1.name,
          newName: s2.name,
          oldValue: v1?.value || '',
          newValue: v2?.value || '',
        };
      },
      ParamAddTag: async (name: string, key: string, value: string) => {
        if (!state.paramTags[name]) state.paramTags[name] = [];
        const existing = state.paramTags[name].find((t: any) => t.key === key);
        if (existing) {
          existing.value = value;
        } else {
          state.paramTags[name].push({ key, value });
        }
        return { name };
      },
      ParamRemoveTag: async (name: string, key: string) => {
        if (state.paramTags[name]) {
          state.paramTags[name] = state.paramTags[name].filter((t: any) => t.key !== key);
        }
        return { name };
      },

      // Secret operations
      SecretList: async (prefix: string, withValue: boolean, filter: string, pageSize?: number, nextToken?: string) => {
        // Simulate error if configured
        if (state.simulateError?.operation === 'SecretList') {
          throw new Error(state.simulateError.message);
        }

        let filtered = state.secrets;

        // Apply prefix filter
        if (prefix) {
          filtered = filtered.filter((s: any) => s.name.startsWith(prefix));
        }

        // Apply regex filter
        if (filter) {
          try {
            const regex = new RegExp(filter, 'i');
            filtered = filtered.filter((s: any) => regex.test(s.name));
          } catch {
            // Invalid regex, ignore
          }
        }

        // Handle pagination
        if (state.enablePagination && pageSize) {
          const startIndex = nextToken ? parseInt(nextToken) : 0;
          const endIndex = startIndex + pageSize;
          const hasMore = endIndex < filtered.length;
          return {
            entries: filtered.slice(startIndex, endIndex).map((s: any) => ({
              name: s.name,
              value: withValue ? s.value : undefined
            })),
            nextToken: hasMore ? String(endIndex) : '',
          };
        }

        return {
          entries: filtered.map((s: any) => ({
            name: s.name,
            value: withValue ? s.value : undefined
          })),
          nextToken: '',
        };
      },
      SecretShow: async (name: string) => {
        const secret = state.secrets.find((s: any) => s.name === name);
        const tags = state.secretTags[name] || [];
        return {
          name,
          arn: secret?.arn !== undefined ? secret.arn : `arn:aws:secretsmanager:us-east-1:123456789:secret:${name}`,
          versionId: secret?.versionId ?? 'v1',
          stagingLabels: secret?.stagingLabels !== undefined ? secret.stagingLabels : ['AWSCURRENT'],
          state: secret?.state ?? '',
          value: secret?.value || 'mock-secret',
          description: '',
          createdDate: new Date().toISOString(),
          tags,
        };
      },
      SecretLog: async (name: string, _limit?: number) => {
        const versions = state.secretVersions[name] || [
          { versionId: 'v1', stagingLabels: ['AWSCURRENT'], value: 'current', isCurrent: true, created: new Date().toISOString() },
        ];
        return {
          name,
          // Mirror the backend: every entry carries a tags array (empty, not
          // undefined) so per-version tag rendering sees the real shape.
          entries: versions.map((v) => ({ ...v, tags: v.tags ?? [] })),
        };
      },
      SecretCreate: async (name: string, value: string) => {
        // Simulate error if configured
        if (state.simulateError?.operation === 'SecretCreate') {
          throw new Error(state.simulateError.message);
        }
        state.secrets.push({ name, value });
        return { name, versionId: 'v1', arn: `arn:aws:secretsmanager:us-east-1:123456789:secret:${name}` };
      },
      SecretUpdate: async (name: string, value: string) => {
        const existing = state.secrets.find((s: any) => s.name === name);
        if (existing) existing.value = value;
        return { name, versionId: 'v2', arn: '' };
      },
      SecretDelete: async (name: string) => {
        state.secrets = state.secrets.filter((s: any) => s.name !== name);
        return { name, deletionDate: new Date().toISOString(), arn: '' };
      },
      SecretDiff: async (spec1: string, spec2: string) => {
        // Parse specs like "my-secret#v1" and "my-secret#v2"
        const parseSpec = (s: string) => {
          const hashIdx = s.lastIndexOf('#');
          if (hashIdx === -1) return { name: s, versionId: '' };
          return { name: s.substring(0, hashIdx), versionId: s.substring(hashIdx + 1) };
        };
        const s1 = parseSpec(spec1);
        const s2 = parseSpec(spec2);
        const versions = state.secretVersions[s1.name] || [];
        const v1 = versions.find((v: any) => v.versionId === s1.versionId);
        const v2 = versions.find((v: any) => v.versionId === s2.versionId);
        return {
          oldName: s1.name,
          oldVersionId: s1.versionId,
          oldValue: v1?.value || '',
          newName: s2.name,
          newVersionId: s2.versionId,
          newValue: v2?.value || '',
        };
      },
      SecretRestore: async (name: string) => {
        return { name, arn: `arn:aws:secretsmanager:us-east-1:123456789:secret:${name}` };
      },
      SecretAddTag: async (name: string, key: string, value: string) => {
        if (!state.secretTags[name]) state.secretTags[name] = [];
        const existing = state.secretTags[name].find((t: any) => t.key === key);
        if (existing) {
          existing.value = value;
        } else {
          state.secretTags[name].push({ key, value });
        }
        return { name };
      },
      SecretRemoveTag: async (name: string, key: string) => {
        if (state.secretTags[name]) {
          state.secretTags[name] = state.secretTags[name].filter((t: any) => t.key !== key);
        }
        return { name };
      },

      // Staging operations
      StagingStatus: async () => {
        calls.push('StagingStatus');
        return {
          param: currentBucket().param,
          secret: currentBucket().secret,
          paramTags: currentBucket().paramTags,
          secretTags: currentBucket().secretTags,
        };
      },
      StagingDiff: async (service: string, _passphrase?: string) => {
        const staged = service === 'param' ? currentBucket().param : currentBucket().secret;
        const tagStaged = service === 'param' ? currentBucket().paramTags : currentBucket().secretTags;
        return {
          itemName: service === 'param' ? 'parameter' : 'secret',
          entries: staged.map((s: any) => ({
            name: s.name,
            namespace: s.namespace ?? '',
            type: s.operation === 'create' ? 'create' : 'normal',
            operation: s.operation,
            remoteValue: s.operation === 'create' ? '' : 'remote-value',
            remoteIdentifier: s.operation === 'create' ? '' : '#1',
            stagedValue: s.value || '',
            description: '',
            warning: '',
          })),
          tagEntries: tagStaged.map((t: any) => ({
            name: t.name,
            addTags: t.addTags,
            removeTags: t.removeTags,
          })),
        };
      },
      StagingApply: async (service: string) => {
        const staged = service === 'param' ? currentBucket().param : currentBucket().secret;
        const tagStaged = service === 'param' ? currentBucket().paramTags : currentBucket().secretTags;
        const entryCount = staged.length;
        const tagCount = tagStaged.length;
        if (service === 'param') {
          currentBucket().param = [];
          currentBucket().paramTags = [];
        } else {
          currentBucket().secret = [];
          currentBucket().secretTags = [];
        }
        // Mirror the Go backend faithfully: empty slices marshal to null (not
        // []), so the frontend must guard spreads/reads. Returning [] here would
        // hide those bugs (this is exactly how the "Apply All" spread crash
        // slipped past the mock once).
        return {
          serviceName: service,
          entryResults: entryCount > 0 ? staged.map((s: any) => ({ name: s.name, status: s.operation === 'delete' ? 'deleted' : 'updated' })) : null,
          tagResults: tagCount > 0 ? tagStaged.map((t: any) => ({ name: t.name, status: 'updated' })) : null,
          conflicts: null,
          entrySucceeded: entryCount,
          entryFailed: 0,
          tagSucceeded: tagCount,
          tagFailed: 0,
        };
      },
      StagingReset: async (service: string) => {
        if (service === 'param') {
          const count = currentBucket().param.length + currentBucket().paramTags.length;
          currentBucket().param = [];
          currentBucket().paramTags = [];
          return { type: 'all', serviceName: 'param', count };
        } else {
          const count = currentBucket().secret.length + currentBucket().secretTags.length;
          currentBucket().secret = [];
          currentBucket().secretTags = [];
          return { type: 'all', serviceName: 'secret', count };
        }
      },
      StagingAdd: async (service: string, name: string, value: string, namespace?: string) => {
        const staged = service === 'param' ? currentBucket().param : currentBucket().secret;
        // Record the namespace the create targets (App Config only) so specs can
        // assert it; empty/omitted is the null/default namespace.
        staged.push({ name, operation: 'create', value, namespace: namespace ?? '' });
        return { name };
      },
      StagingEdit: async (service: string, name: string, value: string, namespace?: string) => {
        const staged = service === 'param' ? currentBucket().param : currentBucket().secret;
        const ns = namespace ?? '';
        const existing = staged.find((s: any) => s.name === name && (s.namespace ?? '') === ns);
        if (existing) {
          existing.value = value;
        } else {
          staged.push({ name, operation: 'update', value, namespace: ns });
        }
        return { name };
      },
      StagingDelete: async (service: string, name: string, _force?: boolean, _recoveryWindow?: number, namespace?: string) => {
        const staged = service === 'param' ? currentBucket().param : currentBucket().secret;
        staged.push({ name, operation: 'delete', namespace: namespace ?? '' });
        return { name };
      },
      StagingUnstage: async (service: string, name: string, namespace?: string) => {
        const ns = namespace ?? '';
        const sameEntry = (s: any) => s.name === name && (s.namespace ?? '') === ns;
        const sameName = (s: any) => s.name === name;
        if (service === 'param') {
          currentBucket().param = currentBucket().param.filter((s: any) => !sameEntry(s));
          currentBucket().paramTags = currentBucket().paramTags.filter((s: any) => !sameName(s));
        } else {
          currentBucket().secret = currentBucket().secret.filter((s: any) => !sameEntry(s));
          currentBucket().secretTags = currentBucket().secretTags.filter((s: any) => !sameName(s));
        }
        return { name };
      },
      StagingAddTag: async (service: string, name: string, key: string, value: string) => {
        const tagStaged = service === 'param' ? currentBucket().paramTags : currentBucket().secretTags;
        let entry = tagStaged.find((t: any) => t.name === name);
        if (!entry) {
          entry = { name, addTags: {}, removeTags: {} };
          tagStaged.push(entry);
        }
        entry.addTags[key] = value;
        return { name };
      },
      StagingRemoveTag: async (service: string, name: string, key: string) => {
        const tagStaged = service === 'param' ? currentBucket().paramTags : currentBucket().secretTags;
        let entry = tagStaged.find((t: any) => t.name === name);
        if (!entry) {
          entry = { name, addTags: {}, removeTags: {} };
          tagStaged.push(entry);
        }
        entry.removeTags[key] = ''; // Value will be fetched from AWS in real implementation
        return { name };
      },
      StagingCancelAddTag: async (service: string, name: string, key: string) => {
        const tagStaged = service === 'param' ? currentBucket().paramTags : currentBucket().secretTags;
        const entry = tagStaged.find((t: any) => t.name === name);
        if (entry) {
          delete entry.addTags[key];
        }
        return { name };
      },
      StagingCancelRemoveTag: async (service: string, name: string, key: string) => {
        const tagStaged = service === 'param' ? currentBucket().paramTags : currentBucket().secretTags;
        const entry = tagStaged.find((t: any) => t.name === name);
        if (entry) {
          delete entry.removeTags[key];
        }
        return { name };
      },
      StagingFileStatus: async () => {
        if (state.simulateError?.operation === 'StagingFileStatus') {
          throw new Error(state.simulateError.message);
        }
        return { exists: state.stashFile.exists, encrypted: state.stashFile.encrypted };
      },
      StagingPersist: async (_service: string, passphrase: string, keep: boolean, mode: string) => {
        if (state.simulateError?.operation === 'StagingPersist') {
          throw new Error(state.simulateError.message);
        }
        // Count entries and tags to persist
        const entryCount = currentBucket().param.length + currentBucket().secret.length;
        const tagCount = currentBucket().paramTags.length + currentBucket().secretTags.length;

        if (entryCount === 0 && tagCount === 0) {
          throw new Error('nothing to stash');
        }

        // Tag each entry/tag with its originating service so drain can route
        // without inferring from the name shape (which fails for Google Cloud/Azure).
        const persistParamEntries = currentBucket().param.map((e: any) => ({ ...e, service: 'param' }));
        const persistSecretEntries = currentBucket().secret.map((e: any) => ({ ...e, service: 'secret' }));
        const persistParamTags = currentBucket().paramTags.map((t: any) => ({ ...t, service: 'param' }));
        const persistSecretTags = currentBucket().secretTags.map((t: any) => ({ ...t, service: 'secret' }));

        // Merge or overwrite file based on mode
        if (mode === 'merge' && state.stashFile.exists) {
          // Merge: combine with existing
          state.stashFile.entries = [...state.stashFile.entries, ...persistParamEntries, ...persistSecretEntries];
          state.stashFile.tags = [...state.stashFile.tags, ...persistParamTags, ...persistSecretTags];
        } else {
          // Overwrite: replace
          state.stashFile.entries = [...persistParamEntries, ...persistSecretEntries];
          state.stashFile.tags = [...persistParamTags, ...persistSecretTags];
        }

        // Update file state
        state.stashFile.exists = true;
        state.stashFile.encrypted = passphrase !== '';

        // Clear agent memory unless keep=true
        if (!keep) {
          currentBucket().param = [];
          currentBucket().secret = [];
          currentBucket().paramTags = [];
          currentBucket().secretTags = [];
        }

        return { entryCount, tagCount };
      },
      StagingDrain: async (_service: string, passphrase: string, keep: boolean, mode: string) => {
        if (state.simulateError?.operation === 'StagingDrain') {
          throw new Error(state.simulateError.message);
        }

        if (!state.stashFile.exists) {
          throw new Error('no staged changes in file to drain');
        }

        // Check passphrase for encrypted files
        if (state.stashFile.encrypted && !passphrase) {
          throw new Error('passphrase required for encrypted file');
        }

        // Check if agent has existing changes
        const agentHasChanges = currentBucket().param.length > 0 || currentBucket().secret.length > 0 ||
                               currentBucket().paramTags.length > 0 || currentBucket().secretTags.length > 0;

        const fileEntries = state.stashFile.entries;
        const fileTags = state.stashFile.tags;

        // Route each entry/tag by its explicit service when present, falling
        // back to the AWS name-shape heuristic (leading '/' means param) for
        // fixtures that predate the service field. This keeps AWS specs
        // unchanged while working for Google Cloud/Azure names, which carry no '/'.
        const isParam = (item: any) => (item.service ? item.service === 'param' : item.name.startsWith('/'));

        // Apply mode
        let merged = false;
        if (mode === 'merge' && agentHasChanges) {
          // Merge: combine file with agent
          currentBucket().param = [...currentBucket().param, ...fileEntries.filter(isParam)];
          currentBucket().secret = [...currentBucket().secret, ...fileEntries.filter((e: any) => !isParam(e))];
          currentBucket().paramTags = [...currentBucket().paramTags, ...fileTags.filter(isParam)];
          currentBucket().secretTags = [...currentBucket().secretTags, ...fileTags.filter((t: any) => !isParam(t))];
          merged = true;
        } else {
          // Overwrite: replace agent with file
          currentBucket().param = fileEntries.filter(isParam);
          currentBucket().secret = fileEntries.filter((e: any) => !isParam(e));
          currentBucket().paramTags = fileTags.filter(isParam);
          currentBucket().secretTags = fileTags.filter((t: any) => !isParam(t));
        }

        // Delete file unless keep=true
        if (!keep) {
          state.stashFile.exists = false;
          state.stashFile.encrypted = false;
          state.stashFile.entries = [];
          state.stashFile.tags = [];
        }

        return { entryCount: fileEntries.length, tagCount: fileTags.length, merged };
      },
      StagingDrop: async () => {
        if (state.simulateError?.operation === 'StagingDrop') {
          throw new Error(state.simulateError.message);
        }

        if (!state.stashFile.exists) {
          throw new Error('no stashed changes to drop');
        }

        // Delete file directly (works even for encrypted files)
        state.stashFile.exists = false;
        state.stashFile.encrypted = false;
        state.stashFile.entries = [];
        state.stashFile.tags = [];

        return { dropped: true };
      },
      StagingCheckStatus: async (service: string, name: string, namespace?: string) => {
        const staged = service === 'param' ? currentBucket().param : currentBucket().secret;
        const tagStaged = service === 'param' ? currentBucket().paramTags : currentBucket().secretTags;
        const ns = namespace ?? '';
        const hasEntry = staged.some((s: any) => s.name === name && (s.namespace ?? '') === ns);
        const hasTags = tagStaged.some((t: any) => t.name === name);
        return { hasEntry, hasTags };
      },
    };

    (window as any).go = { gui: { App: mockApp } };
    (window as any).runtime = {
      EventsOn: () => {},
      EventsOff: () => {},
      EventsEmit: () => {},
      WindowSetTitle: () => {},
      BrowserOpenURL: () => {},
    };
  }, state);
}

// ============================================================================
// Test Helpers
// ============================================================================

/**
 * Wait for the item list to be ready after navigation/refresh
 */
export async function waitForItemList(page: Page) {
  await page.waitForSelector('.item-list');
}

/**
 * Wait for the view to be loaded (works even when list is empty)
 */
export async function waitForViewLoaded(page: Page) {
  // Wait for the filter bar which is always present even when list is empty
  await page.waitForSelector('.filter-bar');
}

/**
 * Navigate to a specific view.
 *
 * Accepts the legacy AWS labels ('Parameters'/'Secrets') as well as the
 * capability display names surfaced by the provider-aware sidebar ('Param',
 * 'Secret', 'Key Vault', 'App Configuration'), so provider-switching specs can
 * target the right service button regardless of provider.
 */
export type NavLabel =
  | 'Parameters'
  | 'Secrets'
  | 'Staging'
  | 'Param'
  | 'Secret'
  | 'Key Vault'
  | 'App Configuration';

export async function navigateTo(page: Page, view: NavLabel) {
  // Scope to the sidebar nav so a short capability label (e.g. "Secret") does
  // not collide with item names that contain the same word (e.g. "my-secret").
  await page.locator('.nav').getByRole('button', { name: new RegExp(view, 'i') }).click();
}

/**
 * Click on an item by its name (more reliable than nth())
 */
export async function clickItemByName(page: Page, name: string) {
  await page.locator('.item-button').filter({ hasText: name }).click();
}

/**
 * Open the create modal for parameters or secrets
 */
export async function openCreateModal(page: Page) {
  await page.getByRole('button', { name: '+ New' }).click();
}

/**
 * Close any open modal by clicking Cancel
 */
export async function closeModal(page: Page) {
  await page.getByRole('button', { name: 'Cancel' }).click();
}
