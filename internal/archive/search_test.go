package archive

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestSearchResultPolymorphicFields locks in the fix for the decode error:
//
//	json: cannot unmarshal array into Go struct field
//	SearchResult.response.docs.creator of type string
//
// IA's advanced-search API returns single-valued metadata as a bare string
// and multi-valued metadata as an array. SearchResult must accept both.
func TestSearchResultPolymorphicFields(t *testing.T) {
	body := []byte(`{
	  "response": {
	    "numFound": 3,
	    "start": 0,
	    "docs": [
	      {
	        "identifier": "one",
	        "title": "Solo Title",
	        "creator": "John Doe",
	        "subject": "Liberia",
	        "language": "eng",
	        "date": "1850"
	      },
	      {
	        "identifier": "two",
	        "title": ["Primary", "Alt"],
	        "creator": ["Alice", "Bob"],
	        "subject": ["American Colonization Society", "Liberia"],
	        "language": ["eng", "fre"],
	        "date": ["1852-01-01"]
	      },
	      {
	        "identifier": "three",
	        "title": "Only identifier and title"
	      }
	    ]
	  }
	}`)

	var resp iaSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got, want := len(resp.Response.Docs), 3; got != want {
		t.Fatalf("docs: got %d, want %d", got, want)
	}

	one := resp.Response.Docs[0]
	if one.Creator.First() != "John Doe" {
		t.Errorf("one.Creator.First = %q, want %q", one.Creator.First(), "John Doe")
	}
	if !reflect.DeepEqual([]string(one.Subject), []string{"Liberia"}) {
		t.Errorf("one.Subject = %v, want [Liberia]", one.Subject)
	}
	if one.Title.First() != "Solo Title" {
		t.Errorf("one.Title.First = %q", one.Title.First())
	}
	if one.Date.First() != "1850" {
		t.Errorf("one.Date.First = %q", one.Date.First())
	}

	two := resp.Response.Docs[1]
	if !reflect.DeepEqual([]string(two.Creator), []string{"Alice", "Bob"}) {
		t.Errorf("two.Creator = %v, want [Alice Bob]", two.Creator)
	}
	if two.Creator.First() != "Alice" {
		t.Errorf("two.Creator.First = %q, want Alice", two.Creator.First())
	}
	if !reflect.DeepEqual([]string(two.Subject), []string{"American Colonization Society", "Liberia"}) {
		t.Errorf("two.Subject = %v", two.Subject)
	}
	if two.Title.First() != "Primary" {
		t.Errorf("two.Title.First = %q", two.Title.First())
	}
	if two.Date.First() != "1852-01-01" {
		t.Errorf("two.Date.First = %q", two.Date.First())
	}

	three := resp.Response.Docs[2]
	if three.Creator.First() != "" {
		t.Errorf("three.Creator.First = %q, want empty", three.Creator.First())
	}
	if len(three.Subject) != 0 {
		t.Errorf("three.Subject = %v, want empty", three.Subject)
	}
}

func TestFlexStringsNullAndMixed(t *testing.T) {
	var f FlexStrings
	if err := json.Unmarshal([]byte(`null`), &f); err != nil {
		t.Fatalf("null: %v", err)
	}
	if f != nil {
		t.Errorf("null → %v, want nil", f)
	}

	if err := json.Unmarshal([]byte(`[]`), &f); err != nil {
		t.Fatalf("empty array: %v", err)
	}
	if len(f) != 0 {
		t.Errorf("empty array → %v", f)
	}

	if err := json.Unmarshal([]byte(`["a", null, "b"]`), &f); err != nil {
		t.Fatalf("array with null: %v", err)
	}
	if !reflect.DeepEqual([]string(f), []string{"a", "b"}) {
		t.Errorf("array with null → %v", f)
	}
}
