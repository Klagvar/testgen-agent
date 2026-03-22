package sample

import (
	"errors"
	"fmt"
	"testing"
)

func TestValidateAge_HappyPath(t *testing.T) {
	err := ValidateAge(25)
	if err != nil {
		t.Errorf("ValidateAge(25) returned unexpected error: %v", err)
	}
}

func TestValidateAge_NegativeAge(t *testing.T) {
	err := ValidateAge(-1)
	if err == nil {
		t.Error("ValidateAge(-1) should return an error")
		return
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("ValidateAge(-1) error should be wrapped with ErrValidation, got: %v", err)
	}
	expectedMsg := "negative age -1: "
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("ValidateAge(-1) error message should start with %q, got: %q", expectedMsg, err.Error())
	}
}

func TestValidateAge_UnrealisticAge(t *testing.T) {
	err := ValidateAge(200)
	if err == nil {
		t.Error("ValidateAge(200) should return an error")
		return
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("ValidateAge(200) error should be wrapped with ErrValidation, got: %v", err)
	}
	expectedMsg := "unrealistic age 200: "
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("ValidateAge(200) error message should start with %q, got: %q", expectedMsg, err.Error())
	}
}

func TestValidateAge_ZeroAge(t *testing.T) {
	err := ValidateAge(0)
	if err != nil {
		t.Errorf("ValidateAge(0) should not return an error, got: %v", err)
	}
}

func TestValidateAge_MaxIntAge(t *testing.T) {
	err := ValidateAge(150)
	if err != nil {
		t.Errorf("ValidateAge(150) should not return an error, got: %v", err)
	}
}

func TestLookupUser_FoundUser(t *testing.T) {
	name, err := LookupUser(1)
	if err != nil {
		t.Errorf("LookupUser(1) returned unexpected error: %v", err)
	}
	if name != "Alice" {
		t.Errorf("LookupUser(1) should return \"Alice\", got: %q", name)
	}
}

func TestLookupUser_NotFoundUser(t *testing.T) {
	name, err := LookupUser(3)
	if err == nil {
		t.Error("LookupUser(3) should return an error")
		return
	}
	if name != "" {
		t.Errorf("LookupUser(3) should return empty string, got: %q", name)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("LookupUser(3) error should be wrapped with ErrNotFound, got: %v", err)
	}
	expectedMsg := "user 3: "
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("LookupUser(3) error message should start with %q, got: %q", expectedMsg, err.Error())
	}
}

func TestLookupUser_NegativeID(t *testing.T) {
	name, err := LookupUser(-1)
	if err == nil {
		t.Error("LookupUser(-1) should return an error")
		return
	}
	if name != "" {
		t.Errorf("LookupUser(-1) should return empty string, got: %q", name)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("LookupUser(-1) error should be wrapped with ErrNotFound, got: %v", err)
	}
	expectedMsg := "user -1: "
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("LookupUser(-1) error message should start with %q, got: %q", expectedMsg, err.Error())
	}
}

func TestCheckPermission_AdminRole(t *testing.T) {
	err := CheckPermission("admin")
	if err != nil {
		t.Errorf("CheckPermission(\"admin\") should not return an error, got: %v", err)
	}
}

func TestCheckPermission_NonAdminRole(t *testing.T) {
	err := CheckPermission("user")
	if err == nil {
		t.Error("CheckPermission(\"user\") should return an error")
		return
	}
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("CheckPermission(\"user\") error should be wrapped with ErrForbidden, got: %v", err)
	}
	expectedMsg := "role \"user\": "
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("CheckPermission(\"user\") error message should start with %q, got: %q", expectedMsg, err.Error())
	}
}

func TestCheckPermission_EmptyRole(t *testing.T) {
	err := CheckPermission("")
	if err == nil {
		t.Error("CheckPermission(\"\") should return an error")
		return
	}
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("CheckPermission(\"\") error should be wrapped with ErrForbidden, got: %v", err)
	}
	expectedMsg := "role \"\": "
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("CheckPermission(\"\") error message should start with %q, got: %q", expectedMsg, err.Error())
	}
}

func TestUnwrapAndClassify_Ok(t *testing.T) {
	result := UnwrapAndClassify(nil)
	if result != "ok" {
		t.Errorf("UnwrapAndClassify(nil) should return \"ok\", got: %q", result)
	}
}

func TestUnwrapAndClassify_NotFound(t *testing.T) {
	err := fmt.Errorf("user 1: %w", ErrNotFound)
	result := UnwrapAndClassify(err)
	if result != "not_found" {
		t.Errorf("UnwrapAndClassify(wrapped ErrNotFound) should return \"not_found\", got: %q", result)
	}
}

func TestUnwrapAndClassify_Forbidden(t *testing.T) {
	err := fmt.Errorf("role \"user\": %w", ErrForbidden)
	result := UnwrapAndClassify(err)
	if result != "forbidden" {
		t.Errorf("UnwrapAndClassify(wrapped ErrForbidden) should return \"forbidden\", got: %q", result)
	}
}

func TestUnwrapAndClassify_Validation(t *testing.T) {
	err := fmt.Errorf("negative age 1: %w", ErrValidation)
	result := UnwrapAndClassify(err)
	if result != "validation" {
		t.Errorf("UnwrapAndClassify(wrapped ErrValidation) should return \"validation\", got: %q", result)
	}
}

func TestUnwrapAndClassify_UnknownError(t *testing.T) {
	err := errors.New("some unknown error")
	result := UnwrapAndClassify(err)
	if result != "unknown" {
		t.Errorf("UnwrapAndClassify(unknown error) should return \"unknown\", got: %q", result)
	}
}

func TestUnwrapAndClassify_NestedWrap(t *testing.T) {
	err := fmt.Errorf("outer error: %w", fmt.Errorf("inner error: %w", ErrNotFound))
	result := UnwrapAndClassify(err)
	if result != "not_found" {
		t.Errorf("UnwrapAndClassify(nested wrapped ErrNotFound) should return \"not_found\", got: %q", result)
	}
}
