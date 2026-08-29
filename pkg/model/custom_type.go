package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/realmroot/lightkite/pkg/utils"
)

type SecretString string

// Scan implements the sql.Scanner interface for reading encrypted data from database
func (s *SecretString) Scan(value interface{}) error {
	if value == nil {
		*s = ""
		return nil
	}
	var encryptedStr string
	switch v := value.(type) {
	case string:
		encryptedStr = v
	case []byte:
		encryptedStr = string(v)
	default:
		return fmt.Errorf("cannot scan %T into SecretString", value)
	}
	// If the string is empty, just set it directly
	if encryptedStr == "" {
		*s = ""
		return nil
	}

	// Decrypt the string
	decrypted, err := utils.DecryptString(encryptedStr)
	if err != nil {
		return fmt.Errorf("failed to decrypt SecretString: %w", err)
	}
	*s = SecretString(decrypted)
	return nil
}

// Value implements the driver.Valuer interface for writing encrypted data to database
func (s SecretString) Value() (driver.Value, error) {
	if s == "" {
		return "", nil
	}
	encrypted := utils.EncryptString(string(s))
	if len(encrypted) > 17 && encrypted[:17] == "encryption_error:" {
		return nil, fmt.Errorf("encryption failed: %s", encrypted[17:])
	}
	return encrypted, nil
}

type SliceString []string

func (s *SliceString) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	var encoded []byte
	switch v := value.(type) {
	case string:
		encoded = []byte(v)
	case []byte:
		encoded = v
	default:
		return fmt.Errorf("cannot scan %T into SliceString", value)
	}
	if len(encoded) == 0 {
		*s = SliceString{}
		return nil
	}
	var values []string
	if err := json.Unmarshal(encoded, &values); err != nil {
		return fmt.Errorf("decode string list: %w", err)
	}
	*s = SliceString(values)
	return nil
}

func (s SliceString) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	encoded, err := json.Marshal([]string(s))
	if err != nil {
		return nil, fmt.Errorf("encode string list: %w", err)
	}
	return string(encoded), nil
}
