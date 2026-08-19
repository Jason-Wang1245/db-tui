package postgres

import (
	"strings"
	"testing"
	"time"

	"github.com/Jason-Wang1245/db-tui/internal/grid"
)

func browserRelationFixture() grid.Relation {
	return grid.Relation{
		ID:   grid.RelationID{Schema: `odd"schema`, Name: `users; drop table audit`},
		Kind: grid.RelationTable, Identity: []string{"tenant_id", "id"}, HasXMin: true,
		Columns: []grid.Column{
			{Name: "tenant_id", DataType: "integer", TypeOID: 23, Sortable: true, IdentityPart: true},
			{Name: "id", DataType: "bigint", TypeOID: 20, Sortable: true, IdentityPart: true},
			{Name: "score", DataType: "integer", TypeOID: 23, Nullable: true, Sortable: true},
		},
	}
}

func TestEffectiveOrderAppendsIdentityTieBreakers(t *testing.T) {
	relation := browserRelationFixture()
	terms, err := effectiveOrder(relation, grid.Sort{Column: "score", Ascending: false})
	if err != nil {
		t.Fatal(err)
	}
	want := []orderTerm{{Column: "score", Ascending: false}, {Column: "tenant_id", Ascending: true}, {Column: "id", Ascending: true}}
	if len(terms) != len(want) {
		t.Fatalf("terms = %#v", terms)
	}
	for index := range want {
		if terms[index] != want[index] {
			t.Fatalf("term %d = %#v, want %#v", index, terms[index], want[index])
		}
	}
}

func TestKeysetQueryQuotesIdentifiersAndParameterizesCursorValues(t *testing.T) {
	relation := browserRelationFixture()
	terms, err := effectiveOrder(relation, grid.Sort{Column: "score", Ascending: true})
	if err != nil {
		t.Fatal(err)
	}
	values := []cursorValue{{Text: "9"}, {Text: "4"}, {Text: "99"}}
	query, args := keysetQuery(relation, terms, values, grid.PageNext, 101)
	for _, expected := range []string{
		`from "odd""schema"."users; drop table audit"`,
		`"score" > $1`,
		`"tenant_id" is not distinct from $2`,
		`order by "score" asc nulls last, "tenant_id" asc nulls last`,
		`limit 101`,
	} {
		if !strings.Contains(query, expected) {
			t.Fatalf("query does not contain %q: %s", expected, query)
		}
	}
	if strings.Contains(query, "99") || len(args) != 3 || args[2] != "99" {
		t.Fatalf("cursor values were not parameterized: query=%s args=%#v", query, args)
	}
}

func TestNullableSeekPredicatesMatchExplicitNullOrdering(t *testing.T) {
	ascending := orderTerm{Column: "score", Ascending: true}
	descending := orderTerm{Column: "score", Ascending: false}
	tests := []struct {
		name   string
		term   orderTerm
		value  cursorValue
		before bool
		want   string
	}{
		{"after ascending value", ascending, cursorValue{Text: "1"}, false, `("score" > $1 or "score" is null)`},
		{"after ascending null", ascending, cursorValue{Null: true}, false, "false"},
		{"before ascending null", ascending, cursorValue{Null: true}, true, `"score" is not null`},
		{"after descending null", descending, cursorValue{Null: true}, false, `"score" is not null`},
		{"before descending value", descending, cursorValue{Text: "1"}, true, `("score" > $1 or "score" is null)`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := cursorComparison(test.term, test.value, 1, test.before); got != test.want {
				t.Fatalf("comparison = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCursorRoundTripRejectsMismatchedMetadata(t *testing.T) {
	cursor := pageCursor{
		Version: 1, Relation: grid.RelationID{Schema: "public", Name: "users"},
		Sort: grid.Sort{Column: "email", Ascending: true}, Terms: []string{"email", "id"},
		Values: []cursorValue{{Text: "a@example.com"}, {Text: "1"}},
	}
	encoded, err := encodeCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	terms := []orderTerm{{Column: "email", Ascending: true}, {Column: "id", Ascending: true}}
	if err := validateKeysetCursor(decoded, cursor.Relation, cursor.Sort, terms); err != nil {
		t.Fatal(err)
	}
	if err := validateKeysetCursor(decoded, grid.RelationID{Schema: "public", Name: "other"}, cursor.Sort, terms); err == nil {
		t.Fatal("mismatched relation cursor was accepted")
	}
	if _, err := decodeCursor("not-base64"); err == nil {
		t.Fatal("malformed cursor was accepted")
	}
}

func TestPostgreSQLValueFormattingIsSingleLineAndTypeAware(t *testing.T) {
	tests := []struct {
		value  any
		column grid.Column
		want   string
	}{
		{nil, grid.Column{}, "NULL"},
		{"line one\nline two\x1b", grid.Column{DataType: "text"}, `line one\nline two\u001b`},
		{[]byte{0x00, 0xff}, grid.Column{DataType: "bytea", TypeOID: 17}, `\x00ff`},
		{time.Date(2026, 8, 19, 12, 30, 0, 0, time.UTC), grid.Column{DataType: "date", TypeOID: 1082}, "2026-08-19"},
	}
	for _, test := range tests {
		cell := formatCell(test.value, test.column)
		if cell.Display != test.want {
			t.Errorf("formatCell(%#v, %s) = %q, want %q", test.value, test.column.DataType, cell.Display, test.want)
		}
	}
}

func TestBestEffortOrderNeverInterpolatesValues(t *testing.T) {
	relation := browserRelationFixture()
	relation.Identity = nil
	order, err := bestEffortOrder(relation, grid.Sort{Column: "score", Ascending: false})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(order, `"score" desc nulls first`) || !strings.Contains(order, `("tenant_id"::text)`) {
		t.Fatalf("order = %q", order)
	}
}
