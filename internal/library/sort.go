package library

import "sort"

func sortWorks(works []Work) {
	sort.SliceStable(works, func(i, j int) bool {
		a, b := works[i].Ref.PublishedAt, works[j].Ref.PublishedAt
		if a.IsZero() != b.IsZero() {
			return !a.IsZero()
		}
		if !a.IsZero() && !a.Equal(b.Time) {
			return a.After(b.Time)
		}
		return works[i].Ref.ID > works[j].Ref.ID
	})
}
