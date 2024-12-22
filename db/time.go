package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type DBTime struct {
	time.Time
}

var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

func normalizeTimezone(s string) string {
	if !strings.ContainsAny(s, "Z+-") && len(s) > 10 {
		return s + "Z"
	}
	return s
}

// For JSON marshaling
func (t *DBTime) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)

	if str == "" || str == "null" {
		t.Time = time.Time{}
		return nil
	}

	str = normalizeTimezone(str)

	var err error
	for _, layout := range timeLayouts {
		t.Time, err = time.Parse(layout, str)
		if err == nil {
			return nil
		}
	}

	return fmt.Errorf("failed to parse time: %s", str)
}

func (t DBTime) MarshalJSON() ([]byte, error) {
	if t.Time.IsZero() {
		return json.Marshal("")
	}
	return json.Marshal(t.Time.Format(time.RFC3339Nano))
}

// For pgx interface
func (t *DBTime) Scan(src interface{}) error {
	switch v := src.(type) {
	case time.Time:
		t.Time = v
		return nil
	case []byte:
		return t.DecodeText(v)
	case string:
		return t.DecodeText([]byte(v))
	case nil:
		t.Time = time.Time{}
		return nil
	default:
		return fmt.Errorf("cannot scan %T into DBTime", src)
	}
}

func (t DBTime) Value() (driver.Value, error) {
	if t.Time.IsZero() {
		return nil, nil
	}
	return t.Time, nil
}

// Text format handling for pgx
func (t *DBTime) DecodeText(src []byte) error {
	if src == nil {
		t.Time = time.Time{}
		return nil
	}

	str := string(src)
	str = normalizeTimezone(str)

	var err error
	var parsed time.Time
	for _, layout := range timeLayouts {
		parsed, err = time.Parse(layout, str)
		if err == nil {
			t.Time = parsed
			return nil
		}
	}

	return fmt.Errorf("failed to parse time: %s", str)
}

func (t DBTime) EncodeText(buf []byte) ([]byte, error) {
	if t.Time.IsZero() {
		return nil, nil
	}
	return append(buf, t.Time.Format(time.RFC3339Nano)...), nil
}

// Nullable version
type NullDBTime struct {
	DBTime
	Valid bool
}

func (t *NullDBTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		t.Valid = false
		return nil
	}
	err := t.DBTime.UnmarshalJSON(data)
	t.Valid = err == nil
	return err
}

func (t NullDBTime) MarshalJSON() ([]byte, error) {
	if !t.Valid {
		return json.Marshal(nil)
	}
	return t.DBTime.MarshalJSON()
}

func (t *NullDBTime) Scan(src interface{}) error {
	if src == nil {
		t.Valid = false
		return nil
	}
	t.Valid = true
	return t.DBTime.Scan(src)
}

func (t NullDBTime) Value() (driver.Value, error) {
	if !t.Valid {
		return nil, nil
	}
	return t.DBTime.Value()
}

func (t *NullDBTime) DecodeText(src []byte) error {
	if src == nil {
		t.Valid = false
		return nil
	}
	t.Valid = true
	return t.DBTime.DecodeText(src)
}

func (t NullDBTime) EncodeText(buf []byte) ([]byte, error) {
	if !t.Valid {
		return nil, nil
	}
	return t.DBTime.EncodeText(buf)
}
