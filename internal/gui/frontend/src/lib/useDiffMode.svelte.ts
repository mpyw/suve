/**
 * Diff mode composable for version comparison
 * Works with both numeric (param) and string (secret) version IDs
 */
export function createDiffMode<T>() {
  let active = $state(false);
  let selectedVersions: T[] = $state([]);

  return {
    get active() {
      return active;
    },
    set active(v: boolean) {
      active = v;
    },
    get selectedVersions() {
      return selectedVersions;
    },
    get canCompare() {
      return selectedVersions.length === 2;
    },

    toggle() {
      active = !active;
      selectedVersions = [];
    },

    toggleSelection(version: T) {
      const idx = selectedVersions.indexOf(version);
      if (idx >= 0) {
        selectedVersions = selectedVersions.filter((v) => v !== version);
      } else if (selectedVersions.length < 2) {
        selectedVersions = [...selectedVersions, version];
      }
    },

    reset() {
      active = false;
      selectedVersions = [];
    },

    isSelected(version: T) {
      return selectedVersions.includes(version);
    },

    isDisabled(version: T) {
      return !selectedVersions.includes(version) && selectedVersions.length >= 2;
    },
  };
}
