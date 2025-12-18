// Package sliceutil provides generic slice manipulation utilities.
package sliceutil

// Deduplicate removes duplicate items from a slice while preserving order.
// The keyFunc extracts a unique key from each item for comparison.
// Only the first occurrence of each key is kept.
//
// Example:
//
//	courses := []storage.Course{{UID: "123"}, {UID: "456"}, {UID: "123"}}
//	unique := sliceutil.Deduplicate(courses, func(c storage.Course) string { return c.UID })
//	// Result: [{UID: "123"}, {UID: "456"}]
func Deduplicate[T any, K comparable](items []T, keyFunc func(T) K) []T {
	if len(items) == 0 {
		return items
	}

	seen := make(map[K]bool, len(items))
	result := make([]T, 0, len(items))

	for _, item := range items {
		key := keyFunc(item)
		if !seen[key] {
			seen[key] = true
			result = append(result, item)
		}
	}

	return result
}
