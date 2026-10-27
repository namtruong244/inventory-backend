package storage_test

import (
	"testing"

	"inventory_backend/internal/storage"

	"github.com/google/uuid"
)

func TestOwnerOf(t *testing.T) {
	u := uuid.New()
	cases := []struct {
		name  string
		url   string
		owner uuid.UUID
		ok    bool
	}{
		{"local item photo", "http://localhost:8080/uploads/items/" + u.String() + "/x.jpg", u, true},
		{"r2 receipt", "https://pub-1.r2.dev/receipts/" + u.String() + "/x.pdf", u, true},
		{"legacy file without a folder", "http://localhost:8080/uploads/items/" + uuid.NewString() + ".jpg", uuid.Nil, false},
		{"external link", "https://images.unsplash.com/photo-1?w=800", uuid.Nil, false},
		{"not a uuid folder", "http://h/uploads/items/someone/x.jpg", uuid.Nil, false},
		{"garbage", "::not a url", uuid.Nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owner, ok := storage.OwnerOf(tc.url)
			if ok != tc.ok || owner != tc.owner {
				t.Fatalf("OwnerOf(%q) = %s, %v; want %s, %v", tc.url, owner, ok, tc.owner, tc.ok)
			}
		})
	}
}

func TestBelongsToOther(t *testing.T) {
	me, them := uuid.New(), uuid.New()
	mine := "http://h/uploads/items/" + me.String() + "/a.jpg"
	theirs := "http://h/uploads/items/" + them.String() + "/a.jpg"
	if storage.BelongsToOther(mine, me) {
		t.Error("own file is not someone else's")
	}
	if !storage.BelongsToOther(theirs, me) {
		t.Error("another user's file must be flagged")
	}
	if storage.BelongsToOther("http://h/uploads/items/legacy.jpg", me) || storage.BelongsToOther("https://x.test/p.jpg", me) {
		t.Error("files without an owner are not flagged")
	}
}
