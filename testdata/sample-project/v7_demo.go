package sample

import (
	"errors"
	"fmt"
	"strings"
)

// EventKind classifies the payload of an Event.
type EventKind int

const (
	EventCreated EventKind = iota
	EventUpdated
	EventDeleted
	EventArchived
)

// Event is a tagged union. The concrete payload lives in Payload and is
// expected to be one of the Payload* types declared below.
type Event struct {
	Kind    EventKind
	Actor   string
	Payload interface{}
}

// Payload types intentionally share a method so that an interface
// (EventPayload) can be satisfied and exercised by go/types.
type PayloadCreate struct{ ResourceID string }
type PayloadUpdate struct {
	ResourceID string
	Fields     map[string]string
}
type PayloadDelete struct{ ResourceID string }

// Describe returns a short human readable description of the payload.
// All three types below implement EventPayload with the same signature,
// which is a direct test of type-checked interface-satisfaction.
func (p PayloadCreate) Describe() string { return "create:" + p.ResourceID }
func (p PayloadUpdate) Describe() string {
	keys := make([]string, 0, len(p.Fields))
	for k := range p.Fields {
		keys = append(keys, k)
	}
	return fmt.Sprintf("update:%s(%s)", p.ResourceID, strings.Join(keys, ","))
}
func (p PayloadDelete) Describe() string { return "delete:" + p.ResourceID }

// EventPayload is the interface satisfied by the three payload types.
type EventPayload interface {
	Describe() string
}

// ErrMalformedEvent is returned by Validate when the event is structurally
// broken and should never be delivered to downstream handlers.
var ErrMalformedEvent = errors.New("malformed event")

// Validate performs a multi-branch sanity check on an event. It is deliberately
// structured to touch: an if/else-if chain, several `if err != nil` error
// paths, and a default "ok" path — so that branch coverage and error-path
// coverage produce interesting numbers.
func Validate(e Event) error {
	if e.Actor == "" {
		return fmt.Errorf("empty actor: %w", ErrMalformedEvent)
	}
	if e.Kind < EventCreated || e.Kind > EventArchived {
		return fmt.Errorf("unknown kind %d: %w", e.Kind, ErrMalformedEvent)
	}
	if err := validatePayload(e); err != nil {
		return fmt.Errorf("payload: %w", err)
	}
	return nil
}

func validatePayload(e Event) error {
	if e.Kind == EventArchived {
		// Archived events may legitimately have no payload.
		return nil
	}
	if e.Payload == nil {
		return fmt.Errorf("nil payload for %v: %w", e.Kind, ErrMalformedEvent)
	}
	p, ok := e.Payload.(EventPayload)
	if !ok {
		return fmt.Errorf("payload %T lacks Describe: %w", e.Payload, ErrMalformedEvent)
	}
	if strings.TrimSpace(p.Describe()) == "" {
		return fmt.Errorf("empty description: %w", ErrMalformedEvent)
	}
	return nil
}

// Summarise dispatches on EventKind with a regular switch (exercises
// KindSwitchCase + KindDefault branch kinds) and further dispatches on the
// concrete payload with a type switch (exercises KindTypeCase). Uses Validate
// so that go/types' same-package call-graph is non-trivial.
func Summarise(e Event) (string, error) {
	if err := Validate(e); err != nil {
		return "", err
	}

	var action string
	switch e.Kind {
	case EventCreated:
		action = "CREATE"
	case EventUpdated:
		action = "UPDATE"
	case EventDeleted:
		action = "DELETE"
	case EventArchived:
		action = "ARCHIVE"
	default:
		return "", fmt.Errorf("unreachable kind %d: %w", e.Kind, ErrMalformedEvent)
	}

	detail := "—"
	switch p := e.Payload.(type) {
	case PayloadCreate:
		detail = "new=" + p.ResourceID
	case PayloadUpdate:
		detail = fmt.Sprintf("id=%s fields=%d", p.ResourceID, len(p.Fields))
	case PayloadDelete:
		detail = "removed=" + p.ResourceID
	case nil:
		detail = "no-payload"
	default:
		detail = fmt.Sprintf("%T", p)
	}

	return fmt.Sprintf("[%s] %s by %s (%s)", action, e.Actor, e.Actor, detail), nil
}

// AggregateByActor groups events by their Actor field and counts each kind.
// The returned map is suitable for display; empty input yields an empty map
// (not nil). Demonstrates a simple loop with a single branch — useful as a
// baseline for branch-coverage numbers.
func AggregateByActor(events []Event) map[string]map[EventKind]int {
	out := map[string]map[EventKind]int{}
	for _, e := range events {
		if e.Actor == "" {
			continue
		}
		if _, ok := out[e.Actor]; !ok {
			out[e.Actor] = map[EventKind]int{}
		}
		out[e.Actor][e.Kind]++
	}
	return out
}
