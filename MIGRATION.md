# TSL v0 → v6.0.10 Migration

## Version

- **From**: `github.com/yaacov/tree-search-language v0.0.0-20190923184055-1c2dad2e354b` (ANTLR-based, Sept 2019)
- **To**: `github.com/yaacov/tree-search-language/v6 v6.0.10` (goyacc-based, 2025)

## Motivation

The old TSL parser limited identifiers to 3 dot-separated parts (`database.table.column`). Our codebase worked around this by encoding deep paths as `__` before parsing and decoding them afterward. v6 supports unlimited dot-separated identifiers natively, making the entire workaround layer deletable.

## Breaking Changes Hit and How Fixed

### 1. Node struct completely changed

| Old | v6 |
|---|---|
| `tsl.Node{Func string, Left interface{}, Right interface{}}` | `*tsl.Node{Kind Kind, Value interface{}, Operator Operator, Left *Node, Right *Node, Children []*Node}` |

**Fix**: Rewrote all ~30 functions and ~40 test cases that construct or inspect TSL nodes.

### 2. Operators changed from strings to int enums

`tsl.EqOp` ("$eq") → `tsl.OpEQ` (288), etc.

**Fix**: Updated `comparisonOperators` map and all operator references.

### 3. `ident.Walk` re-parses return values

v6's `ident.Walk` calls `tsl.ParseTSL()` on the check function's return string. Our field mapping produces JSONB expressions like `spec->>'region'` which fail to re-parse.

**Fix**: Wrote a custom `IdentWalk()` that directly replaces identifier values without re-parsing. Removed the `ident` import entirely.

### 4. IN syntax changed from `()` to `[]`

Old: `id in ('a', 'b')`. v6: `id in ['a', 'b']`.

**Fix**: Added `NormalizeInSyntax()` preprocessor for backward compatibility. Converts `in (...)` to `in [...]` in unquoted context before parsing.

### 5. NOT operator binding changed

Old: `NOT x = 'y'` → NOT wraps entire comparison. v6: `NOT x = 'y'` → NOT binds only to `x` (like unary minus). Users must write `NOT (x = 'y')` for the old behavior.

**Fix**: Updated tests to use parenthesized `NOT (...)` syntax.

### 6. RFC3339 timestamps parsed as `time.Time`

Old: `'2026-01-01T00:00:00Z'` → `KindStringLiteral` with string value. v6: → `KindTimestampLiteral` with `time.Time` value.

**Fix**: Updated `conditionSubfieldConverter` to handle both `KindStringLiteral` and `KindTimestampLiteral`, converting `time.Time` back to RFC3339 string for SQL binding.

### 7. Bare identifiers are valid expressions

Old: `garbage` → parse error. v6: `garbage` → valid `KindIdentifier` node.

**Fix**: Updated error test to use truly unparseable input (`= = =`).

## Before/After SQL Table

Generated SQL is **identical** for all cases. Only the intermediate representation changed.

| Input | SQL Output (both versions) | Args |
|---|---|---|
| `spec.region = 'us-east'` | `spec->>'region' = ?` | `[us-east]` |
| `spec.release.channel = 'stable'` | `spec->'release'->>'channel' = ?` | `[stable]` |
| `spec.a.b.c = 'x'` | `spec->'a'->'b'->>'c' = ?` | `[x]` |
| `spec.replicas > 9` | `CAST(spec->>'replicas' AS numeric) > ?` | `[9]` |
| `labels.env = 'prod'` | `labels->>'env' = ?` | `[prod]` |
| `status.conditions.Reconciled = 'True'` | `jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?` | `[$[*]?(@.type=="Reconciled"), True]` |
| `status.conditions.Reconciled.last_updated_time < '2026-01-01T00:00:00Z'` | `CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) < ?::timestamptz` | `[$[*]?(@.type=="Reconciled"), last_updated_time, 2026-01-01T00:00:00Z]` |

## Deleted Functions/Regexes

| Item | File | Line (before) | Purpose |
|---|---|---|---|
| `specDeepPathPattern` | `pkg/db/sql_helpers.go` | 203 | Regex matching `spec.a.b.c` for encoding |
| `PreprocessSpecSubfields()` | `pkg/db/sql_helpers.go` | 216-263 | Encoded deep spec paths: `spec.a.b.c` → `spec.a__b__c` |
| `conditionSubfieldPattern` | `pkg/db/sql_helpers.go` | 270 | Regex matching 4-part condition paths |
| `PreprocessConditionSubfields()` | `pkg/db/sql_helpers.go` | 280-317 | Encoded 4-part conditions: `Reconciled.last_updated_time` → `Reconciled__last_updated_time` |
| `TestPreprocessSpecSubfields` | `pkg/db/sql_helpers_test.go` | 727-798 | Tests for deleted function |
| `TestPreprocessConditionSubfields` | `pkg/db/sql_helpers_test.go` | 340-405 | Tests for deleted function |
| `TestGetField_SpecNestedEncoded` | `pkg/db/sql_helpers_test.go` | 887-929 | Renamed to `TestGetField_SpecNested`, inputs changed from `__` to `.` |

## Added Functions

| Item | File | Purpose |
|---|---|---|
| `NormalizeInSyntax()` | `pkg/db/sql_helpers.go` | Backward-compat: converts `IN (...)` → `IN [...]` |
| `IdentWalk()` | `pkg/db/sql_helpers.go` | Custom identifier walker (replaces v6's `ident.Walk` which re-parses) |
| `TestSearchV6Equivalence` | `pkg/db/search_v6_equivalence_test.go` | Golden test proving SQL equivalence |

## Key Code Changes

- `getField()` spec branch: `strings.Split(..., "__")` → `strings.Split(..., ".")` — v6 gives full dotted path
- `conditionsNodeConverter()`: `SplitN(parts[2], "__", 2)` → `len(parts) == 4` check — v6 gives 4-part path directly
- `extractConditionsWalk()` NOT handling: `n.Left` → `n.Right` — v6 NOT is unary with child in Right
- `comparisonOperators`: `map[string]string` → `map[tsl.Operator]string`
- `buildSearchValues()`: removed 2 preprocessing calls, added `NormalizeInSyntax`
- `treeWalkForRelatedTables/treeWalkForAddingTableName`: `ident.Walk` → `db.IdentWalk` (custom)
