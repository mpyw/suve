# Staging State Transitions

このドキュメントはステージング機能におけるエンティティ（Parameter/Secret）とタグの状態遷移を説明します。

## エンティティ（Parameter/Secret）の状態遷移

### 状態一覧

| 状態 | 説明 |
|------|------|
| `Not Staged` | ステージングされていない状態 |
| `Create` | 新規作成がステージされている状態（AWSに存在しない） |
| `Update` | 更新がステージされている状態（AWSに存在する） |
| `Delete` | 削除がステージされている状態 |

### 状態遷移図

```
                                    ┌─────────────┐
                                    │ Not Staged  │
                                    └──────┬──────┘
                                           │
              ┌────────────────────────────┼────────────────────────────┐
              │                            │                            │
              ▼                            ▼                            ▼
       ┌──────────┐                 ┌──────────┐                 ┌──────────┐
       │  Create  │                 │  Update  │                 │  Delete  │
       │  (add)   │                 │  (edit)  │                 │ (delete) │
       └────┬─────┘                 └────┬─────┘                 └────┬─────┘
            │                            │                            │
            │ delete staged item         │ delete                     │ edit
            │ → Unstaged                 │ → Delete                   │ → Update
            │                            │                            │
            │ re-add (same name)         │ edit (re-edit)             │
            │ → stays Create             │ → stays Update             │
            │   (updates draft)          │   (updates value)          │
            │                            │                            │
            ▼                            ▼                            ▼
       ┌──────────────────────────────────────────────────────────────────┐
       │                    reset / apply                                 │
       │                                                                  │
       │  reset NAME      → Unstaged (removes from staging)               │
       │  reset NAME~N    → Update (restores old version to staging)      │
       │  reset --all     → All Unstaged                                  │
       │  apply           → AWS反映後 Unstaged                             │
       └──────────────────────────────────────────────────────────────────┘
```

### 状態遷移の詳細

#### Not Staged → Create (add)

新規リソースをステージングする際の遷移。

```go
// internal/usecase/staging/add.go
entry := staging.Entry{
    Operation: staging.OperationCreate,
    Value:     lo.ToPtr(input.Value),
    StagedAt:  time.Now(),
}
```

**条件:**
- リソースがAWSに存在しない
- 同名のステージが存在しない、または既にCreateステージがある（上書き）

**禁止:**
- Update/Deleteステージが存在する場合はエラー

#### Not Staged → Update (edit)

既存リソースの変更をステージングする際の遷移。

```go
// internal/usecase/staging/edit.go
entry := staging.Entry{
    Operation:      staging.OperationUpdate,
    Value:          lo.ToPtr(input.Value),
    StagedAt:       time.Now(),
    BaseModifiedAt: baseModifiedAt, // コンフリクト検出用
}
```

**条件:**
- リソースがAWSに存在する
- `BaseModifiedAt`にAWSのLastModifiedを記録（コンフリクト検出用）

#### Not Staged → Delete (delete)

リソース削除をステージングする際の遷移。

```go
// internal/usecase/staging/delete.go
entry := staging.Entry{
    Operation:      staging.OperationDelete,
    StagedAt:       time.Now(),
    BaseModifiedAt: &lastModified,
    DeleteOptions:  deleteOptions, // Secrets Manager用
}
```

**条件:**
- リソースがAWSに存在する

#### Create → Not Staged (delete staged item)

ステージされた新規作成をキャンセルする際の遷移。

```go
// internal/usecase/staging/delete.go
if existingEntry.Operation == staging.OperationCreate {
    // Createステージを削除（AWSには存在しないのでDeleteステージは不要）
    u.Store.UnstageEntry(service, input.Name)
    return &DeleteOutput{Unstaged: true}
}
```

**動作:**
- Deleteステージを作成せず、Createステージを削除

#### Update → Delete (delete)

更新ステージを削除ステージに変更する際の遷移。

**動作:**
- Updateステージを削除ステージで上書き

#### Delete → Update (edit)

削除ステージをキャンセルして更新ステージに変更する際の遷移。

```go
// internal/usecase/staging/edit.go
case staging.OperationDelete:
    // Cancel deletion and convert to UPDATE
    // AWSから現在値を取得して更新ステージを作成
```

**動作:**
- AWSから現在値を取得
- Deleteステージを上書きしてUpdateステージを作成

#### Any → Not Staged (reset)

ステージをリセットする際の遷移。

```go
// internal/usecase/staging/reset.go
u.Store.UnstageEntry(service, name)
```

**バリエーション:**
- `reset NAME`: 指定リソースのステージを削除
- `reset NAME~N`: 過去バージョンの値でUpdateステージを作成
- `reset --all`: 全ステージを削除

#### Any → Applied → Not Staged (apply)

ステージをAWSに適用する際の遷移。

```go
// internal/usecase/staging/apply.go
err := u.Strategy.Apply(ctx, name, entry)
if err == nil {
    u.Store.UnstageEntry(service, name) // 成功したらステージ解除
}
```

**動作:**
1. コンフリクト検出（BaseModifiedAtとAWSのLastModifiedを比較）
2. AWS APIを呼び出し
3. 成功したらステージを削除

---

## タグの状態遷移

タグはエンティティとは独立してステージングされます。

### 状態一覧

| 状態 | 説明 |
|------|------|
| `Not Staged` | タグ変更がステージされていない |
| `Staged` | タグの追加/削除がステージされている |

### タグエントリの構造

```go
// internal/staging/stage.go
type TagEntry struct {
    Add    map[string]string   // 追加・更新するタグ
    Remove maputil.Set[string] // 削除するタグキー
    StagedAt time.Time
    BaseModifiedAt *time.Time  // コンフリクト検出用
}
```

### 状態遷移図

```
                    ┌─────────────┐
                    │ Not Staged  │
                    └──────┬──────┘
                           │
                           │ tag (add or remove)
                           ▼
                    ┌─────────────┐
              ┌────▶│   Staged    │◀────┐
              │     └──────┬──────┘     │
              │            │            │
              │            │            │
         tag  │            │            │ tag
         (追加操作)        │            │ (追加操作)
                           │
                           │ All operations cancel out
                           │ OR reset/apply
                           ▼
                    ┌─────────────┐
                    │ Not Staged  │
                    └─────────────┘
```

### 状態遷移の詳細

#### Not Staged → Staged (tag)

タグ変更をステージングする際の遷移。

```go
// internal/usecase/staging/tag.go
tagEntry = staging.TagEntry{
    StagedAt:       time.Now(),
    Add:            input.AddTags,
    Remove:         input.RemoveTags,
    BaseModifiedAt: &result.LastModified,
}
```

#### Staged → Staged (累積操作)

既存のタグステージに追加操作をマージする際の遷移。

```go
// internal/usecase/staging/tag.go
// 追加タグをマージ
for k, v := range input.AddTags {
    tagEntry.Add[k] = v
    tagEntry.Remove.Remove(k) // Addが優先
}

// 削除タグをマージ
for k := range input.RemoveTags {
    delete(tagEntry.Add, k)
    tagEntry.Remove.Add(k)
}
```

**マージルール:**
- 同じキーに対してAddとRemoveが指定された場合、**最後の操作が勝つ**
- 例: `tag env=prod` → `untag env` → Remove[env]
- 例: `untag env` → `tag env=prod` → Add[env=prod]

#### Staged → Not Staged (相殺)

全ての操作が相殺された場合の遷移。

```go
// internal/usecase/staging/tag.go
if len(tagEntry.Add) == 0 && tagEntry.Remove.Len() == 0 {
    u.Store.UnstageTag(service, name)
}
```

**例:**
1. `tag env=prod` → Staged(Add[env=prod])
2. `untag env` → Not Staged（AddからRemoveへ移動後、空になる）

#### Staged → Not Staged (reset/apply)

リセットまたは適用時の遷移。

---

## コンフリクト検出

### エンティティのコンフリクト

```go
// internal/staging/conflict.go
func CheckConflicts(ctx context.Context, strategy ApplyStrategy, entries map[string]Entry) map[string]struct{} {
    // Create操作: リソースが既に存在する場合はコンフリクト
    // Update/Delete操作: AWS LastModified > BaseModifiedAt の場合はコンフリクト
}
```

| 操作 | コンフリクト条件 |
|------|------------------|
| Create | AWSにリソースが既に存在する（他者が作成済み） |
| Update | AWS LastModified > ステージング時のLastModified |
| Delete | AWS LastModified > ステージング時のLastModified |

### タグのコンフリクト

タグ操作では現在コンフリクト検出は行われません（将来実装予定）。

---

## Secrets Manager固有の状態

### 削除オプション

```go
// internal/staging/stage.go
type DeleteOptions struct {
    Force          bool // 即時完全削除
    RecoveryWindow int  // 復旧期間（7-30日）
}
```

**通常削除:** 指定期間後に完全削除（デフォルト30日）
**強制削除:** 即時完全削除（復旧不可）

---

## 状態遷移マトリクス

### エンティティ操作

| 現在の状態 | add | edit | delete | reset | apply |
|-----------|-----|------|--------|-------|-------|
| Not Staged | Create | Update | Delete | - | - |
| Create | Create (更新) | Create (更新) | Not Staged | Not Staged | AWS反映→Not Staged |
| Update | Error | Update (更新) | Delete | Not Staged | AWS反映→Not Staged |
| Delete | Error | Update | Delete | Not Staged | AWS反映→Not Staged |

### タグ操作

| 現在の状態 | tag add | tag remove | reset | apply |
|-----------|---------|------------|-------|-------|
| Not Staged | Staged | Staged | - | - |
| Staged | Staged (マージ) | Staged (マージ) | Not Staged | AWS反映→Not Staged |
