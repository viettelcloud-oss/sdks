package core

import (
	"context"
	"iter"
)

// Page holds one page of results returned by a list endpoint.
type Page[T any] struct {
	Count   int
	Results []T
}

// Paginate returns an iterator that fetches pages until all results are
// exhausted.  fetch is called for each page with the current page number
// and size; it must return a Page whose Results slice is safe to retain
// across calls.
func Paginate[T any](
	ctx context.Context,
	fetch func(ctx context.Context, page int, size int) (Page[T], error),
) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		pageNumber := 1
		pageSize := 50
		yielded := 0
		for {
			page, err := fetch(ctx, pageNumber, pageSize)
			if err != nil {
				yield(nil, err)
				return
			}
			for i := range page.Results {
				if !yield(&page.Results[i], nil) {
					return
				}
				yielded++
			}
			if len(page.Results) == 0 || yielded >= page.Count {
				return
			}
			pageNumber++
		}
	}
}
