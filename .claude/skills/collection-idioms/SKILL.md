---
name: collection-idioms
description: Load when writing or reviewing Go that builds or transforms slices/maps. Prefer samber/lo, samber/lo/it, and stdlib slices/maps callback & iterator utilities over hand-rolled make + range + append; keep an explicit loop only where side effects or control flow are genuinely complex.
---

# Collection idioms

Default to declarative transforms. A `out := make([]T, 0, len(xs)); for _, x := range xs { out = append(out, f(x)) }; return out` should almost always be one expression instead.

## Toolbox

- **slice → slice, pure 1:1** — `lo.Map(xs, func(x T, _ int) U { return ... })`
- **slice → slice, filtered** — `lo.Filter(xs, func(x T, _ int) bool { ... })`; **filter + transform in one pass** — `lo.FilterMap(xs, func(x T, _ int) (U, bool) { ...; return u, keep })`
- **iterator (`iter.Seq`) → slice** — `slices.Collect(it.Map(seq, func(x T) U { ... }))`, where `it` is `github.com/samber/lo/it` (range-over-func iterators)
- **sorted map keys** — `maputil.SortedKeys(m)` returns `iter.Seq[K]` (ascending); `maputil.SortedNames(items, getName)` returns the unique names as `iter.Seq[string]`. Under the hood these are `slices.Sorted(maps.Keys(m))` — use the stdlib form directly when you don't want the helper.
- **stdlib building blocks** — `slices.Collect`, `slices.Sorted`, `slices.SortFunc`, `slices.Values`, `maps.Keys`, `maps.Values`
- **lookup** — `lo.Find`, `lo.FindIndexOf`, `lo.MaxBy`

Use `it.Map`/`it.Filter` **only when the source is genuinely an `iter.Seq`** (e.g. `maputil.SortedKeys(m)`). If the source is a plain slice, use `lo.Map` — don't wrap a slice in an iterator just to transform it.

## Keep an explicit loop when the body…

This is the "complex side effects" carve-out — readability wins, don't force a helper:

- **side-effects per element** — writes to an `io.Writer`, logs, mutates external state
- **control flow** — early `return`/`break`/`continue`-as-search, or it's a find/any/all
- **cross-iteration state** — a dedup `seen` set, a running accumulator, pagination (`NextToken`) that appends across pages
- **builds a map** with computed keys, or mutates a map in place (`delete`) — the `lo`/`it` map helpers target slices, not map construction
- **type mismatch** — e.g. a struct key that isn't `cmp.Ordered`, which needs `slices.SortFunc`, not `maputil.SortedKeys`
- forcing the functional form would be *less* readable than the loop

A single per-element `if` that only sets fields on the value being produced is fine inside a `lo.Map`/`lo.FilterMap` closure — that is not "complex".

## Out of scope

`lo.FromPtr` / `lo.ToPtr` are pointer helpers, unrelated to this convention. Leave them as-is; pointer-shape cleanups are a separate concern.

## Examples

Slice → slice:

```go
// avoid
entries := make([]genericlist.Entry, len(result.Entries))
for i, e := range result.Entries {
	entries[i] = genericlist.Entry{Name: e.Name, Value: e.Value, Error: e.Error}
}
// prefer
entries := lo.Map(result.Entries, func(e ListEntry, _ int) genericlist.Entry {
	return genericlist.Entry{Name: e.Name, Value: e.Value, Error: e.Error}
})
```

Iterator (sorted keys) → slice:

```go
// avoid
out := make([]domain.Tag, 0, len(tags))
for k := range maputil.SortedKeys(tags) {
	out = append(out, domain.Tag{Key: k, Value: lo.FromPtr(tags[k])})
}
return out
// prefer
return slices.Collect(it.Map(maputil.SortedKeys(tags), func(k string) domain.Tag {
	return domain.Tag{Key: k, Value: lo.FromPtr(tags[k])}
}))
```

Filter + transform:

```go
// avoid
entries := make([]Entry, 0, len(rows))
for _, row := range rows {
	if !match(row) {
		continue
	}
	entries = append(entries, toEntry(row))
}
// prefer
entries := lo.FilterMap(rows, func(row Row, _ int) (Entry, bool) {
	if !match(row) {
		return Entry{}, false
	}
	return toEntry(row), true
})
```

## Note on empty results

`lo.Map`/`lo.Filter` return a non-nil empty slice (`[]`) for empty input, whereas an un-appended `make(..., 0, 0)` or a nil-declared slice marshals to `null`. This is almost always invisible, but if a JSON contract distinguishes `[]` from `null` (as some serializers do), verify the round-trip — see `internal/maputil.Set.MarshalJSON` for a case that guards it explicitly.
