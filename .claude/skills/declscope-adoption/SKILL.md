---
description: Adopt declscope on an existing Go codebase and drive its diagnostics to zero. Read this when introducing declscope to a repository, when clearing a declscope baseline, or when a declscope diagnostic is hard to act on. Covers reading the diagnostics as structure, the remedy for each shape, and the measurement traps that produce false confidence.
license: MIT
metadata:
    github-path: skills/declscope-adoption
    github-ref: refs/tags/v0.6.0
    github-repo: https://github.com/mpyw/declscope
    github-tree-sha: 44a6bafb66c3ae73dae7abfea78d0804b3f672c6
name: declscope-adoption
---
# Adopting declscope

Written against **declscope 0.6.0**. Check the version first: this describes how that release behaves, not how an older one does.

```bash
declscope -V=full
```

**Read [the README](https://github.com/mpyw/declscope#readme) before the first decision.** This skill covers what to do about the diagnostics. What each directive means, and what the config accepts, is there.

## Check what is switched on

Two of the three rules are off unless the repository asks for them. A count of zero may mean the code is clean, or it may mean nothing is being checked.

| Setting | Default | |
| --- | --- | --- |
| `rules.naming.qualify` | `never` | The naming rule is **off** |
| `rules.naming.exported` | `false` | Even when on, it skips exported declarations |
| `rules.allowSurplus` | `false` | The surplus rule is **on** |
| `rules.allowBoundary` | `false` | The boundary rule is **on**. Set, it leaves only the naming rule |
| `filter.only` | None | When set anywhere in the chain, files outside it are never read |

The config is looked up from each analyzed package's directory **upwards**, so a subtree can carry its own and a repository can have several. Find them all, and do not read the root alone:

```bash
find . -name '.declscope.y*ml' -not -path './.git/*' \
  -exec sh -c 'echo "== $1"; cat "$1"' _ {} \;
```

Finding none means the naming rule is off everywhere, and the other two are on. Finding one is not the answer on its own. A config that never sets `qualify` leaves that rule off, and one that sets `allowBoundary` leaves off the rule this tool exists for.

**The files compose, so the nearest one does not tell you what applies.** Every file between the package and the module root is read, outermost first. A nearer file owns the keys it states and inherits the rest.

`filter` is the exception to that: `only` intersects down the chain and `omit` unions, so a config file can only ever shrink what is read. A root `omit` holds everywhere below it, and no nested file undoes it.

Each file's patterns are read against **its own** directory, and anchor there when they hold a separator. `gen/**` in the root and the same line in a nested file name different directories. A bare name and a leading `**/` float instead, and a `..` in a pattern is an error.

### Start at the default, and offer the rest

**The configuration is the repository owner's decision, not yours.** Ask, and wait for an answer, before writing a config file or changing any code.

**The minimum is no config file at all.** `boundary` and `surplus` are on. Those two answer a question about the code, where the naming rule answers one about a convention. Most repositories report a handful. Adopting this much is a complete adoption.

**If the goal is a tidier codebase, offer the naming rule on top.** It is a convention. It fires where nothing is wrong, and it costs real work:

```yaml
rules:
  naming:
    qualify: ondemand   # ask once a package holds a second namespace
    exported: true      # reach exported names too, since inside the package they read as bare
```

| | Measured |
| --- | --- |
| Default, no config | A handful in most repositories |
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

## The two kinds of report

declscope reports two things. **Read them separately.**

| Rule | What it means |
| --- | --- |
| `boundary` | A file reaches a declaration another file holds. A property of the code |
| `qualify` | A name does not carry its file's namespace. A convention |

Boundary first. It is the one that points at structure.

## Measure before deciding

A count is not a work list. Group it first.

A baseline suppresses everything it holds, so move it aside before measuring. Otherwise every command below reports zero and the codebase looks clean.

```bash
mv .declscope-baseline.yaml /tmp/bl.bak     # put it back, or delete it, when done

declscope ./... 2>&1 | grep -c "is private to\|is declared private"   # boundary
declscope ./... 2>&1 | grep -c "does not carry"                       # naming
declscope ./... 2>&1 | grep "is private" \
  | sed -E 's/.*namespace "([^"]+)".*/\1/' | sort | uniq -c | sort -rn
```

Boundary violations cluster. Measured across eight repositories, one structural decision cleared between 10 and 100 entries every time. In one repository 34 of 35 sat in a single namespace.

**Start where the count is concentrated, not where it is large.**

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

That last row is worth its own note. In one repository a single file held three concerns, and splitting it into three cleared every entry in that cluster **without renaming a single declaration**. The file name was the thing that was wrong.

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

**Never set `rules.allowBoundary` to reach zero.** It silences the rule this tool exists for, and every count after it is meaningless. It is the repository owner's choice, for a repository that wants the ownership mark in a name without the scope behind it. It is never a step in an adoption. A baseline is one, because it records what the code already has and still reports what is new. Ask before writing it, the same as any other config change, and never propose it as a way past a diagnostic you could not resolve.

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

**A failed build reports zero diagnostics.** It looks exactly like success. Check `go build ./...` before reading any count.

```bash
go build ./... && declscope ./...   # never read the count without this
```

**A zero may be the filter, not the code.** A `filter.only` anywhere in the chain can leave a package with nothing to read. A package nothing was read from reports nothing. `declscope` says so only when a nested `only` was cancelled by one above it, so the quiet cases stay quiet. Count the files the analysis actually saw before trusting a zero.

**A zero from `boundary` may be the switch, not the code.** `rules.allowBoundary: true` silences the rule entirely, and the run looks like a clean repository. Read every config before reporting a count, the same way you would for `qualify`.

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

1. Group the diagnostics by rule and namespace
2. Clear `boundary` by moving the boundary, not by widening everything
3. Re-measure. Naming often falls with it, since merging two namespaces into one takes `ondemand` out of force
4. Fix the file names that do not match their contents
5. Rename what is left, in natural word order
6. Delete the baseline
7. Check the core count, and `go build`, `go test` and `declscope` in that order
