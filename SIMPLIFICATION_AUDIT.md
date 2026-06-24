# Post-Migration Simplification Audit

TSL v6 migration is done. This audit checks whether hand-rolled logic can now be delegated to TSL v6, GORM, Squirrel, or stdlib.

---

## Verdict Table

| # | Component | Verdict | Summary |
|---|-----------|---------|---------|
| 1 | JSONB path builder in `getField` | **KEEP** | No library covers `->` / `->>` with per-segment validation |
| 2 | Numeric CAST injection in `FieldNameWalk` | **KEEP** | Domain-specific CAST injection; no library equivalent |
| 3 | Condition converters | **KEEP** | `jsonb_path_query_first` generation is entirely custom |
| 4 | `treeWalkForRelatedTables` / `treeWalkForAddingTableName` | **KEEP** | v6 `ident.Walk` re-parses returns; custom `IdentWalk` required |
| 5 | Validation regexes | **KEEP** | Domain-specific allowlists; weakening them for a library is disallowed |
| 6a | `zeroSlice` | **KEEP** | `reflect.MakeSlice` is the only option for dynamic types |
| 6b | `GetTableName` | **KEEP** | Only `Resource` implements `Tabler`; Cluster/NodePool rely on reflection fallback |
| 6c | `newListContext` reflection | **KEEP** | Generics would require full service layer refactor for marginal gain |
| 7 | `addJoins` JOIN builder | **KEEP** | Soft-delete filter (`deleted_time IS NULL`) not covered by GORM joins |
| 8 | v6 walkers we're not using | N/A | Nothing applicable; see details below |
| 9a | `ConditionExpression` struct | **DELETE** | Defined but never instantiated |
| 9b | `startsWithProperties` / `hasProperty` / `propertiesNodeConverter` | **KEEP** | Future code — properties column planned |
| 9c | `statusFieldMappings` | **KEEP** | Handles bare `status.conditions` queries mapping to column name |
| 9d | `NormalizeInSyntax` | **KEEP** | Necessary backward-compat shim; v6 has no config flag for old syntax |

---

## Detailed Verdicts

### 1. JSONB path builder in `getField` — KEEP

**Why the library can't do it**: GORM's `datatypes.JSONQuery` emits `json_extract_path_text()` not `->>`/`->`, breaking expression-index matching. It also lacks per-segment `validateFieldKey()` injection checks and can't output bare SQL strings for TSL tree node replacement. Our builder is 15 lines, injection-safe, and covers the full use case.

### 2. Numeric CAST injection in `FieldNameWalk` — KEEP

**Why the library can't do it**: No TSL v6 walker or GORM feature wraps arbitrary JSONB text extractions in `CAST(... AS numeric)` conditionally based on the comparison RHS type. This is domain-specific logic that inspects `KindNumericLiteral` on the right side — no library has this concept.

### 3. Condition converters — KEEP

**Why the library can't do it**: `jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status'` with `CAST(... AS TIMESTAMPTZ)` wrapping for subfields is entirely custom PostgreSQL JSONB logic. No library generates this. The converters are well-tested and stable.

### 4. `treeWalkForRelatedTables` / `treeWalkForAddingTableName` — KEEP

**Why v6 `ident.Walk` can't replace these**: v6's `ident.Walk` re-parses the check function's return value via `ParseTSL()`. After `FieldNameWalk` runs, identifiers contain JSONB syntax like `spec->>'region'` which fails re-parsing. Our custom `IdentWalk` directly replaces the value without re-parsing — verified safe.

### 5. Validation regexes — KEEP

**Why the library can't do it**: `labelKeyPattern` (`^[a-z0-9_]+$`), `conditionTypePattern` (`^[A-Z][a-zA-Z0-9]*$`), and `validConditionStatuses` are domain-specific allowlists that enforce Kubernetes naming conventions and prevent injection. No library provides these; weakening them to accept broader input is explicitly disallowed.

### 6a. `zeroSlice` — KEEP

**Why there's no simpler pattern**: The function accepts `interface{}` (unknown slice type at compile time). `reflect.MakeSlice` is the only way to create a typed zero-length slice dynamically. Go generics would eliminate this, but require refactoring the entire `List()` API from `interface{}` to `List[T any]()` — architectural change, not a simplification.

### 6b. `GetTableName` — KEEP

**Why `Tabler` interface alone isn't enough**: Only `Resource` implements `TableName() string`. `Cluster`, `NodePool`, and other models rely on GORM's default naming (reflection → `inflection.Plural(strings.ToLower(name))`). `GetTableName` must handle both paths. Making all models implement `Tabler` would be cleaner but is a separate initiative, not a simplification.

### 6c. `newListContext` reflection — KEEP

**Why generics aren't a drop-in**: Current signature `List(ctx, args, &[]api.Cluster{})` is polymorphic at the call site. Generic version `List[api.Cluster](ctx, args)` would require changing every caller, the `GenericService` interface, the mock generation, and the DAO layer. The reflection code is 5 lines, localized, and correct.

### 7. `addJoins` — KEEP

**Why GORM's join API doesn't cover it**: The manual SQL includes `AND %s.deleted_time IS NULL` for soft-delete filtering on the joined table. GORM's `Joins()` with relationship metadata doesn't inject this condition. GORM's soft-delete plugin only applies to the primary model, not joined tables. The manual GROUP BY is also needed to deduplicate rows from one-to-many joins in search results.

### 8. v6 walkers we're not using

v6 provides 4 walkers:

| Walker | Purpose | Applicable? |
|--------|---------|-------------|
| `ident.Walk` | Replace identifiers (re-parses return) | No — our `IdentWalk` is safer for JSONB |
| `sql.Walk` | TSL → Squirrel SQL | Yes — already used in `treeWalkForSqlizer` |
| `semantics.Walk` | In-memory record evaluation | No — we do SQL, not in-memory filtering |
| `graphviz.Walk` | AST → dot file visualization | No — debugging only |

**No v6 walker subsumes any of our custom walkers.** All our custom walkers do domain-specific transforms (JSONB path building, CAST injection, condition extraction) that v6 has no concept of.

---

## Dead Code — DELETE

### 9a. `ConditionExpression` struct

```go
// pkg/db/sql_helpers.go:368-371
type ConditionExpression struct {
	Expr sq.Sqlizer
}
```

Defined but **never instantiated**. `ExtractConditionQueries` returns `[]sq.Sqlizer` directly. This struct was likely from an earlier design iteration. Zero references outside its definition.

### 9b. Properties pattern — KEEP

`startsWithProperties`, `hasProperty`, `propertiesNodeConverter` — future code for a planned `properties` column. Not currently exercised but intentionally scaffolded.

### Diff sketch for dead code removal:

```diff
--- a/pkg/db/sql_helpers.go
+++ b/pkg/db/sql_helpers.go
-type ConditionExpression struct {
-	Expr sq.Sqlizer
-}
```

---

## Roundabout Library Usage — None Found

No cases where we call a library in a roundabout way that a newer API covers. The v6 import paths are direct, Squirrel usage is idiomatic, GORM usage matches v2 patterns.

---

## Summary

| Action | Count | Items |
|--------|-------|-------|
| **KEEP** | 11 | All hand-rolled logic except dead code |
| **DELETE** | 1 | `ConditionExpression` struct |
| **REPLACE** | 0 | — |
| **UNSURE** | 0 | — |

The codebase is already lean for what it does. The hand-rolled logic exists because the libraries genuinely don't cover the use cases (JSONB path building with injection validation, conditional CAST wrapping, condition extraction with jsonb_path_query_first). The only deletable code is vestigial dead code from features that were never fully adopted (`properties` pattern) or type definitions that were superseded (`ConditionExpression`).
