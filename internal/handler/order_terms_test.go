package handler

import (
	"net/http/httptest"
	"testing"

	"ragflow/internal/dao"

	"github.com/gin-gonic/gin"
)

func termsForQuery(t *testing.T, rawQuery, orderby string, desc bool) []dao.OrderTerm {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/v1/list?"+rawQuery, nil)
	return orderTermsFromQuery(c, orderby, desc)
}

func TestOrderTermsFromQuery(t *testing.T) {
	cases := []struct {
		name     string
		rawQuery string
		orderby  string
		desc     bool
		want     []dao.OrderTerm
	}{
		{
			name:     "sort carries several columns with their own directions",
			rawQuery: "sort=name:asc,create_time:desc",
			orderby:  "create_time",
			desc:     true,
			want:     []dao.OrderTerm{{Column: "name"}, {Column: "create_time", Desc: true}},
		},
		{
			name:     "sort wins over the older pair",
			rawQuery: "sort=name:asc&orderby=update_time&desc=true",
			orderby:  "update_time",
			desc:     true,
			want:     []dao.OrderTerm{{Column: "name"}},
		},
		{
			name:     "the older pair still orders on its own",
			rawQuery: "orderby=update_time&desc=false",
			orderby:  "update_time",
			want:     []dao.OrderTerm{{Column: "update_time"}},
		},
		{
			// The column is kept here and dropped by the entity allowlist, which is
			// where a name the list does not order by has always been answered.
			name:     "an unknown column is carried rather than rejected",
			rawQuery: "sort=nonsense:asc",
			orderby:  "create_time",
			desc:     true,
			want:     []dao.OrderTerm{{Column: "nonsense"}},
		},
		{
			name:     "a sort with nothing usable falls back to the older pair",
			rawQuery: "sort=:asc,&orderby=name&desc=true",
			orderby:  "name",
			desc:     true,
			want:     []dao.OrderTerm{{Column: "name", Desc: true}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := termsForQuery(t, tc.rawQuery, tc.orderby, tc.desc)
			if len(got) != len(tc.want) {
				t.Fatalf("orderTermsFromQuery(%q) = %+v, want %+v", tc.rawQuery, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("orderTermsFromQuery(%q)[%d] = %+v, want %+v", tc.rawQuery, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// A handler that rejects an unrecognised `orderby` reads the sort terms first, so
// this is the signal it gates that check on.
func TestSortTermsFromQuerySignalsWhoDecides(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newContext := func(rawQuery string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/v1/list?"+rawQuery, nil)
		return c
	}

	if terms := sortTermsFromQuery(newContext("orderby=nonsense&desc=maybe")); len(terms) != 0 {
		t.Fatalf("a request without sort reported terms: %+v", terms)
	}
	if terms := sortTermsFromQuery(newContext("sort=:asc,")); len(terms) != 0 {
		t.Fatalf("an unusable sort reported terms: %+v", terms)
	}
	terms := sortTermsFromQuery(newContext("sort=name:desc&orderby=nonsense"))
	if len(terms) != 1 || terms[0] != (dao.OrderTerm{Column: "name", Desc: true}) {
		t.Fatalf("sort terms = %+v, want one name DESC term", terms)
	}

	// The same terms decide the order, and the older pair is only read without them.
	if got := orderTerms(terms, "create_time", false); len(got) != 1 || got[0] != terms[0] {
		t.Fatalf("orderTerms dropped the sort terms: %+v", got)
	}
	got := orderTerms(nil, "create_time", true)
	if len(got) != 1 || got[0] != (dao.OrderTerm{Column: "create_time", Desc: true}) {
		t.Fatalf("orderTerms = %+v, want the older pair", got)
	}
}
