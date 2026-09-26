---
name: declscope-adoption
description: Adopt declscope on an existing Go codebase and drive its diagnostics to zero. Read this when introducing declscope to a repository, when clearing a declscope baseline, or when a declscope diagnostic is hard to act on. Covers reading the diagnostics as structure, the remedy for each shape, and the measurement traps that produce false confidence.
license: MIT
x-embedded-by: declscope
x-embedded-version: 0.14.0
x-embedded-at: "2026-09-26T11:57:01Z"
x-embedded-digest: "sha256:388a62bcce15dbe409b6dfffc830d7493070df24e53a02ee962efbc2074927fe"
---

# Adopting declscope

Written against **declscope 0.14.0**. Check the version first: this describes how that release behaves, not how an older one does.

```bash
declscope -V=full
```

**Read [the README](https://github.com/mpyw/declscope#readme) before the first decision.** This skill covers what to do about the diagnostics. What each directive means, and what the config accepts, is there.

## Check what is switched on

The naming rule is off unless the repository asks for it. A count of zero may mean the code is clean, or it may mean nothing is being checked.

| Setting | Default | |
| --- | --- | --- |
| `rules.naming.qualify` | `never` | The naming rule is **off** |
| `rules.naming.exported` | `false` | Even when on, it skips exported declarations |
| `rules.surplus` | `strict` | The surplus rule is **on** and judges each declaration a directive widens. `loose` judges a directive as a whole; `off` turns it off |
| `rules.boundary` | `on` | The boundary rule is **on**. `off` stops checking reach; set `surplus: off` alongside it |
| `rules.unused` | `strict` | The unused rule is **on**. It reports an ignore that silenced nothing, and a scope directive that restates the scope in force. `loose` reports a scope directive only when no configuration could make it bind; `off` turns it off. Malformed directives are the `directive` rule's, which is always on |
| `filter.only` | None | When set anywhere in the chain, files outside it are never read |

The config is looked up from each analyzed package's directory **upwards**, so a subtree can carry its own and a repository can have several. Find them all, and do not read the root alone:

```bash
declscope survey ./...   # Checks in force: one row per config chain, with what it switched on
```

That answers it directly. To read the files themselves:

```bash
find . -name '.declscope.y*ml' -not -path './.git/*' \
  -exec sh -c 'echo "== $1"; cat "$1"' _ {} \;
```

Finding none means the naming rule is off everywhere, and `boundary`, `surplus` and `unused` are on. Finding one is not the answer on its own. A config that never sets `qualify` leaves that rule off, and one that sets `boundary: off` leaves off the rule this tool exists for.

**The files compose, so the nearest one does not tell you what applies.** Every file between the package and the module root is read, outermost first. A nearer file owns the keys it states and inherits the rest.

`filter` is the exception to that: `only` intersects down the chain and `omit` unions, so a config file can only ever shrink what is read. A root `omit` holds everywhere below it, and no nested file undoes it.

Each file's patterns are read against **its own** directory, and anchor there when they hold a separator. `gen/**` in the root and the same line in a nested file name different directories. A bare name and a leading `**/` float instead, and a `..` in a pattern is an error.

### Start at the default, and offer the rest

**The configuration is the repository owner's decision, not yours.** Ask, and wait for an answer, before writing a config file or changing any code.

**The minimum is no config file at all.** `boundary`, `surplus: strict` and `unused: strict` are on. They check reach and whether directives still change anything; the naming rule checks a convention. Adopting this much is a complete adoption.

**Size the default `surplus` findings before changing code.** `loose` judges a `//declscope:package` as a whole, so one reached declaration keeps the whole directive quiet. The default `strict` also reports each declaration the directive widens for nothing, and one `declscope -fix` run inserts every `//declscope:private` it asks for. If the owner wants the narrower check, set `surplus: loose` explicitly.

```bash
declscope survey -format=json ./... | jq .totals.surplus
```

An explicit `-config` replaces the repository's own config. Copy its keys in first when measuring a different mode.

**Size the default `unused` findings too.** Under `loose`, a `//declscope:private` that names the scope `defaults.unexported` gives is kept, since another default could make it bind. The default `strict` reports it, and one `declscope -fix` run deletes it. When changing `defaults.unexported`, expect reports for directives that restate the new default. If the owner wants to keep those explicit decisions, set `unused: loose`.

**If the goal is a tidier codebase, offer the naming rule on top.** It is a convention. It fires where nothing is wrong, and it costs real work. Size it before offering, with a throwaway config rather than by counting message fragments:

```bash
printf 'rules:\n  naming:\n    qualify: ondemand\n    exported: true\n' > /tmp/q.yaml
declscope survey -config /tmp/q.yaml -format=json ./... | jq .totals.qualify
```


```yaml
rules:
  naming:
    qualify: ondemand   # ask once a package holds a second namespace
    exported: true      # reach exported names too, since inside the package they read as bare
```

| | Measured |
| --- | --- |
| Default, no config | Measure in the repository being adopted |
| `qualify: ondemand` | 89 in one repository whose boundary count was zero |
| plus `exported: true` | 1008 in a large one |
| `qualify: always` | More again, including single-unit packages where a prefix distinguishes nothing |

Put the numbers in front of the person deciding, rather than describing the settings:

```bash
for q in never ondemand always; do
  printf 'rules:\n  naming:\n    qualify: %s\n    exported: true\n' "$q" > /tmp/q.yaml
  printf '%-9s %s\n' "$q" "$(declscope -config /tmp/q.yaml ./... 2>&1 | grep -c 'does not carry')"
done
```

Every count in the rest of this skill assumes `qualify: ondemand` with `exported: true`. That is what the numbers were taken under, not a recommendation.

## Shrink the exported surface first

**`declscope shrink` reports the exported declarations of `internal/` packages that nothing outside their package uses.** With `-fix` it unexports them. The analyzer cannot answer this: it reads one package, and any importer might use an exported name. Inside `internal/`, Go limits the importers to one directory tree, so `shrink` loads the whole module and sees every one of them.

An exported name inside `internal/` claims that another package depends on it. Where nothing does, the claim is false, and it hides the declaration from the rest of declscope. An exported declaration takes package scope by default, so no boundary is ever reported on it. Unexported, it takes `private`, and the analyzer checks who reaches it.

**It is a subcommand, not a rule the analyzer runs.** `go vet` and golangci-lint never report it, and it has no config key. A clean `declscope ./...` says nothing about it.

### Run it before the analyzer

**Run `shrink`, and apply its fixes, before you work on the analyzer's reports.** A declaration it unexports becomes private to its namespace. Wherever another file of the package uses it, the analyzer then reports a boundary crossing that was not there before. Fixing in the other order means a second round.

This happened when declscope held itself to `shrink`. The fix unexported twelve declarations, and nine of them were used from other files of their package. The analyzer then needed nine `//declscope:package` directives to state those crossings.

| Step | Command |
| --- | --- |
| 1. Read what `shrink` reports | `declscope shrink ./...` |
| 2. Unexport, once the owner agrees | `declscope shrink -fix ./...` |
| 3. Confirm the build | `go vet ./...` and `go test ./...` |
| 4. Read what the analyzer now reports | `declscope ./...` |
| 5. State or move each new crossing | See [What each shape means](#what-each-shape-means) |

Ask before step 2, the same as any other change. The fix renames every identifier naming the declaration, all inside its own package, and the doc comment that opens with the name.

### Reading what it reports

| Report | What to do |
| --- | --- |
| `... uses it` and nothing more | The fix is offered. Apply it with `-fix` |
| `... (no fix: <reason>)` | A use may exist that `shrink` cannot prove, or the rename is unsafe. **Do not unexport it by hand.** Read the reason first |
| `... only the external tests of <pkg> use it` | Keep it exported. Add `//declscope:ignore overexported // <why>` when the tests use it on purpose |
| `declscope shrink: not judged: <pkg>: <reason>` on stderr | That package was not checked. It is not clean |

**A package not judged is not a package with nothing to report.**

`shrink` stands down wherever an importer could be unseen. That is outside `internal/`, in `package main`, and beside assembly or cgo. It is also under an `internal/` that a nested module's path extends. The stderr line names each such package, and the exit status ignores it.

Silence a report with `//declscope:ignore overexported` and a reason. A bare `//declscope:ignore` does not reach this rule. `shrink` reports an ignore that silenced nothing, as the analyzer does for its own.

**Deleting unused code is not `shrink`'s job.** Once a declaration is unexported, staticcheck's `unused` and gopls' `unusedfunc` report it when nothing uses it. Run them after `shrink`, not before.

### Keep it in CI

Run it before the analyzer there too, so that a failure reads in the order it is fixed. It exits 3 when it reports anything.

```yaml
- run: declscope shrink ./...
- run: declscope ./...
```

**Names written as strings are outside what `shrink` can see.**

A template can name a field, and a constant can go to `reflect.Value.MethodByName`. A script can read the symbol table. Each uses a declaration by name. When the value reaches them through an interface, `shrink` already treats it as used. When it does not, add the ignore with the reason.

## The two kinds of report

declscope reports two things. **Read them separately.**

| Rule | What it means |
| --- | --- |
| `boundary` | A file reaches a declaration another file holds. A property of the code |
| `qualify` | A name does not carry its file's namespace. A convention |
| `overexported` | An exported name inside `internal/` that nothing outside its package uses. Only `declscope shrink` reports it, and it goes [first](#shrink-the-exported-surface-first) |

Boundary first. It is the one that points at structure.

## Measure before deciding

A count is not a work list. Group it first — with two commands, not with `grep`.

```bash
declscope survey -format=json ./...           # which package to open first
declscope inspect -format=json <that package> # what shape it is in
```

`-format` takes `markdown` (the default, readable in a terminal and paste-ready) or `json`. Read the JSON.

Add `-test=false` once the first pass is read. In a large package most of what crosses is scaffolding — `export_test.go` reaching internals is what that file is for — and it outranks the crossings worth acting on.

**`survey` refuses to print a count it cannot stand behind.** It stops on a package that does not type-check, and it reports what was in force before anything else: which config governed which packages, whether each rule was on, and how many entries a baseline holds. A rule that was not asked prints `-`, never `0` — including a rule that stood itself down, as `surplus` does for a package holding assembly, cgo or a build-excluded file.

`-allow-errors` continues past a package that does not compile. It is named under `type check` and given no row, so nothing in the tables reads as a clean result for it.

That removes three steps this skill used to require. Do **not** move the baseline aside to measure: `survey` reports `baselined` as its own column, so what is suppressed and what is left are visible at once. Do not count message fragments either; the wording of a diagnostic is not an interface, and the JSON is.

| What you need | Where it is |
| --- | --- |
| Is anything even being checked | `checks.configs[].rules`, `checks.typeCheck` |
| Which package to open | `packages[]`, already sorted; the first row is the heaviest |
| Is this deferred or decided | `boundary.baselined` against `boundary.declared` |
| Which two namespaces to merge | `inspect`'s `crossings[]`, one row per ordered pair, with `clears` |
| Where the structure is | `inspect`'s `edges[]`, one row per declaration **and reaching namespace** |
| Where a name is wrong | `inspect`'s `names[]`, with `fixable` saying whether `-fix` would rename it |

**`clears` is the number the decision turns on.** A crossing's `reached` says how much of a namespace it touches; `clears` says how many findings would go away if the two became one namespace, which is less whenever a third namespace reaches the same declaration. Rank the work by `clears`, not by `reached` or `uses`.

**`edges[]` does not count findings.** One declaration reached from three namespaces is three rows and one finding, so the rows always outnumber `findings.boundary.reported`. Count distinct `declaration` values, or read `crossings[]`, or read the tally.

A crossing's `state` is one of six. Three are outcomes of a finding — `reported`, `baselined`, `ignored` — and three say why there was no finding: `declared` (a `//declscope:package` says it is shared), `open` (package-scoped because nothing says otherwise, which is most exported API), and `unchecked` (`rules.boundary` is `off`).

**A package with nothing reported, much baselined and nothing declared has never been decided about.** It reads as clean under the analyzer alone, which is why `declared` is a column.

Boundary violations cluster. Measured across eight repositories, one structural decision cleared between 10 and 100 entries every time. In one repository 34 of 35 sat in a single namespace.

**Start where the count is concentrated, not where it is large.** The row order does not give you that: rows are sorted by how much is undecided, which is size. Concentration is the `largest crossing` column — a package with 12 findings spread over 6 namespaces sorts above one with 4 in a single crossing, and the second is the one where one decision clears the cluster.

### Reading a saturation

**This needs the naming rule switched on.** At the configuration to start from it is off, so `qualifyTargets` is `0`, `names[]` is empty and every ratio prints `-`. Measure it with the throwaway config above before reading any of what follows.

`inspect` reports, per namespace, how many of the declarations the naming rule examines there fail it. The ratio says which thing is wrong, and the answer is rarely the rename the diagnostic suggests.

| Saturation | What is wrong | The answer |
| --- | --- | --- |
| Nearly all of them | The **namespace name** | `//declscope:namespace`, or rename the file |
| Around half | One file holding several concerns | Split the file |
| One or two | Those declarations | Rename them |

A baselined finding counts toward it: the baseline defers a decision rather than settling it, so regenerating one moves this number without a line of code changing.

## What each shape means

| The diagnostics say | The code is | Do this |
| --- | --- | --- |
| File A's declarations are nearly all used from B, and nothing goes back | A is B's working parts, not a layer under it | `//declscope:namespace <B>` on A |
| A and B reference each other both ways | One device split across two files | Give both the same namespace and rename to say so |
| A type's fields are read from four files | One type filed by concern | The files share a namespace, or join the core |
| Calls run one way through three files | A pipeline, and the layers are real | Keep the files. Declare only what crosses, with the reason |
| `pkg.Foo` is asked to become `pkg.PkgFoo` | The file is the package's API | `//declscope:core` |
| One helper is used from several files | Shared on purpose | `//declscope:package // why` at the declaration |
| A name reads badly with its namespace in it | Often the file name, not the declaration | Rename the file |

**Two rows can fire on one cluster.** A mutual pair whose declarations are also read from four other namespaces matches both the second row and the third. Take the one with the larger `clears`: merging two namespaces settles only what no third namespace reaches, so the fan-in case is usually the smaller change and the core case the larger.

That last row is worth its own note. In one repository a single file held three concerns, and splitting it into three cleared every entry in that cluster **without renaming a single declaration**. The file name was the thing that was wrong.

## Shared structs: private fields

When other namespaces use a struct, give each field the smallest scope it needs. The type states the widest one, and each field narrows it only where it can.

| Declaration | Directive |
| --- | --- |
| The struct type, spelled from another namespace | `//declscope:package`. Its fields inherit it |
| A field no other namespace reads | `//declscope:private`, after its doc comment and a `//` line |
| A field another namespace reads | None |
| An embedded field | None. It is not a target |

**Do not restate the type's scope on a field.** A `//declscope:package` on a field under a `//declscope:package` type binds nothing. The same goes for any directive on an embedded field. Both are reported:

```text
unused //declscope:package on callee.shared: nothing it reaches takes a scope
unused //declscope:private: no checked declaration carries it
```

**Put the private fields last.** The fields other stages read are the struct's interface, so they come first:

```go
// callee is a resolved call target.
//
//declscope:package
type callee struct {
	// obj is the declared function or method, when there is one.
	obj *types.Func
	// inputs are the values passed.
	inputs []ssa.Value
	// builtin is set for a call to a builtin function.
	builtin *ssa.Builtin
	// fn is the function called, when it is known statically.
	//
	//declscope:private
	fn *ssa.Function
}
```

> [!WARNING]
> Do not reorder fields where the order is observable. Add the directives in place instead.
>
> | Order is observable through | |
> | --- | --- |
> | Unkeyed composite literals | `callee{f, in, b, fn}` binds by position |
> | Positional or binary encodings | The wire format follows the field order |
> | `unsafe` offsets | `unsafe.Offsetof` changes |
> | 64-bit atomics | They rely on first-word alignment on 32-bit platforms |

To find which fields cross, let declscope tell you:

1. Mark every field `//declscope:private`
2. Run declscope
3. Remove the directive from each field it reports as `declared private by //declscope:private, but is used from namespace ...`
4. Move the fields that kept it to the bottom

> [!TIP]
> Under `rules.surplus: strict`, declscope reports these fields itself. It also reports each declaration under a file-level `//declscope:package` that no other namespace uses. One `declscope -fix` run inserts every `//declscope:private`. Step 4 stays manual, because the fix never moves a field.

**A type nobody else spells needs no directive.** Sometimes callers only get it from a constructor, and never spell its name or its fields. Then `//declscope:package` on the type is reported as surplus:

```text
//declscope:package on hidden, hidden.a: no use from another namespace is visible to declscope
```

Keep that type and all its fields private. Expose small package-scoped functions or methods that return what the callers need.

Do not reach for `//declscope:core` or a file-level `//declscope:package` to quiet these reports. Both hide the boundaries instead of stating them.

## Naming

The namespace may sit anywhere in the name and the right edge may fall inside a word. A prefix is one answer, not the answer.

| Namespace | Carried by |
| --- | --- |
| `collect` | `collectFiles`, `addFuncToCollection`, `parseCollectedDecl` |
| `parse` | `SpecifierParser` |
| `store` | `storing`, `stored` |

Prefer natural word order. A verb namespace makes a prefix read as an instruction: `collectAddFunc` is a command, `addFuncToCollection` is a name. Entry points are the exception, since `collectFiles` already reads as what it does.

When the namespace is an inflected form, the stem is not derived from it. A `tracing.go` declaring `trace*` needs one line:

```yaml
rules:
  naming:
    vocabulary:
      tracing: [trace]
```

## Do not turn the check off

**Never set `rules.boundary: off` to reach zero.** It silences the rule this tool exists for, and every count after it is meaningless. It is the repository owner's choice, for a repository that wants the ownership mark in a name without the scope behind it. It is never a step in an adoption. A baseline is one, because it records what the code already has and still reports what is new. Ask before writing it, the same as any other config change, and never propose it as a way past a diagnostic you could not resolve.

`//declscope:core` exempts a file from the naming rule and merges it into one namespace. Marking every file in a package core means declscope checks nothing there.

**Count it.** A package where every file is core needs a reason you can state in one sentence. There should be few of them.

```bash
for d in $(find . -name '*.go' -not -name '*_test.go' | xargs -n1 dirname | sort -u); do
  n=$(ls $d/*.go 2>/dev/null | grep -vc _test)
  c=$(grep -l "declscope:core" $d/*.go 2>/dev/null | grep -vc _test)
  [ "$n" = "$c" ] && [ "$n" != "0" ] && echo "all core: $d"
done
```

Two of nine repositories reached zero without using `core` at all. One solved its worst cluster by promoting a file to its own package, because the file already documented itself as temporary and had one caller. A package boundary was the honest answer, and no directive could have said it.

## A baseline is for arriving, not for staying

`declscope baseline ./...` records what exists so that new code is held to the rule. It suppresses and does not endorse.

If the goal is zero, delete the file rather than regenerating it. An empty baseline left in the tree says nothing.

## Traps

These cost real time. Each was measured, not guessed.

**A failed build reports zero diagnostics.** It looks exactly like success. `declscope survey` refuses instead of printing such a count, naming the packages that did not compile, so measure through it:

```bash
declscope survey ./...              # refuses on a package that does not type-check
go build ./... && declscope ./...   # never read a bare count without this
```

**A zero may be the filter, not the code.** A `filter.only` anywhere in the chain can leave a package with nothing to read. A package nothing was read from reports nothing. `declscope` says so only when a nested `only` was cancelled by one above it, so the quiet cases stay quiet. `declscope inspect` lists the files each namespace was built from (`namespaces[].files`); a package whose files are missing from it is one the filter removed.

**A clean analyzer says nothing about `shrink`.** The analyzer never reports `overexported`, and `shrink` never reports what the analyzer does. Run both, `shrink` first.

**A zero from `boundary` may be the switch, not the code.** `rules.boundary: off` silences the rule entirely, and the run looks like a clean repository. Read every config before reporting a count, the same way you would for `qualify`.

**`-fix` widens; it does not draw boundaries.** On a codebase with boundary findings, `declscope -fix ./...` inserts `//declscope:package` above every crossed declaration — the wholesale widening step 2 of the order of work exists to avoid. Run `-fix -diff` first and read it. Its place in an adoption is renaming, after the structure is settled, and only where `names[].fixable` is true.

**A dirty working tree poisons a comparison.** Measuring option A, then option B without reverting, measures A and B together. `git stash` leaves untracked files behind, so a new file from the previous attempt stays. Copy the tree instead:

```bash
cp -r repo /tmp/try-a   # and measure there
```

**A bulk rename reaches further than intended.** A `\bname\b` substitution across every `.go` file will hit `keys`, `named` and `check`. Those live in testdata and in unrelated packages too. Limit the paths, then read `git status` to see what actually changed.

**A file created to satisfy a name is often a file too small to exist.** One rename produced a file of about twenty lines holding one constructor. It returned a type declared in the file beside it, and renaming the constructor where it already was turned out to be the answer. Before adding a file, ask whether renaming the declaration would do.

**`//declscope:namespace` goes before the package clause.** Placed after it, the directive is silently inert and the diagnostics do not move. If a change makes no difference at all, check the placement first.

## Known gaps

[#64](https://github.com/mpyw/declscope/issues/64) is open. Inflections are generated only in the lengthening direction, so a `storing.go` is never carried by `store*`. The vocabulary entry above covers it in one line.

## Order of work

1. `declscope survey ./...`, and read Checks in force before any count
2. If the repository has `internal/` packages, run `declscope shrink ./...` and settle it before anything else. Its fixes add boundary reports, and nothing that follows adds reports back
3. Clear `boundary` by moving the boundary, not by widening everything
4. Re-measure with `survey`. Naming often falls with it, since merging two namespaces into one takes `ondemand` out of force
5. Fix the file names that do not match their contents
6. Rename what is left, in natural word order
7. Delete the baseline
8. Check the core count, and `go build`, `go test`, `declscope shrink` and `declscope` in that order
