package app

import "testing"

func TestIntimacyBoundsExcludeCloseContact(t *testing.T) {
	cases := []struct {
		name   string
		bounds []string
		want   bool
	}{
		{"empty", nil, false},
		{"unrelated refusal", []string{"不替人作保"}, false},
		{"absolute", []string{"不与人有肌肤之亲"}, true},
		{"absolute phrasing two", []string{"守身如玉，不与人行房事"}, true},
		{"refuse verb", []string{"拒绝肌肤之亲"}, true},
		{"never prefix", []string{"绝不与任何人身体接触"}, true},
		{"promise not refusal", []string{"此生以身相许者，唯他一人"}, false},
		{"state not rule", []string{"清白之身"}, false},
		{"willing", []string{"愿意与人肌肤之亲"}, false},
		{"willing two", []string{"可与有情人肌肤之亲"}, false},
		{"emotional only", []string{"不与人建立亲密关系"}, false},
		{"emotional only two", []string{"不谈男女之事，只论道法"}, false},
		{"blank entry", []string{"  "}, false},
	}
	for _, tc := range cases {
		if got := intimacyBoundsExcludeCloseContact(tc.bounds); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
