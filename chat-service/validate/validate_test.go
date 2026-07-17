package validate

import "testing"

func TestGroupName(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"", true},
		{"a", true},
		{"开发组", false},
		{"  team-a  ", false},
		{"!!!", true},
		{string(make([]rune, 50)), true},
	}
	for _, c := range cases {
		_, err := GroupName(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("GroupName(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
		}
	}
}

func TestGroupID(t *testing.T) {
	if _, err := GroupID("ab", true); err == nil {
		t.Fatal("short id should fail")
	}
	if _, err := GroupID("search", true); err == nil {
		t.Fatal("reserved id should fail")
	}
	id, err := GroupID("my_team-01", true)
	if err != nil || id != "my_team-01" {
		t.Fatalf("got %q %v", id, err)
	}
}

func TestUsername(t *testing.T) {
	if _, err := Username("ab"); err == nil {
		t.Fatal("expected error")
	}
	u, err := Username("alice_01")
	if err != nil || u != "alice_01" {
		t.Fatalf("got %q %v", u, err)
	}
}

func TestCleanStripsControls(t *testing.T) {
	s := Clean("a\x00b\nc")
	if s != "ab\nc" {
		t.Fatalf("got %q", s)
	}
}
