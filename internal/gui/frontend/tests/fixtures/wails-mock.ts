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
  removeTags: string[];
}

export interface MockState {
  params: Parameter[];
  secrets: Secret[];
  stagedSSM: StagedEntry[];
  stagedSM: StagedEntry[];
  stagedSSMTags: StagedTagEntry[];
  stagedSMTags: StagedTagEntry[];
  paramTags: Record<string, Tag[]>;
  secretTags: Record<string, Tag[]>;
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
  removeTags: string[] = []
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
  stagedSSM: [],
  stagedSM: [],
  stagedSSMTags: [],
  stagedSMTags: [],
  paramTags: {
    '/app/config': [{ key: 'env', value: 'production' }],
  },
  secretTags: {
    'my-secret': [{ key: 'team', value: 'backend' }],
  },
};

/**
 * Empty state - no parameters, secrets, or staged changes
 */
export const emptyMockState: Partial<MockState> = {
  params: [],
  secrets: [],
  stagedSSM: [],
  stagedSM: [],
  stagedSSMTags: [],
  stagedSMTags: [],
  paramTags: {},
  secretTags: {},
};

/**
 * State with SSM staged changes only
 */
export function createSSMStagedState(entries: StagedEntry[]): Partial<MockState> {
  return {
    stagedSSM: entries,
    stagedSM: [],
  };
}

/**
 * State with SM staged changes only
 */
export function createSMStagedState(entries: StagedEntry[]): Partial<MockState> {
  return {
    stagedSSM: [],
    stagedSM: entries,
  };
}

/**
 * State with both SSM and SM staged changes
 */
export function createMixedStagedState(
  ssmEntries: StagedEntry[],
  smEntries: StagedEntry[]
): Partial<MockState> {
  return {
    stagedSSM: ssmEntries,
    stagedSM: smEntries,
  };
}

/**
 * State with tag-only staged changes
 */
export function createTagOnlyStagedState(
  ssmTags: StagedTagEntry[],
  smTags: StagedTagEntry[]
): Partial<MockState> {
  return {
    stagedSSM: [],
    stagedSM: [],
    stagedSSMTags: ssmTags,
    stagedSMTags: smTags,
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

// ============================================================================
// Main Mock Setup Function
// ============================================================================

export async function setupWailsMocks(page: Page, customState?: Partial<MockState>) {
  const state = { ...defaultMockState, ...customState };

  await page.addInitScript((mockState: MockState) => {
    // Track state changes during test
    const state = JSON.parse(JSON.stringify(mockState));

    const mockApp = {
      // Parameter operations
      ParamList: async (_prefix: string, _recursive: boolean, _withValue: boolean, _filter: string) => ({
        entries: state.params.map((p: any) => ({ name: p.name, type: p.type, value: p.value })),
        nextToken: '',
      }),
      ParamShow: async (name: string) => {
        const param = state.params.find((p: any) => p.name === name);
        const tags = state.paramTags[name] || [];
        return {
          name,
          value: param?.value || 'mock-value',
          version: 1,
          type: param?.type || 'String',
          description: '',
          lastModified: new Date().toISOString(),
          tags,
        };
      },
      ParamLog: async (name: string) => ({
        name,
        entries: [
          { version: 1, value: 'current', type: 'String', isCurrent: true, lastModified: new Date().toISOString() },
        ],
      }),
      ParamSet: async (name: string, value: string, _type: string) => {
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
      ParamDiff: async () => ({ oldName: '', newName: '', oldValue: '', newValue: '' }),
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
      SecretList: async () => ({
        entries: state.secrets.map((s: any) => ({ name: s.name, value: s.value })),
        nextToken: '',
      }),
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
      SecretLog: async (name: string) => ({
        name,
        entries: [
          { versionId: 'v1', stages: ['AWSCURRENT'], value: 'current', isCurrent: true, created: new Date().toISOString() },
        ],
      }),
      SecretCreate: async (name: string, value: string) => {
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
      SecretDiff: async () => ({ oldName: '', oldVersionId: '', oldValue: '', newName: '', newVersionId: '', newValue: '' }),
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
        ssm: state.stagedSSM,
        sm: state.stagedSM,
        ssmTags: state.stagedSSMTags,
        smTags: state.stagedSMTags,
      }),
      StagingDiff: async (service: string) => {
        const staged = service === 'ssm' ? state.stagedSSM : state.stagedSM;
        const tagStaged = service === 'ssm' ? state.stagedSSMTags : state.stagedSMTags;
        return {
          itemName: service === 'ssm' ? 'parameter' : 'secret',
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
        const staged = service === 'ssm' ? state.stagedSSM : state.stagedSM;
        const tagStaged = service === 'ssm' ? state.stagedSSMTags : state.stagedSMTags;
        const entryCount = staged.length;
        const tagCount = tagStaged.length;
        if (service === 'ssm') {
          state.stagedSSM = [];
          state.stagedSSMTags = [];
        } else {
          state.stagedSM = [];
          state.stagedSMTags = [];
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
        if (service === 'ssm') {
          const count = state.stagedSSM.length + state.stagedSSMTags.length;
          state.stagedSSM = [];
          state.stagedSSMTags = [];
          return { type: 'all', serviceName: 'ssm', count };
        } else {
          const count = state.stagedSM.length + state.stagedSMTags.length;
          state.stagedSM = [];
          state.stagedSMTags = [];
          return { type: 'all', serviceName: 'sm', count };
        }
      },
      StagingAdd: async (service: string, name: string, value: string) => {
        const staged = service === 'ssm' ? state.stagedSSM : state.stagedSM;
        staged.push({ name, operation: 'create', value });
        return { name };
      },
      StagingEdit: async (service: string, name: string, value: string) => {
        const staged = service === 'ssm' ? state.stagedSSM : state.stagedSM;
        const existing = staged.find((s: any) => s.name === name);
        if (existing) {
          existing.value = value;
        } else {
          staged.push({ name, operation: 'update', value });
        }
        return { name };
      },
      StagingDelete: async (service: string, name: string) => {
        const staged = service === 'ssm' ? state.stagedSSM : state.stagedSM;
        staged.push({ name, operation: 'delete' });
        return { name };
      },
      StagingUnstage: async (service: string, name: string) => {
        if (service === 'ssm') {
          state.stagedSSM = state.stagedSSM.filter((s: any) => s.name !== name);
          state.stagedSSMTags = state.stagedSSMTags.filter((s: any) => s.name !== name);
        } else {
          state.stagedSM = state.stagedSM.filter((s: any) => s.name !== name);
          state.stagedSMTags = state.stagedSMTags.filter((s: any) => s.name !== name);
        }
        return { name };
      },
      StagingAddTag: async (service: string, name: string, key: string, value: string) => {
        const tagStaged = service === 'ssm' ? state.stagedSSMTags : state.stagedSMTags;
        let entry = tagStaged.find((t: any) => t.name === name);
        if (!entry) {
          entry = { name, addTags: {}, removeTags: [] };
          tagStaged.push(entry);
        }
        entry.addTags[key] = value;
        return { name };
      },
      StagingRemoveTag: async (service: string, name: string, key: string) => {
        const tagStaged = service === 'ssm' ? state.stagedSSMTags : state.stagedSMTags;
        let entry = tagStaged.find((t: any) => t.name === name);
        if (!entry) {
          entry = { name, addTags: {}, removeTags: [] };
          tagStaged.push(entry);
        }
        entry.removeTags.push(key);
        return { name };
      },
      StagingCancelAddTag: async (service: string, name: string, key: string) => {
        const tagStaged = service === 'ssm' ? state.stagedSSMTags : state.stagedSMTags;
        const entry = tagStaged.find((t: any) => t.name === name);
        if (entry) {
          delete entry.addTags[key];
        }
        return { name };
      },
      StagingCancelRemoveTag: async (service: string, name: string, key: string) => {
        const tagStaged = service === 'ssm' ? state.stagedSSMTags : state.stagedSMTags;
        const entry = tagStaged.find((t: any) => t.name === name);
        if (entry) {
          entry.removeTags = entry.removeTags.filter((k: string) => k !== key);
        }
        return { name };
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
