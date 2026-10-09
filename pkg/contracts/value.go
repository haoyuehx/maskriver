// Package contracts defines the Main-owned M1 component contracts.
// APIRevision is a development freeze, not a stable public SDK promise.
package contracts

import (
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"
)

const APIRevision = "m1a-v1"

type Kind uint8

const (
	Null Kind = iota
	Text
	Bytes
	Int
	Uint
	Float
	Decimal
	Bool
	Date
	LocalDateTime
	Instant
)

// Value is immutable and comparable. Its zero value is SQL NULL, not empty text.
// Payload access is explicit; logging and JSON serialization are redacted.
// Decimal retains scale; dates and local datetimes carry no invented timezone.
type Value struct {
	kind Kind
	raw  string
}

var decimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)
var instantPattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$`)

// NewValue accepts the exact representations specified in docs/contracts.md.
// Errors never include payloads. Bytes accepts arbitrary binary string data.
func NewValue(kind Kind, raw string) (Value, error) {
	valid := true
	switch kind {
	case Null:
		valid = raw == ""
	case Text:
		valid = utf8.ValidString(raw)
	case Bytes:
	case Int:
		n, err := strconv.ParseInt(raw, 10, 64)
		valid = err == nil && strconv.FormatInt(n, 10) == raw
	case Uint:
		n, err := strconv.ParseUint(raw, 10, 64)
		valid = err == nil && strconv.FormatUint(n, 10) == raw
	case Float:
		n, err := strconv.ParseFloat(raw, 64)
		valid = err == nil && !math.IsNaN(n) && !math.IsInf(n, 0)
		if valid {
			raw = strconv.FormatFloat(n, 'g', -1, 64)
		}
	case Decimal:
		valid = decimalPattern.MatchString(raw)
	case Bool:
		valid = raw == "true" || raw == "false"
	case Date:
		t, err := time.Parse(time.DateOnly, raw)
		valid = err == nil && t.Format(time.DateOnly) == raw && t.Year() >= 1
	case LocalDateTime:
		t, err := time.Parse("2006-01-02T15:04:05.999999999", raw)
		valid = err == nil && t.Format("2006-01-02T15:04:05.999999999") == raw && t.Year() >= 1
	case Instant:
		t, err := time.Parse(time.RFC3339Nano, raw)
		valid = err == nil && instantPattern.MatchString(raw) && t.Year() >= 1 && t.UTC().Year() >= 1 && t.UTC().Year() <= 9999
		if valid {
			raw = t.UTC().Format(time.RFC3339Nano)
		}
	default:
		valid = false
	}
	if !valid {
		return Value{}, ErrInvalid
	}
	return Value{kind: kind, raw: raw}, nil
}

func (v Value) Kind() Kind   { return v.kind }
func (v Value) IsNull() bool { return v.kind == Null }

// Payload is for adapters/algorithms only. Never put the returned data in logs.
func (v Value) Payload() string              { return v.raw }
func (v Value) String() string               { return "[REDACTED]" }
func (v Value) GoString() string             { return v.String() }
func (v Value) Format(s fmt.State, _ rune)   { _, _ = io.WriteString(s, v.String()) }
func (v Value) MarshalJSON() ([]byte, error) { return []byte(`"[REDACTED]"`), nil }

// UnmarshalJSON refuses to turn a redacted report into executable data.
func (*Value) UnmarshalJSON([]byte) error { return ErrUnsupported }
