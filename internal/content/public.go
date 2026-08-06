// Public read-only accessors for content.Post. ListPublished/GetPublished
// are the only entry points the public-facing site may use — they wrap
// storage.ListPublished/GetPublishedBySlug and never return a post whose
// status is not 'published' (see design.md §3 on the public/admin read
// split).
package content

import (
	"database/sql"
	"fmt"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

// publishedListLimit bounds ListPublished to a fixed page size.
const publishedListLimit = 100

// ListPublished returns up to publishedListLimit published posts, ordered
// by published_at DESC, id DESC. Draft posts are never included.
func ListPublished(db *sql.DB) ([]Post, error) {
	posts, err := storage.ListPublished(db, publishedListLimit)
	if err != nil {
		return nil, fmt.Errorf("content: list published posts: %w", err)
	}
	out := make([]Post, len(posts))
	for i, p := range posts {
		out[i] = *fromStorage(p)
	}
	return out, nil
}

// GetPublished returns the published post with the given slug. Returns
// storage.ErrNotFound if no post with that slug exists, or if it exists
// but is not published (i.e. it is a draft).
func GetPublished(db *sql.DB, slug string) (Post, error) {
	p, err := storage.GetPublishedBySlug(db, slug)
	if err != nil {
		return Post{}, fmt.Errorf("content: get published post: %w", err)
	}
	return *fromStorage(p), nil
}
