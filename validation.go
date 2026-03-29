package quicframe

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

var (
	validateOnce sync.Once
	validateInst *validator.Validate
)

// FieldError describes a single validation failure.
type FieldError struct {
	Field string `msgpack:"field"`
	Tag   string `msgpack:"tag"`
	Value string `msgpack:"value,omitempty"`
}

// ValidationError wraps one or more field-level validation failures.
type ValidationError struct {
	Fields []FieldError `msgpack:"fields"`
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Fields) == 0 {
		return "validation failed"
	}

	parts := make([]string, 0, len(e.Fields))
	for _, field := range e.Fields {
		if field.Value != "" {
			parts = append(parts, fmt.Sprintf("%s failed %s=%s", field.Field, field.Tag, field.Value))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s failed %s", field.Field, field.Tag))
	}
	return "validation failed: " + strings.Join(parts, ", ")
}

// Validator returns the shared struct validator used by BindAndValidate.
func Validator() *validator.Validate {
	validateOnce.Do(func() {
		validateInst = validator.New(validator.WithRequiredStructEnabled())
	})
	return validateInst
}

// Validate validates a value using `validate` tags.
func Validate(v interface{}) error {
	if v == nil {
		return nil
	}
	if err := Validator().Struct(v); err != nil {
		var invalid *validator.InvalidValidationError
		if errors.As(err, &invalid) {
			return fmt.Errorf("quicframe: validate: %w", invalid)
		}

		var verr validator.ValidationErrors
		if errors.As(err, &verr) {
			fields := make([]FieldError, 0, len(verr))
			for _, fe := range verr {
				fields = append(fields, FieldError{
					Field: fe.Field(),
					Tag:   fe.Tag(),
					Value: fe.Param(),
				})
			}
			return &ValidationError{Fields: fields}
		}

		return fmt.Errorf("quicframe: validate: %w", err)
	}
	return nil
}
