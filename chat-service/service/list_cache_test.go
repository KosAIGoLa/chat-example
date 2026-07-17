package service

import (
	"testing"
	"time"
)

func TestListCacheDisabled(t *testing.T) {
	c, err := NewListCache("", "", 0, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if c.Enabled() {
		t.Fatal("expected disabled cache")
	}
	var out []string
	if c.GetJSON("k", &out) {
		t.Fatal("disabled cache should miss")
	}
	c.SetJSON("k", []string{"a"})
	c.Del("k")
	c.Close()
}

func TestListCacheKeyBuilders(t *testing.T) {
	if KeyFriends(1) != "list:friends:1" {
		t.Fatalf("friends key: %s", KeyFriends(1))
	}
	if KeyPrivatePins(5, 2) != "list:private:pins:2:5" {
		t.Fatalf("pins key should order ids: %s", KeyPrivatePins(5, 2))
	}
	if KeyGroupMembers("g_ab") != "list:group:members:g_ab" {
		t.Fatalf("members key: %s", KeyGroupMembers("g_ab"))
	}
}
