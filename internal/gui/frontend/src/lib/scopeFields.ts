// Scope-form helpers driven by the capability descriptor. A provider lists the
// scope fields its form collects (ProviderCapability.scopeFields), and each
// service names the field it needs (ServiceCapability.scopeField). The table
// below maps a field name to the ScopeSelection property that holds it and to
// how its input is drawn, so no component switches on the provider.
import type { capability, gui } from '../../wailsjs/go/models';

export interface ScopeFieldSpec {
  // key is the ScopeSelection property that holds the field.
  key: keyof gui.ScopeSelection;
  // id is the input's element id (stable for tests and demo scripts).
  id: string;
  label: string;
  placeholder: string;
  hint?: string;
}

// SCOPE_FIELDS describes every capability scope-field name.
export const SCOPE_FIELDS: Record<string, ScopeFieldSpec> = {
  project: {
    key: 'projectId',
    id: 'gcloud-project',
    label: 'Project ID',
    placeholder: 'my-project',
  },
  vault: {
    key: 'vaultName',
    id: 'azure-vault',
    label: 'Key Vault name',
    placeholder: 'my-vault (secrets)',
  },
  store: {
    key: 'storeName',
    id: 'azure-store',
    label: 'App Configuration store',
    placeholder: 'my-store (params)',
  },
  namespace: {
    key: 'namespace',
    id: 'azure-namespace',
    label: 'App Configuration NS',
    placeholder: '(NULL)',
    hint: 'Azure calls this a label; empty means (NULL).',
  },
};

// scopeFieldValue reads the value of a capability scope field from sel.
export function scopeFieldValue(sel: gui.ScopeSelection | null | undefined, field: string): string {
  const spec = SCOPE_FIELDS[field];
  if (!spec || !sel) return '';
  return (sel[spec.key] as string | undefined) ?? '';
}

// scopeFieldSpecs returns the form fields of a provider, in capability order.
export function scopeFieldSpecs(
  cap: capability.ProviderCapability | null | undefined
): [string, ScopeFieldSpec][] {
  return (cap?.scopeFields ?? []).flatMap((f) =>
    SCOPE_FIELDS[f] ? [[f, SCOPE_FIELDS[f]] as [string, ScopeFieldSpec]] : []
  );
}

// servicesInScope returns the services the selection offers: a service with a
// scopeField is available only when that field is set (Azure: vault → Key
// Vault, store → App Configuration; Google Cloud: project → Secret Manager).
export function servicesInScope(
  cap: capability.ProviderCapability | null | undefined,
  sel: gui.ScopeSelection | null | undefined
): capability.ServiceCapability[] {
  return (cap?.services ?? []).filter((s) => !s.scopeField || !!scopeFieldValue(sel, s.scopeField));
}

// hasRequiredScope reports whether sel offers at least one service of its
// provider. A provider without scope fields (AWS) is always complete.
export function hasRequiredScope(
  cap: capability.ProviderCapability | null | undefined,
  sel: gui.ScopeSelection
): boolean {
  return servicesInScope(cap, sel).length > 0;
}

// selectionFor builds a ScopeSelection for provider cap that fills only the
// provider's own scope fields, each from value(field).
export function selectionFor(
  cap: capability.ProviderCapability | null | undefined,
  provider: string,
  value: (field: string) => string
): gui.ScopeSelection {
  const sel = {
    provider,
    projectId: '',
    vaultName: '',
    storeName: '',
    namespace: '',
  } as gui.ScopeSelection;
  for (const [field, spec] of scopeFieldSpecs(cap)) {
    (sel as unknown as Record<string, string>)[spec.key] = value(field);
  }
  return sel;
}
