package sample

import (
	"cmp"
	"errors"
)

// Set is a generic set backed by a map.
type Set[T comparable] struct {
	items map[T]struct{}
}

// NewSet creates a new Set from the given items.
func NewSet[T comparable](items ...T) Set[T] {
	s := Set[T]{items: make(map[T]struct{}, len(items))}
	for _, item := range items {
		s.items[item] = struct{}{}
	}
	return s
}

// Add inserts an item into the set. Returns true if the item was new.
func (s *Set[T]) Add(item T) bool {
	if _, exists := s.items[item]; exists {
		return false
	}
	s.items[item] = struct{}{}
	return true
}

// Contains checks if an item is in the set.
func (s *Set[T]) Contains(item T) bool {
	_, exists := s.items[item]
	return exists
}

// Size returns the number of items in the set.
func (s *Set[T]) Size() int {
	return len(s.items)
}

// Remove removes an item from the set. Returns true if it was present.
func (s *Set[T]) Remove(item T) bool {
	if _, exists := s.items[item]; !exists {
		return false
	}
	delete(s.items, item)
	return true
}

// MapSlice applies a transformation function to each element of a slice.
func MapSlice[T any, U any](input []T, fn func(T) U) []U {
	result := make([]U, len(input))
	for i, v := range input {
		result[i] = fn(v)
	}
	return result
}

// Filter returns elements of a slice that satisfy the predicate.
func Filter[T any](input []T, pred func(T) bool) []T {
	var result []T
	for _, v := range input {
		if pred(v) {
			result = append(result, v)
		}
	}
	return result
}

// Reduce aggregates a slice into a single value.
func Reduce[T any, U any](input []T, initial U, fn func(U, T) U) U {
	acc := initial
	for _, v := range input {
		acc = fn(acc, v)
	}
	return acc
}

// MinMax returns the minimum and maximum values from a non-empty slice.
func MinMax[T cmp.Ordered](input []T) (T, T, error) {
	var zero T
	if len(input) == 0 {
		return zero, zero, errors.New("empty slice")
	}
	minVal, maxVal := input[0], input[0]
	for _, v := range input[1:] {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	return minVal, maxVal, nil
}
