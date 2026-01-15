// Simple inline diff highlighting utility

export interface DiffSegment {
  text: string;
  type: 'unchanged' | 'added' | 'removed';
}

/**
 * Compute inline diff between two strings using word-level comparison.
 * Returns segments for rendering with highlighting.
 */
export function computeInlineDiff(
  oldText: string,
  newText: string
): { oldSegments: DiffSegment[]; newSegments: DiffSegment[] } {
  const oldLines = oldText.split('\n');
  const newLines = newText.split('\n');

  const oldSegments: DiffSegment[] = [];
  const newSegments: DiffSegment[] = [];

  const maxLines = Math.max(oldLines.length, newLines.length);

  for (let i = 0; i < maxLines; i++) {
    const oldLine = oldLines[i] ?? '';
    const newLine = newLines[i] ?? '';

    if (i > 0) {
      oldSegments.push({ text: '\n', type: 'unchanged' });
      newSegments.push({ text: '\n', type: 'unchanged' });
    }

    if (oldLine === newLine) {
      oldSegments.push({ text: oldLine, type: 'unchanged' });
      newSegments.push({ text: newLine, type: 'unchanged' });
    } else {
      // Compute character-level diff for this line
      const { oldSegs, newSegs } = diffLine(oldLine, newLine);
      oldSegments.push(...oldSegs);
      newSegments.push(...newSegs);
    }
  }

  return { oldSegments, newSegments };
}

function diffLine(
  oldLine: string,
  newLine: string
): { oldSegs: DiffSegment[]; newSegs: DiffSegment[] } {
  // Find common prefix
  let prefixLen = 0;
  while (
    prefixLen < oldLine.length &&
    prefixLen < newLine.length &&
    oldLine[prefixLen] === newLine[prefixLen]
  ) {
    prefixLen++;
  }

  // Find common suffix
  let suffixLen = 0;
  while (
    suffixLen < oldLine.length - prefixLen &&
    suffixLen < newLine.length - prefixLen &&
    oldLine[oldLine.length - 1 - suffixLen] === newLine[newLine.length - 1 - suffixLen]
  ) {
    suffixLen++;
  }

  const oldSegs: DiffSegment[] = [];
  const newSegs: DiffSegment[] = [];

  // Common prefix
  if (prefixLen > 0) {
    oldSegs.push({ text: oldLine.slice(0, prefixLen), type: 'unchanged' });
    newSegs.push({ text: newLine.slice(0, prefixLen), type: 'unchanged' });
  }

  // Different middle part
  const oldMiddle = oldLine.slice(prefixLen, oldLine.length - suffixLen);
  const newMiddle = newLine.slice(prefixLen, newLine.length - suffixLen);

  if (oldMiddle) {
    oldSegs.push({ text: oldMiddle, type: 'removed' });
  }
  if (newMiddle) {
    newSegs.push({ text: newMiddle, type: 'added' });
  }

  // Common suffix
  if (suffixLen > 0) {
    oldSegs.push({ text: oldLine.slice(oldLine.length - suffixLen), type: 'unchanged' });
    newSegs.push({ text: newLine.slice(newLine.length - suffixLen), type: 'unchanged' });
  }

  // Handle empty line case
  if (oldSegs.length === 0 && oldLine === '') {
    // Line was added in new
  }
  if (newSegs.length === 0 && newLine === '') {
    // Line was removed in old
  }

  return { oldSegs, newSegs };
}
