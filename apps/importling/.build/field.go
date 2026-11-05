package main

import "slices"

// Field is what importling knows of one value: the values the names give,
// and the doubts about them. It is sure when the names give one value and no
// doubt, and open otherwise, and an open field asks the user, offering its
// values as suggestions
type Field[T comparable] struct {
	Values []T      `json:",omitempty"`
	Doubts []string `json:",omitempty"`
}

func sure[T comparable](value T) Field[T] {
	return Field[T]{Values: []T{value}}
}

func ask[T comparable](doubt string) Field[T] {
	return Field[T]{Doubts: []string{doubt}}
}

func (f Field[T]) Sure() bool {
	return len(f.Values) == 1 && len(f.Doubts) == 0
}

// Value is the field's value when it is sure, or else the zero value
func (f Field[T]) Value() T {
	var zero T
	if !f.Sure() {
		return zero
	}
	return f.Values[0]
}

// Questions say why an open field is open
func (f Field[T]) Questions() []string {
	switch {
	case len(f.Doubts) > 0:
		return f.Doubts
	case len(f.Values) > 1:
		return []string{"The names disagree"}
	case len(f.Values) == 0:
		return []string{"The names do not say"}
	}
	return nil
}

// reask opens a field the user left empty or mistyped, which offers again
// what it had
func (f Field[T]) reask(reason string) Field[T] {
	return Field[T]{Values: f.Values, Doubts: []string{reason}}
}

// join puts together what two sources say of one value. Two sources that
// agree make a sure field, and two that disagree an open one with both values
func join[T comparable](a, b Field[T]) Field[T] {
	for _, value := range b.Values {
		if !slices.Contains(a.Values, value) {
			a.Values = append(slices.Clip(a.Values), value)
		}
	}
	return a.doubt(b.Doubts...)
}

// doubt opens a field with the reasons given
func (f Field[T]) doubt(reasons ...string) Field[T] {
	for _, reason := range reasons {
		if !slices.Contains(f.Doubts, reason) {
			f.Doubts = append(slices.Clip(f.Doubts), reason)
		}
	}
	return f
}
