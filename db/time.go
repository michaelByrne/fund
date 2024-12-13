package db

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type DBTime struct {
	time.Time
}

var timeLayouts = []string{
	time.RFC3339Nano,          // "2006-01-02T15:04:05.999999999Z07:00"
	time.RFC3339,              // "2006-01-02T15:04:05Z07:00"
	"2006-01-02T15:04:05.999", // "2006-01-02T15:04:05.999" (no timezone)
	"2006-01-02T15:04:05",     // "2006-01-02T15:04:05" (no timezone)
	"2006-01-02",              // "2006-01-02" (date only)
}

func normalizeTimezone(s string) string {
	if !strings.ContainsAny(s, "Z+-") && len(s) > 10 { // Check if no timezone and it's datetime
		return s + "Z" // Assume UTC if missing
	}
	return s
}

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

	return errors.New("failed to parse time: " + str)
}

func (t *DBTime) MarshalJSON() ([]byte, error) {
	if t.Time.IsZero() {
		return json.Marshal("")
	}

	return json.Marshal(t.Time.Format(time.RFC3339Nano))
}

func (t *DBTime) Scan(value interface{}) error {
	if value == nil {
		*t = DBTime{}
		return nil
	}
	switch v := value.(type) {
	case time.Time:
		t.Time = v
		return nil
	default:
		return errors.New("invalid type for DBTime")
	}
}

func (t *DBTime) Value() (driver.Value, error) {
	if t.Time.IsZero() {
		return nil, nil
	}
	return t.Time, nil
}

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

func (t *NullDBTime) MarshalJSON() ([]byte, error) {
	if !t.Valid {
		return json.Marshal(nil)
	}
	return t.DBTime.MarshalJSON()
}

func (t *NullDBTime) Scan(value interface{}) error {
	if value == nil {
		t.DBTime = DBTime{}
		t.Valid = false
		return nil
	}
	t.Valid = true
	switch v := value.(type) {
	case time.Time:
		t.DBTime = DBTime{Time: v}
		return nil
	default:
		return errors.New("invalid type for NullDBTime")
	}
}

func (t *NullDBTime) Value() (driver.Value, error) {
	if !t.Valid {
		return nil, nil
	}
	return t.DBTime.Time, nil
}
