package labels

import (
	"reflect"
	"testing"
)

func TestQualify_CollisionsGetNamespaceUniqueStayBare(t *testing.T) {
	got := Qualify([]Item{
		{"team-a", "api"},
		{"team-b", "api"}, // collides with team-a/api
		{"team-a", "web"}, // unique name
	})
	want := []string{"team-a/api", "team-b/api", "web"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestQualify_EmptyNamespaceStaysBare(t *testing.T) {
	// Even on a name collision, an item with no namespace can't be qualified.
	got := Qualify([]Item{{"", "api"}, {"team-b", "api"}})
	want := []string{"api", "team-b/api"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestQualify_Empty(t *testing.T) {
	if got := Qualify(nil); len(got) != 0 {
		t.Errorf("nil input = %v, want empty", got)
	}
}
