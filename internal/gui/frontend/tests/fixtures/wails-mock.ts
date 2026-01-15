import { Page } from '@playwright/test';

// ============================================================================
// Type Definitions
// ============================================================================

export interface Parameter {
  name: string;
  type: 'String' | 'SecureString' | 'StringList';
  value: string;
}

export interface Secret {
  name: string;
  value: string;
}

export interface Tag {
  key: string;
  value: string;
}

export type StagedOperation = 'create' | 'update' | 'delete';

export interface StagedEntry {
  name: string;
  operation: StagedOperation;
  value?: string;
}

export interface StagedTagEntry {
  name: string;
  addTags: Record<string, string>;
  removeTags: Record<string, string>;
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
  stages: string[];
  value: string;
  isCurrent: boolean;
  created: string;
}

export interface AWSIdentity {
  accountId: string;
  region: string;
  profile: string;
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
      { versionId: 'v3-current', stages: ['AWSCURRENT'], value: 'secret-value-1', isCurrent: true, created: new Date().toISOString() },
      { versionId: 'v2-previous', stages: ['AWSPREVIOUS'], value: 'secret-value-old', isCurrent: false, created: new Date(Date.now() - 86400000).toISOString() },
      { versionId: 'v1-initial', stages: [], value: 'secret-value-initial', isCurrent: false, created: new Date(Date.now() - 172800000).toISOString() },
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
        { versionId: 'ver-003', stages: ['AWSCURRENT'], value: '{"current": "v3"}', isCurrent: true, created: new Date().toISOString() },
        { versionId: 'ver-002', stages: ['AWSPREVIOUS'], value: '{"previous": "v2"}', isCurrent: false, created: new Date(Date.now() - 86400000).toISOString() },
        { versionId: 'ver-001', stages: [], value: '{"initial": "v1"}', isCurrent: false, created: new Date(Date.now() - 172800000).toISOString() },
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

// ============================================================================
// Main Mock Setup Function
// ============================================================================

export async function setupWailsMocks(page: Page, customState?: Partial<MockState>) {
  const state = { ...defaultMockState, ...customState };

  await page.addInitScript((mockState: MockState) => {
    // Track state changes during test
    const state = JSON.parse(JSON.stringify(mockState));

    const mockApp = {
      // AWS Identity
      GetAWSIdentity: async () => {
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
              value: withValue ? p.value : undefined
            })),
            nextToken: hasMore ? String(endIndex) : '',
          };
        }

        return {
          entries: filtered.map((p: any) => ({
            name: p.name,
            type: p.type,
            value: withValue ? p.value : undefined
          })),
          nextToken: '',
        };
      },
      ParamShow: async (name: string) => {
        const param = state.params.find((p: any) => p.name === name);
        const tags = state.paramTags[name] || [];
        const versions = state.paramVersions[name];
        const currentVersion = versions ? versions.find((v: any) => v.isCurrent) : null;
        return {
          name,
          value: param?.value || 'mock-value',
          version: currentVersion?.version || 1,
          type: param?.type || 'String',
          description: '',
          lastModified: currentVersion?.lastModified || new Date().toISOString(),
          tags,
        };
      },
      ParamLog: async (name: string, _limit?: number) => {
        const versions = state.paramVersions[name] || [
          { version: 1, value: 'current', type: 'String', isCurrent: true, lastModified: new Date().toISOString() },
        ];
        return {
          name,
          entries: versions,
        };
      },
      ParamSet: async (name: string, value: string, _type: string) => {
        // Simulate error if configured
        if (state.simulateError?.operation === 'ParamSet') {
          throw new Error(state.simulateError.message);
        }
        const existing = state.params.find((p: any) => p.name === name);
        if (existing) {
          existing.value = value;
        } else {
          state.params.push({ name, type: 'String', value });
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
          arn: `arn:aws:secretsmanager:us-east-1:123456789:secret:${name}`,
          versionId: 'v1',
          versionStage: ['AWSCURRENT'],
          value: secret?.value || 'mock-secret',
          description: '',
          createdDate: new Date().toISOString(),
          tags,
        };
      },
      SecretLog: async (name: string, _limit?: number) => {
        const versions = state.secretVersions[name] || [
          { versionId: 'v1', stages: ['AWSCURRENT'], value: 'current', isCurrent: true, created: new Date().toISOString() },
        ];
        return {
          name,
          entries: versions,
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
      StagingStatus: async () => ({
        param: state.stagedParam,
        secret: state.stagedSecret,
        paramTags: state.stagedParamTags,
        secretTags: state.stagedSecretTags,
      }),
      StagingDiff: async (service: string, _passphrase?: string) => {
        const staged = service === 'param' ? state.stagedParam : state.stagedSecret;
        const tagStaged = service === 'param' ? state.stagedParamTags : state.stagedSecretTags;
        return {
          itemName: service === 'param' ? 'parameter' : 'secret',
          entries: staged.map((s: any) => ({
            name: s.name,
            type: s.operation === 'create' ? 'create' : 'normal',
            operation: s.operation,
            awsValue: s.operation === 'create' ? '' : 'aws-value',
            awsIdentifier: s.operation === 'create' ? '' : '#1',
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
        const staged = service === 'param' ? state.stagedParam : state.stagedSecret;
        const tagStaged = service === 'param' ? state.stagedParamTags : state.stagedSecretTags;
        const entryCount = staged.length;
        const tagCount = tagStaged.length;
        if (service === 'param') {
          state.stagedParam = [];
          state.stagedParamTags = [];
        } else {
          state.stagedSecret = [];
          state.stagedSecretTags = [];
        }
        return {
          serviceName: service,
          entryResults: staged.map((s: any) => ({ name: s.name, status: s.operation === 'delete' ? 'deleted' : 'updated' })),
          tagResults: tagStaged.map((t: any) => ({ name: t.name, status: 'updated' })),
          conflicts: [],
          entrySucceeded: entryCount,
          entryFailed: 0,
          tagSucceeded: tagCount,
          tagFailed: 0,
        };
      },
      StagingReset: async (service: string) => {
        if (service === 'param') {
          const count = state.stagedParam.length + state.stagedParamTags.length;
          state.stagedParam = [];
          state.stagedParamTags = [];
          return { type: 'all', serviceName: 'param', count };
        } else {
          const count = state.stagedSecret.length + state.stagedSecretTags.length;
          state.stagedSecret = [];
          state.stagedSecretTags = [];
          return { type: 'all', serviceName: 'secret', count };
        }
      },
      StagingAdd: async (service: string, name: string, value: string) => {
        const staged = service === 'param' ? state.stagedParam : state.stagedSecret;
        staged.push({ name, operation: 'create', value });
        return { name };
      },
      StagingEdit: async (service: string, name: string, value: string) => {
        const staged = service === 'param' ? state.stagedParam : state.stagedSecret;
        const existing = staged.find((s: any) => s.name === name);
        if (existing) {
          existing.value = value;
        } else {
          staged.push({ name, operation: 'update', value });
        }
        return { name };
      },
      StagingDelete: async (service: string, name: string, _keepCurrentValue?: boolean, _currentVersion?: number) => {
        const staged = service === 'param' ? state.stagedParam : state.stagedSecret;
        staged.push({ name, operation: 'delete' });
        return { name };
      },
      StagingUnstage: async (service: string, name: string) => {
        if (service === 'param') {
          state.stagedParam = state.stagedParam.filter((s: any) => s.name !== name);
          state.stagedParamTags = state.stagedParamTags.filter((s: any) => s.name !== name);
        } else {
          state.stagedSecret = state.stagedSecret.filter((s: any) => s.name !== name);
          state.stagedSecretTags = state.stagedSecretTags.filter((s: any) => s.name !== name);
        }
        return { name };
      },
      StagingAddTag: async (service: string, name: string, key: string, value: string) => {
        const tagStaged = service === 'param' ? state.stagedParamTags : state.stagedSecretTags;
        let entry = tagStaged.find((t: any) => t.name === name);
        if (!entry) {
          entry = { name, addTags: {}, removeTags: {} };
          tagStaged.push(entry);
        }
        entry.addTags[key] = value;
        return { name };
      },
      StagingRemoveTag: async (service: string, name: string, key: string) => {
        const tagStaged = service === 'param' ? state.stagedParamTags : state.stagedSecretTags;
        let entry = tagStaged.find((t: any) => t.name === name);
        if (!entry) {
          entry = { name, addTags: {}, removeTags: {} };
          tagStaged.push(entry);
        }
        entry.removeTags[key] = ''; // Value will be fetched from AWS in real implementation
        return { name };
      },
      StagingCancelAddTag: async (service: string, name: string, key: string) => {
        const tagStaged = service === 'param' ? state.stagedParamTags : state.stagedSecretTags;
        const entry = tagStaged.find((t: any) => t.name === name);
        if (entry) {
          delete entry.addTags[key];
        }
        return { name };
      },
      StagingCancelRemoveTag: async (service: string, name: string, key: string) => {
        const tagStaged = service === 'param' ? state.stagedParamTags : state.stagedSecretTags;
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
        const entryCount = state.stagedParam.length + state.stagedSecret.length;
        const tagCount = state.stagedParamTags.length + state.stagedSecretTags.length;

        if (entryCount === 0 && tagCount === 0) {
          throw new Error('nothing to stash');
        }

        // Merge or overwrite file based on mode
        if (mode === 'merge' && state.stashFile.exists) {
          // Merge: combine with existing
          state.stashFile.entries = [...state.stashFile.entries, ...state.stagedParam, ...state.stagedSecret];
          state.stashFile.tags = [...state.stashFile.tags, ...state.stagedParamTags, ...state.stagedSecretTags];
        } else {
          // Overwrite: replace
          state.stashFile.entries = [...state.stagedParam, ...state.stagedSecret];
          state.stashFile.tags = [...state.stagedParamTags, ...state.stagedSecretTags];
        }

        // Update file state
        state.stashFile.exists = true;
        state.stashFile.encrypted = passphrase !== '';

        // Clear agent memory unless keep=true
        if (!keep) {
          state.stagedParam = [];
          state.stagedSecret = [];
          state.stagedParamTags = [];
          state.stagedSecretTags = [];
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
        const agentHasChanges = state.stagedParam.length > 0 || state.stagedSecret.length > 0 ||
                               state.stagedParamTags.length > 0 || state.stagedSecretTags.length > 0;

        const fileEntries = state.stashFile.entries;
        const fileTags = state.stashFile.tags;

        // Apply mode
        let merged = false;
        if (mode === 'merge' && agentHasChanges) {
          // Merge: combine file with agent
          state.stagedParam = [...state.stagedParam, ...fileEntries.filter(e => e.name.startsWith('/'))];
          state.stagedSecret = [...state.stagedSecret, ...fileEntries.filter(e => !e.name.startsWith('/'))];
          state.stagedParamTags = [...state.stagedParamTags, ...fileTags];
          merged = true;
        } else {
          // Overwrite: replace agent with file
          state.stagedParam = fileEntries.filter(e => e.name.startsWith('/'));
          state.stagedSecret = fileEntries.filter(e => !e.name.startsWith('/'));
          state.stagedParamTags = fileTags.filter(t => t.name.startsWith('/'));
          state.stagedSecretTags = fileTags.filter(t => !t.name.startsWith('/'));
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
      StagingCheckStatus: async (service: string, name: string) => {
        const staged = service === 'param' ? state.stagedParam : state.stagedSecret;
        const tagStaged = service === 'param' ? state.stagedParamTags : state.stagedSecretTags;
        const hasEntry = staged.some((s: any) => s.name === name);
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
 * Navigate to a specific view
 */
export async function navigateTo(page: Page, view: 'Parameters' | 'Secrets' | 'Staging') {
  await page.getByRole('button', { name: new RegExp(view, 'i') }).click();
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
