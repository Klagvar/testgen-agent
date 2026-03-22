// Tests patterns: encoding/json, io.Reader/Writer, struct tags
package sample

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// User has JSON and DB struct tags (tests struct tag visibility in prompt).
type User struct {
	ID    int    `json:"id" db:"user_id"`
	Name  string `json:"name" db:"user_name"`
	Email string `json:"email,omitempty" db:"email_addr"`
	Age   int    `json:"age" db:"age"`
}

// MarshalUser serializes a User to JSON bytes.
func MarshalUser(u User) ([]byte, error) {
	return json.Marshal(u)
}

// UnmarshalUser deserializes JSON bytes into a User.
func UnmarshalUser(data []byte) (User, error) {
	var u User
	if err := json.Unmarshal(data, &u); err != nil {
		return User{}, fmt.Errorf("unmarshal user: %w", err)
	}
	return u, nil
}

// ReadAndDecode reads JSON from an io.Reader and decodes into a User.
func ReadAndDecode(r io.Reader) (User, error) {
	var u User
	dec := json.NewDecoder(r)
	if err := dec.Decode(&u); err != nil {
		return User{}, fmt.Errorf("decode user: %w", err)
	}
	return u, nil
}

// EncodeToWriter writes a User as JSON to the given writer.
func EncodeToWriter(w io.Writer, u User) error {
	enc := json.NewEncoder(w)
	return enc.Encode(u)
}

// CountWords reads all text from a reader and counts words.
func CountWords(r io.Reader) (int, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return 0, nil
	}
	return len(strings.Fields(text)), nil
}
