package storage

import (
	"net/url"
	"regexp"

	"github.com/google/uuid"
)

// Uploaded files live under <folder>/<userID>/<uuid>.<ext>, so a file URL says
// whose it is. Files uploaded before this layout (<folder>/<uuid>.<ext>) have no owner.
var ownedPath = regexp.MustCompile(`/(?:items|receipts)/([0-9a-fA-F-]{36})/[^/]+$`)

// UserFolder is the folder to hand to StorageProvider.Upload for [userID]'s files.
func UserFolder(folder string, userID uuid.UUID) string {
	return folder + "/" + userID.String()
}

// OwnerOf returns the user whose namespace [fileURL] sits in. ok is false for legacy
// files without a namespace and for URLs that are not in this layout at all
// (external photo links), which nobody owns.
func OwnerOf(fileURL string) (owner uuid.UUID, ok bool) {
	u, err := url.Parse(fileURL)
	if err != nil {
		return uuid.Nil, false
	}
	m := ownedPath.FindStringSubmatch(u.Path)
	if m == nil {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(m[1])
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// BelongsToOther reports whether [fileURL] is in another user's namespace.
// Such a file must never be attached to, or deleted for, [userID].
func BelongsToOther(fileURL string, userID uuid.UUID) bool {
	owner, ok := OwnerOf(fileURL)
	return ok && owner != userID
}
