package sample

import (
	"errors"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		event   Event
		wantErr bool
		errIs   error
		errMsg  string
	}{
		{
			name:    "valid created event",
			event:   Event{Kind: EventCreated, Actor: "user1", Payload: PayloadCreate{ResourceID: "res1"}},
			wantErr: false,
		},
		{
			name:    "valid update event",
			event:   Event{Kind: EventUpdated, Actor: "user1", Payload: PayloadUpdate{ResourceID: "res1", Fields: map[string]string{"field1": "value1"}}},
			wantErr: false,
		},
		{
			name:    "empty actor",
			event:   Event{Kind: EventCreated, Actor: "", Payload: PayloadCreate{ResourceID: "res1"}},
			wantErr: true,
			errIs:   ErrMalformedEvent,
			errMsg:  "empty actor",
		},
		{
			name:    "invalid event kind",
			event:   Event{Kind: 999, Actor: "user1", Payload: PayloadCreate{ResourceID: "res1"}},
			wantErr: true,
			errIs:   ErrMalformedEvent,
			errMsg:  "unknown kind",
		},
		{
			name:    "nil payload for created event",
			event:   Event{Kind: EventCreated, Actor: "user1", Payload: nil},
			wantErr: true,
			errIs:   ErrMalformedEvent,
			errMsg:  "nil payload",
		},
		{
			name:    "invalid payload type",
			event:   Event{Kind: EventCreated, Actor: "user1", Payload: "invalid"},
			wantErr: true,
			errIs:   ErrMalformedEvent,
			errMsg:  "payload string lacks Describe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.event)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error, got nil")
					return
				}
				if tt.errIs != nil && !errors.Is(err, tt.errIs) {
					t.Errorf("Validate() error = %v, want error to be %v", err, tt.errIs)
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, want error to contain %s", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidatePayload(t *testing.T) {
	tests := []struct {
		name    string
		event   Event
		wantErr bool
		errIs   error
		errMsg  string
	}{
		{
			name:    "archived event with nil payload",
			event:   Event{Kind: EventArchived, Actor: "user1", Payload: nil},
			wantErr: false,
		},
		{
			name:    "archived event with payload",
			event:   Event{Kind: EventArchived, Actor: "user1", Payload: PayloadCreate{ResourceID: "resource1"}},
			wantErr: false,
		},
		{
			name:    "nil payload for created event",
			event:   Event{Kind: EventCreated, Actor: "user1", Payload: nil},
			wantErr: true,
			errIs:   ErrMalformedEvent,
			errMsg:  "nil payload",
		},
		{
			name:    "invalid payload type",
			event:   Event{Kind: EventCreated, Actor: "user1", Payload: "invalid"},
			wantErr: true,
			errIs:   ErrMalformedEvent,
			errMsg:  "payload string lacks Describe",
		},

		{
			name:    "valid payload",
			event:   Event{Kind: EventCreated, Actor: "user1", Payload: PayloadCreate{ResourceID: "resource1"}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePayload(tt.event)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validatePayload() expected error, got nil")
					return
				}
				if tt.errIs != nil && !errors.Is(err, tt.errIs) {
					t.Errorf("validatePayload() error = %v, want error to be %v", err, tt.errIs)
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validatePayload() error = %v, want error to contain %s", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validatePayload() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestSummarise(t *testing.T) {
	tests := []struct {
		name    string
		event   Event
		want    string
		wantErr bool
		errIs   error
	}{
		{
			name:    "valid created event",
			event:   Event{Kind: EventCreated, Actor: "user1", Payload: PayloadCreate{ResourceID: "resource1"}},
			want:    "[CREATE] user1 by user1 (new=resource1)",
			wantErr: false,
		},
		{
			name:    "valid updated event",
			event:   Event{Kind: EventUpdated, Actor: "user1", Payload: PayloadUpdate{ResourceID: "resource1", Fields: map[string]string{"field1": "value1", "field2": "value2"}}},
			want:    "[UPDATE] user1 by user1 (id=resource1 fields=2)",
			wantErr: false,
		},
		{
			name:    "valid deleted event",
			event:   Event{Kind: EventDeleted, Actor: "user1", Payload: PayloadDelete{ResourceID: "resource1"}},
			want:    "[DELETE] user1 by user1 (removed=resource1)",
			wantErr: false,
		},
		{
			name:    "valid archived event with no payload",
			event:   Event{Kind: EventArchived, Actor: "user1", Payload: nil},
			want:    "[ARCHIVE] user1 by user1 (no-payload)",
			wantErr: false,
		},
		{
			name:    "invalid event - empty actor",
			event:   Event{Kind: EventCreated, Actor: "", Payload: PayloadCreate{ResourceID: "resource1"}},
			wantErr: true,
			errIs:   ErrMalformedEvent,
		},
		{
			name:    "invalid event - invalid event kind",
			event:   Event{Kind: 999, Actor: "user1", Payload: PayloadCreate{ResourceID: "resource1"}},
			wantErr: true,
			errIs:   ErrMalformedEvent,
		},
		{
			name:    "invalid event - nil payload",
			event:   Event{Kind: EventCreated, Actor: "user1", Payload: nil},
			wantErr: true,
			errIs:   ErrMalformedEvent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Summarise(tt.event)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Summarise() expected error, got nil")
					return
				}
				if tt.errIs != nil && !errors.Is(err, tt.errIs) {
					t.Errorf("Summarise() error = %v, want error to be %v", err, tt.errIs)
				}
			} else {
				if err != nil {
					t.Errorf("Summarise() unexpected error = %v", err)
					return
				}
				if got != tt.want {
					t.Errorf("Summarise() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestAggregateByActor(t *testing.T) {
	tests := []struct {
		name   string
		events []Event
		want   map[string]map[EventKind]int
	}{
		{
			name:   "empty events",
			events: []Event{},
			want:   map[string]map[EventKind]int{},
		},
		{
			name: "single event",
			events: []Event{
				{Kind: EventCreated, Actor: "user1"},
			},
			want: map[string]map[EventKind]int{
				"user1": {
					EventCreated: 1,
				},
			},
		},
		{
			name: "multiple events same actor",
			events: []Event{
				{Kind: EventCreated, Actor: "user1"},
				{Kind: EventUpdated, Actor: "user1"},
				{Kind: EventCreated, Actor: "user1"},
			},
			want: map[string]map[EventKind]int{
				"user1": {
					EventCreated: 2,
					EventUpdated: 1,
				},
			},
		},
		{
			name: "multiple actors",
			events: []Event{
				{Kind: EventCreated, Actor: "user1"},
				{Kind: EventUpdated, Actor: "user2"},
				{Kind: EventCreated, Actor: "user1"},
			},
			want: map[string]map[EventKind]int{
				"user1": {
					EventCreated: 2,
				},
				"user2": {
					EventUpdated: 1,
				},
			},
		},
		{
			name: "events with empty actor",
			events: []Event{
				{Kind: EventCreated, Actor: "user1"},
				{Kind: EventUpdated, Actor: ""},
				{Kind: EventCreated, Actor: "user1"},
			},
			want: map[string]map[EventKind]int{
				"user1": {
					EventCreated: 2,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AggregateByActor(tt.events)
			if len(got) != len(tt.want) {
				t.Errorf("AggregateByActor() length = %v, want %v", len(got), len(tt.want))
				return
			}
			for actor, kinds := range got {
				wantKinds, exists := tt.want[actor]
				if !exists {
					t.Errorf("AggregateByActor() unexpected actor %v", actor)
					continue
				}
				if len(kinds) != len(wantKinds) {
					t.Errorf("AggregateByActor() actor %v kind count = %v, want %v", actor, len(kinds), len(wantKinds))
					continue
				}
				for kind, count := range kinds {
					wantCount, exists := wantKinds[kind]
					if !exists {
						t.Errorf("AggregateByActor() unexpected kind %v for actor %v", kind, actor)
						continue
					}
					if count != wantCount {
						t.Errorf("AggregateByActor() actor %v kind %v count = %v, want %v", actor, kind, count, wantCount)
					}
				}
			}
		})
	}
}
