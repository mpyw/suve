---
description: Adopt declscope on an existing Go codebase and drive its diagnostics to zero. Read this when introducing declscope to a repository, when clearing a declscope baseline, or when a declscope diagnostic is hard to act on. Covers reading the diagnostics as structure, the remedy for each shape, and the measurement traps that produce false confidence.
license: MIT
metadata:
    github-path: skills/declscope-adoption
    github-ref: refs/tags/v0.3.2
    github-repo: https://github.com/mpyw/declscope
    github-tree-sha: 3291467765b4ce0c2447be0073dc123e7dc29941
name: declscope-adoption
---
# Adopting declscope

Written against **declscope 0.3.2**. Check the version first, since one behaviour described here changed in 0.3.0.

```bash
declscope -V=full
```

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

That last row is worth its own note. In one repository, splitting `statements.go` into `query.go`, `exec.go` and `bind.go` cleared every entry **without renaming a single declaration**. The file name was the thing that was wrong.

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

**A dirty working tree poisons a comparison.** Measuring option A, then option B without reverting, measures A and B together. `git stash` leaves untracked files behind, so a new file from the previous attempt stays. Copy the tree instead:

```bash
cp -r repo /tmp/try-a   # and measure there
```

**A bulk rename reaches further than intended.** A `\bname\b` substitution across every `.go` file will hit `keys`, `named` and `check`. Those live in testdata and in unrelated packages too. Limit the paths, then read `git status` to see what actually changed.

**A file created to satisfy a name is often a file too small to exist.** One rename produced a 23-line file holding `Build`. It returned a type declared in the file next to it, and `BuildProgram` in that file was the answer. Before adding a file, ask whether renaming the declaration would do.

**`//declscope:namespace` goes before the package clause.** Placed after it, the directive is silently inert and the diagnostics do not move. If a change makes no difference at all, check the placement first.

## Known gaps

[#64](https://github.com/mpyw/declscope/issues/64) is open. Inflections are generated only in the lengthening direction, so a `storing.go` is never carried by `store*`. The vocabulary entry above covers it in one line.

Fixed in 0.3.0: a `doc.go` holding only a package comment used to count toward the namespace count and turn `ondemand` on. Nine packages in one repository reported for that reason alone. On 0.3.0 those reports are gone, and any `//declscope:core` written to work around it can come out.

## Order of work

1. Group the diagnostics by rule and namespace
2. Clear `boundary` by moving the boundary, not by widening everything
3. Re-measure. Naming often falls with it, since merging two namespaces into one takes `ondemand` out of force
4. Fix the file names that do not match their contents
5. Rename what is left, in natural word order
6. Delete the baseline
7. Check the core count, and `go build`, `go test` and `declscope` in that order
