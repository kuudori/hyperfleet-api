package db

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/yaacov/tree-search-language/v6/pkg/tsl"
)

func TestConditionsNodeConverterStatus(t *testing.T) {
	tests := []struct {
		name          string
		field         string
		value         string
		expectedSQL   string
		errorContains string
		expectedArgs  []interface{}
		expectError   bool
	}{
		{
			name:         "Reconciled condition True",
			field:        "status.conditions.Reconciled",
			value:        "True",
			expectedSQL:  "jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?",
			expectedArgs: []interface{}{`$[*] ? (@.type == "Reconciled")`, "True"},
		},
		{
			name:         "Reconciled condition False",
			field:        "status.conditions.Reconciled",
			value:        "False",
			expectedSQL:  "jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?",
			expectedArgs: []interface{}{`$[*] ? (@.type == "Reconciled")`, "False"},
		},
		{
			name:         "Available condition True",
			field:        "status.conditions.Available",
			value:        "True",
			expectedSQL:  "jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?",
			expectedArgs: []interface{}{`$[*] ? (@.type == "Available")`, "True"},
		},
		{
			name:         "Available condition Unknown",
			field:        "status.conditions.Available",
			value:        "Unknown",
			expectedSQL:  "jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?",
			expectedArgs: []interface{}{`$[*] ? (@.type == "Available")`, "Unknown"},
		},
		{
			name:          "Invalid condition status",
			field:         "status.conditions.Reconciled",
			value:         "Invalid",
			expectError:   true,
			errorContains: "condition status 'Invalid' is invalid",
		},
		{
			name:          "Invalid condition type - lowercase",
			field:         "status.conditions.ready",
			value:         "True",
			expectError:   true,
			errorContains: "must be PascalCase",
		},
		{
			name:          "Invalid condition type - with underscore",
			field:         "status.conditions.Reconciled_Status",
			value:         "True",
			expectError:   true,
			errorContains: "must be PascalCase",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			node := &tsl.Node{
				Kind:     tsl.KindBinaryExpr,
				Operator: tsl.OpEQ,
				Left:     &tsl.Node{Kind: tsl.KindIdentifier, Value: tt.field},
				Right:    &tsl.Node{Kind: tsl.KindStringLiteral, Value: tt.value},
			}

			result, err := conditionsNodeConverter(node)

			if tt.expectError {
				Expect(err).ToNot(BeNil())
				if tt.errorContains != "" {
					Expect(err.Error()).To(ContainSubstring(tt.errorContains))
				}
				return
			}

			Expect(err).To(BeNil())

			sqlizer := result.(interface {
				ToSql() (string, []interface{}, error)
			})
			sql, args, sqlErr := sqlizer.ToSql()
			Expect(sqlErr).ToNot(HaveOccurred())
			Expect(sql).To(Equal(tt.expectedSQL))
			Expect(args).To(HaveLen(len(tt.expectedArgs)))
			for i, expectedArg := range tt.expectedArgs {
				Expect(args[i]).To(Equal(expectedArg))
			}
		})
	}
}

func TestConditionsNodeConverterSubfields(t *testing.T) {
	tests := []struct {
		name          string
		field         string
		expectedSQL   string
		errorContains string
		value         interface{}
		expectedArgs  []interface{}
		op            tsl.Operator
		expectError   bool
	}{
		{
			name:        "last_updated_time less than",
			field:       "status.conditions.Reconciled.last_updated_time",
			op:          tsl.OpLT,
			value:       "2026-03-06T00:00:00Z",
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) < ?::timestamptz",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Reconciled")`,
				"last_updated_time",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:        "last_updated_time greater than",
			field:       "status.conditions.Reconciled.last_updated_time",
			op:          tsl.OpGT,
			value:       "2026-03-06T00:00:00Z",
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) > ?::timestamptz",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Reconciled")`,
				"last_updated_time",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:        "last_updated_time less than or equal",
			field:       "status.conditions.Reconciled.last_updated_time",
			op:          tsl.OpLE,
			value:       "2026-03-06T00:00:00Z",
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) <= ?::timestamptz",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Reconciled")`,
				"last_updated_time",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:        "last_updated_time greater than or equal",
			field:       "status.conditions.Reconciled.last_updated_time",
			op:          tsl.OpGE,
			value:       "2026-03-06T00:00:00Z",
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) >= ?::timestamptz",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Reconciled")`,
				"last_updated_time",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:        "last_updated_time equal",
			field:       "status.conditions.Reconciled.last_updated_time",
			op:          tsl.OpEQ,
			value:       "2026-03-06T00:00:00Z",
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) = ?::timestamptz",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Reconciled")`,
				"last_updated_time",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:        "last_updated_time not equal",
			field:       "status.conditions.Reconciled.last_updated_time",
			op:          tsl.OpNE,
			value:       "2026-03-06T00:00:00Z",
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) != ?::timestamptz",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Reconciled")`,
				"last_updated_time",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:        "last_transition_time less than",
			field:       "status.conditions.Available.last_transition_time",
			op:          tsl.OpLT,
			value:       "2026-03-06T00:00:00Z",
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) < ?::timestamptz",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Available")`,
				"last_transition_time",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:        "observed_generation less than",
			field:       "status.conditions.Reconciled.observed_generation",
			op:          tsl.OpLT,
			value:       float64(5),
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS INTEGER) < ?",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Reconciled")`,
				"observed_generation",
				5,
			},
		},
		{
			name:        "observed_generation equal",
			field:       "status.conditions.Reconciled.observed_generation",
			op:          tsl.OpEQ,
			value:       float64(3),
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS INTEGER) = ?",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Reconciled")`,
				"observed_generation",
				3,
			},
		},
		{
			name:          "Invalid subfield name",
			field:         "status.conditions.Reconciled.unknown_field",
			op:            tsl.OpLT,
			value:         "2026-03-06T00:00:00Z",
			expectError:   true,
			errorContains: "not supported",
		},
		{
			name:          "Invalid operator for subfield",
			field:         "status.conditions.Reconciled.last_updated_time",
			op:            tsl.OpLike,
			value:         "2026%",
			expectError:   true,
			errorContains: "not supported for condition subfield",
		},
		{
			name:          "Invalid condition type in subfield query",
			field:         "status.conditions.ready.last_updated_time",
			op:            tsl.OpLT,
			value:         "2026-03-06T00:00:00Z",
			expectError:   true,
			errorContains: "must be PascalCase",
		},
		{
			name:          "Invalid timestamp format",
			field:         "status.conditions.Reconciled.last_updated_time",
			op:            tsl.OpLT,
			value:         "not-a-timestamp",
			expectError:   true,
			errorContains: "expected RFC3339 format",
		},
		{
			name:          "Float value for integer subfield",
			field:         "status.conditions.Reconciled.observed_generation",
			op:            tsl.OpLT,
			value:         float64(3.5),
			expectError:   true,
			errorContains: "expected integer value",
		},
		{
			name:          "Integer overflow for integer subfield",
			field:         "status.conditions.Reconciled.observed_generation",
			op:            tsl.OpLT,
			value:         float64(3000000000),
			expectError:   true,
			errorContains: "out of 32-bit integer range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			var rightNode *tsl.Node
			switch v := tt.value.(type) {
			case string:
				rightNode = &tsl.Node{Kind: tsl.KindStringLiteral, Value: v}
			case float64:
				rightNode = &tsl.Node{Kind: tsl.KindNumericLiteral, Value: v}
			}

			node := &tsl.Node{
				Kind:     tsl.KindBinaryExpr,
				Operator: tt.op,
				Left:     &tsl.Node{Kind: tsl.KindIdentifier, Value: tt.field},
				Right:    rightNode,
			}

			result, err := conditionsNodeConverter(node)

			if tt.expectError {
				Expect(err).ToNot(BeNil())
				if tt.errorContains != "" {
					Expect(err.Error()).To(ContainSubstring(tt.errorContains))
				}
				return
			}

			Expect(err).To(BeNil())

			sqlizer := result.(interface {
				ToSql() (string, []interface{}, error)
			})
			sql, args, sqlErr := sqlizer.ToSql()
			Expect(sqlErr).ToNot(HaveOccurred())
			Expect(sql).To(Equal(tt.expectedSQL))
			Expect(args).To(HaveLen(len(tt.expectedArgs)))
			for i, expectedArg := range tt.expectedArgs {
				Expect(args[i]).To(Equal(expectedArg))
			}
		})
	}
}

func TestHasConditionWithSubfields(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		expected bool
	}{
		{
			name:     "3-part condition field",
			field:    "status.conditions.Reconciled",
			expected: true,
		},
		{
			name:     "4-part subfield (v6 native)",
			field:    "status.conditions.Reconciled.last_updated_time",
			expected: true,
		},
		{
			name:     "Non-condition field",
			field:    "labels.environment",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			node := &tsl.Node{
				Kind:     tsl.KindBinaryExpr,
				Operator: tsl.OpEQ,
				Left:     &tsl.Node{Kind: tsl.KindIdentifier, Value: tt.field},
				Right:    &tsl.Node{Kind: tsl.KindStringLiteral, Value: "value"},
			}

			result := hasCondition(node)
			Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestExtractConditionQueriesWithSubfields(t *testing.T) {
	tests := []struct {
		name                 string
		searchQuery          string
		expectedConditionSQL string
		expectedConditions   int
		expectError          bool
	}{
		{
			name:               "Subfield query only",
			searchQuery:        "status.conditions.Reconciled.last_updated_time < '2026-03-06T00:00:00Z'",
			expectedConditions: 1,
			expectedConditionSQL: "CAST(jsonb_path_query_first(status_conditions, " +
				"?::jsonpath) ->> ? AS TIMESTAMPTZ) < ?::timestamptz",
		},
		{
			name: "Mixed status and subfield queries",
			searchQuery: "status.conditions.Reconciled='False' AND " +
				"status.conditions.Reconciled.last_updated_time < '2026-03-06T00:00:00Z'",
			expectedConditions: 2,
		},
		{
			name: "Subfield query combined with label query",
			searchQuery: "labels.region='us-east' AND " +
				"status.conditions.Reconciled.last_updated_time < '2026-03-06T00:00:00Z'",
			expectedConditions: 1,
		},
		{
			name:        "NOT operator on condition query returns error",
			searchQuery: "NOT (status.conditions.Reconciled='True')",
			expectError: true,
		},
		{
			name:        "NOT operator on condition subfield query returns error",
			searchQuery: "NOT (status.conditions.Reconciled.last_updated_time < '2026-03-06T00:00:00Z')",
			expectError: true,
		},
		{
			name:        "NOT operator on nested condition under AND returns error",
			searchQuery: "NOT (status.conditions.Reconciled='True' AND name='test')",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			// v6 handles deep identifiers natively — no preprocessing needed
			tslTreeWrapper, err := tsl.ParseTSL(tt.searchQuery)
			Expect(err).ToNot(HaveOccurred())

			_, conditions, serviceErr := ExtractConditionQueries(tslTreeWrapper.Node)

			if tt.expectError {
				Expect(serviceErr).ToNot(BeNil())
				return
			}

			Expect(serviceErr).To(BeNil())
			Expect(conditions).To(HaveLen(tt.expectedConditions))

			if tt.expectedConditions > 0 && tt.expectedConditionSQL != "" {
				sql, _, sqlErr := conditions[0].ToSql()
				Expect(sqlErr).ToNot(HaveOccurred())
				Expect(sql).To(Equal(tt.expectedConditionSQL))
			}
		})
	}
}

func TestExtractConditionQueries(t *testing.T) {
	tests := []struct {
		name                 string
		searchQuery          string
		expectedConditionSQL string
		expectedConditions   int
		expectError          bool
	}{
		{
			name:                 "Single condition query",
			searchQuery:          "status.conditions.Reconciled='True'",
			expectedConditions:   1,
			expectedConditionSQL: "jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?",
		},
		{
			name:               "No condition queries",
			searchQuery:        "name='test'",
			expectedConditions: 0,
		},
		{
			name:               "Mixed query with condition",
			searchQuery:        "name='test' AND status.conditions.Reconciled='True'",
			expectedConditions: 1,
		},
		{
			name:               "Multiple condition queries",
			searchQuery:        "status.conditions.Reconciled='True' AND status.conditions.Available='True'",
			expectedConditions: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			tslTreeWrapper, err := tsl.ParseTSL(tt.searchQuery)
			Expect(err).ToNot(HaveOccurred())

			_, conditions, serviceErr := ExtractConditionQueries(tslTreeWrapper.Node)

			if tt.expectError {
				Expect(serviceErr).ToNot(BeNil())
				return
			}

			Expect(serviceErr).To(BeNil())
			Expect(conditions).To(HaveLen(tt.expectedConditions))

			if tt.expectedConditions > 0 && tt.expectedConditionSQL != "" {
				sql, _, sqlErr := conditions[0].ToSql()
				Expect(sqlErr).ToNot(HaveOccurred())
				Expect(sql).To(Equal(tt.expectedConditionSQL))
			}
		})
	}
}

func TestHasCondition(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		expected bool
	}{
		{
			name:     "Valid condition field",
			field:    "status.conditions.Reconciled",
			expected: true,
		},
		{
			name:     "Status field without conditions prefix",
			field:    "status.other_field",
			expected: false,
		},
		{
			name:     "Labels field",
			field:    "labels.environment",
			expected: false,
		},
		{
			name:     "Simple field",
			field:    "name",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			node := &tsl.Node{
				Kind:     tsl.KindBinaryExpr,
				Operator: tsl.OpEQ,
				Left:     &tsl.Node{Kind: tsl.KindIdentifier, Value: tt.field},
				Right:    &tsl.Node{Kind: tsl.KindStringLiteral, Value: "value"},
			}

			result := hasCondition(node)
			Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestConditionTypeValidation(t *testing.T) {
	tests := []struct {
		name        string
		condType    string
		expectMatch bool
	}{
		{"Valid - Reconciled", "Reconciled", true},
		{"Valid - Available", "Available", true},
		{"Valid - Progressing", "Progressing", true},
		{"Valid - CustomCondition", "CustomCondition", true},
		{"Valid - With numbers", "Reconciled2", true},
		{"Invalid - lowercase", "ready", false},
		{"Invalid - starts with number", "2Reconciled", false},
		{"Invalid - contains underscore", "Reconciled_State", false},
		{"Invalid - contains hyphen", "Reconciled-State", false},
		{"Invalid - empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			result := conditionTypePattern.MatchString(tt.condType)
			Expect(result).To(Equal(tt.expectMatch))
		})
	}
}

func TestGetField_SpecMapping(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:     "valid snake_case key",
			input:    "spec.is_default",
			expected: "spec->>'is_default'",
		},
		{
			name:     "valid single word key",
			input:    "spec.region",
			expected: "spec->>'region'",
		},
		{
			name:     "valid key with digits",
			input:    "spec.release_image_v2",
			expected: "spec->>'release_image_v2'",
		},
		{
			name:        "invalid key with uppercase",
			input:       "spec.ReleaseImage",
			expectError: true,
		},
		{
			name:        "invalid key with hyphens",
			input:       "spec.release-image",
			expectError: true,
		},
		{
			name:        "empty key",
			input:       "spec.",
			expectError: true,
		},
		{
			name:        "injection attempt",
			input:       "spec.'; DROP TABLE resources;--",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			field, err := getField(tt.input, map[string]string{})
			if tt.expectError {
				Expect(err).ToNot(BeNil())
			} else {
				Expect(err).To(BeNil())
				Expect(field).To(Equal(tt.expected))
			}
		})
	}
}

func TestGetField_SpecDisallowed(t *testing.T) {
	RegisterTestingT(t)

	disallowed := map[string]string{"spec": "spec"}

	_, err := getField("spec.is_default", disallowed)
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("not a valid field name"))
}

func TestGetField_SpecNested(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "1-level: spec.region",
			input:    "spec.region",
			expected: "spec->>'region'",
		},
		{
			name:     "2-level: spec.release.channel",
			input:    "spec.release.channel",
			expected: "spec->'release'->>'channel'",
		},
		{
			name:     "3-level: spec.release.config.zone",
			input:    "spec.release.config.zone",
			expected: "spec->'release'->'config'->>'zone'",
		},
		{
			name:     "2-level with underscore in key: spec.release.image_v2",
			input:    "spec.release.image_v2",
			expected: "spec->'release'->>'image_v2'",
		},
		{
			name:     "leading/trailing spaces are trimmed",
			input:    "  spec.region  ",
			expected: "spec->>'region'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			field, err := getField(tt.input, map[string]string{})
			Expect(err).To(BeNil())
			Expect(field).To(Equal(tt.expected))
		})
	}
}

// TestFieldNameWalk_NumericCast verifies that FieldNameWalk applies CAST(... AS numeric)
// to spec JSONB fields when compared against a number. This logic was previously in a
// separate WrapSpecNumericCasts tree walk and is now integrated into FieldNameWalk.
func TestFieldNameWalk_NumericCast(t *testing.T) {
	noDisallowed := map[string]string{}

	parseAndWalk := func(t *testing.T, search string) *tsl.Node {
		t.Helper()
		tree, err := tsl.ParseTSL(search)
		Expect(err).ToNot(HaveOccurred())
		result, serviceErr := FieldNameWalk(tree.Node, noDisallowed)
		Expect(serviceErr).To(BeNil())
		return result
	}

	t.Run("spec field with numeric RHS — CAST applied", func(t *testing.T) {
		RegisterTestingT(t)
		result := parseAndWalk(t, "spec.replicas > 9")
		Expect(result.Left.Value).To(Equal("CAST(spec->>'replicas' AS numeric)"))
	})

	t.Run("nested spec field with numeric RHS — CAST applied", func(t *testing.T) {
		RegisterTestingT(t)
		result := parseAndWalk(t, "spec.release.version > 9")
		Expect(result.Left.Value).To(Equal("CAST(spec->'release'->>'version' AS numeric)"))
	})

	t.Run("spec field with string RHS — no CAST", func(t *testing.T) {
		RegisterTestingT(t)
		result := parseAndWalk(t, "spec.channel = 'dev'")
		Expect(result.Left.Value).To(Equal("spec->>'channel'"))
	})

	t.Run("non-spec field with numeric RHS — no CAST", func(t *testing.T) {
		RegisterTestingT(t)
		result := parseAndWalk(t, "generation > 1")
		Expect(result.Left.Value).To(Equal("generation"))
	})

	t.Run("AND tree: only spec+numeric nodes get CAST", func(t *testing.T) {
		RegisterTestingT(t)
		result := parseAndWalk(t, "spec.replicas > 9 AND generation > 1 AND spec.channel = 'dev'")

		andLeft := result.Left
		specIdent := andLeft.Left.Left.Value.(string)
		Expect(specIdent).To(Equal("CAST(spec->>'replicas' AS numeric)"))

		genIdent := andLeft.Right.Left.Value.(string)
		Expect(genIdent).To(Equal("generation"))

		chanIdent := result.Right.Left.Value.(string)
		Expect(chanIdent).To(Equal("spec->>'channel'"))
	})
}

func TestConditionStatusValidation(t *testing.T) {
	tests := []struct {
		status      string
		expectValid bool
	}{
		{"True", true},
		{"False", true},
		{"Unknown", true},
		{"true", false},
		{"false", false},
		{"unknown", false},
		{"Yes", false},
		{"No", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			RegisterTestingT(t)

			result := validConditionStatuses[tt.status]
			Expect(result).To(Equal(tt.expectValid))
		})
	}
}

func TestNormalizeInSyntax(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "converts IN (...) to IN [...]",
			input:    "id in ('a', 'b')",
			expected: "id in ['a', 'b']",
		},
		{
			name:     "handles IN with uppercase",
			input:    "id IN ('a')",
			expected: "id IN ['a']",
		},
		{
			name:     "preserves IN [...] (already v6 syntax)",
			input:    "id in ['a', 'b']",
			expected: "id in ['a', 'b']",
		},
		{
			name:     "does not affect grouped expressions",
			input:    "(name = 'test') AND id = '1'",
			expected: "(name = 'test') AND id = '1'",
		},
		{
			name:     "does not modify quoted 'in' text",
			input:    "name = 'in (test)'",
			expected: "name = 'in (test)'",
		},
		{
			name:     "no in keyword",
			input:    "name = 'test'",
			expected: "name = 'test'",
		},
		{
			name:     "values with parens inside quotes",
			input:    "id in ('a(b)', 'c')",
			expected: "id in ['a(b)', 'c']",
		},
		{
			name:     "word containing 'in' is not matched (binding)",
			input:    "binding = 'x'",
			expected: "binding = 'x'",
		},
		{
			name:     "word containing 'in' at start (internal)",
			input:    "internal = 'x'",
			expected: "internal = 'x'",
		},
		{
			name:     "word ending with 'in' before real IN keyword (kind)",
			input:    "kind in ('Channel')",
			expected: "kind in ['Channel']",
		},
		{
			name:     "field 'index' not confused with IN",
			input:    "index = 5 AND id in ('a')",
			expected: "index = 5 AND id in ['a']",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			result := NormalizeInSyntax(tt.input)
			Expect(result).To(Equal(tt.expected))
		})
	}
}
