package sample

import (
	"bytes"
	"strings"
	"testing"
)

func TestMarshalUser(t *testing.T) {
	tests := []struct {
		name     string
		user     User
		expected string
	}{
		{
			name: "normal user",
			user: User{
				ID:    1,
				Name:  "John Doe",
				Email: "john@example.com",
				Age:   30,
			},
			expected: `{"id":1,"name":"John Doe","email":"john@example.com","age":30}`,
		},

		{
			name: "user with omitempty email",
			user: User{
				ID:    3,
				Name:  "Bob Johnson",
				Email: "",
				Age:   35,
			},
			expected: `{"id":3,"name":"Bob Johnson","age":35}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MarshalUser(tt.user)
			if err != nil {
				t.Fatalf("MarshalUser failed: %v", err)
			}

			expectedBytes := []byte(tt.expected)
			if !bytes.Equal(result, expectedBytes) {
				t.Errorf("MarshalUser() = %s, want %s", result, expectedBytes)
			}
		})
	}
}

func TestEncodeToWriter(t *testing.T) {
	tests := []struct {
		name    string
		user    User
		want    string
		wantErr bool
	}{
		{
			name: "normal user",
			user: User{
				ID:    1,
				Name:  "John Doe",
				Email: "john@example.com",
				Age:   30,
			},
			want:    `{"id":1,"name":"John Doe","email":"john@example.com","age":30}` + "\n",
			wantErr: false,
		},

		{
			name: "user with omitempty email",
			user: User{
				ID:    3,
				Name:  "Bob Johnson",
				Email: "",
				Age:   35,
			},
			want:    `{"id":3,"name":"Bob Johnson","age":35}` + "\n",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := EncodeToWriter(&buf, tt.user)
			if (err != nil) != tt.wantErr {
				t.Fatalf("EncodeToWriter() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				result := buf.String()
				if result != tt.want {
					t.Errorf("EncodeToWriter() = %q, want %q", result, tt.want)
				}
			}
		})
	}
}

func TestUnmarshalUser(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    User
		wantErr bool
	}{
		{
			name: "valid user data",
			data: []byte(`{"id":1,"name":"John Doe","email":"john@example.com","age":30}`),
			want: User{
				ID:    1,
				Name:  "John Doe",
				Email: "john@example.com",
				Age:   30,
			},
			wantErr: false,
		},
		{
			name:    "invalid json data",
			data:    []byte(`{"id":1,"name":"John Doe","age":30`),
			want:    User{},
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    []byte{},
			want:    User{},
			wantErr: true,
		},
		{
			name: "user with omitempty email",
			data: []byte(`{"id":3,"name":"Bob Johnson","age":35}`),
			want: User{
				ID:    3,
				Name:  "Bob Johnson",
				Email: "",
				Age:   35,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := UnmarshalUser(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalUser() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if result != tt.want {
					t.Errorf("UnmarshalUser() = %v, want %v", result, tt.want)
				}
			}
		})
	}
}

func TestReadAndDecode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    User
		wantErr bool
	}{
		{
			name:  "valid user data",
			input: `{"id":1,"name":"John Doe","email":"john@example.com","age":30}`,
			want: User{
				ID:    1,
				Name:  "John Doe",
				Email: "john@example.com",
				Age:   30,
			},
			wantErr: false,
		},
		{
			name:    "invalid json data",
			input:   `{"id":1,"name":"John Doe","age":30`,
			want:    User{},
			wantErr: true,
		},
		{
			name:    "empty reader",
			input:   "",
			want:    User{},
			wantErr: true,
		},
		{
			name:    "whitespace only reader",
			input:   "   \n\t  ",
			want:    User{},
			wantErr: true,
		},
		{
			name:  "user with omitempty email",
			input: `{"id":3,"name":"Bob Johnson","age":35}`,
			want: User{
				ID:    3,
				Name:  "Bob Johnson",
				Email: "",
				Age:   35,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			result, err := ReadAndDecode(reader)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ReadAndDecode() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if result != tt.want {
					t.Errorf("ReadAndDecode() = %v, want %v", result, tt.want)
				}
			}
		})
	}
}

func TestCountWords(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{
			name:  "single word",
			input: "hello",
			want:  1,
		},
		{
			name:  "multiple words",
			input: "hello world",
			want:  2,
		},
		{
			name:  "multiple spaces",
			input: "hello   world",
			want:  2,
		},
		{
			name:    "empty reader",
			input:   "",
			want:    0,
			wantErr: false,
		},
		{
			name:  "whitespace only",
			input: "   \n\t  ",
			want:  0,
		},
		{
			name:  "text with leading/trailing whitespace",
			input: "  hello world  ",
			want:  2,
		},
		{
			name:  "text with various whitespace characters",
			input: "hello\nworld\tfoo\rbar",
			want:  4,
		},
		{
			name:  "single character",
			input: "a",
			want:  1,
		},
		{
			name:  "single character with spaces",
			input: " a ",
			want:  1,
		},
		{
			name:  "large text",
			input: "a b c d e f g h i j k l m n o p q r s t u v w x y z",
			want:  26,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			result, err := CountWords(reader)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CountWords() error = %v, wantErr %v", err, tt.wantErr)
			}
			if result != tt.want {
				t.Errorf("CountWords() = %v, want %v", result, tt.want)
			}
		})
	}
}
