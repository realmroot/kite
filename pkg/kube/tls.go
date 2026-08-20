package kube

import (
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strings"

	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
)

func NormalizeCABundle(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	data := []byte(value)
	if !bytes.Contains(data, []byte("BEGIN CERTIFICATE")) {
		decoded, err := base64.StdEncoding.Strict().DecodeString(value)
		if err != nil {
			return nil, errors.New("CA bundle must be PEM or base64-encoded PEM")
		}
		data = decoded
	}
	if err := ValidateCAData(data); err != nil {
		return nil, err
	}
	return data, nil
}

func ValidateCAData(data []byte) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	rest := data
	certificates := 0
	for len(bytes.TrimSpace(rest)) > 0 {
		block, remaining := pem.Decode(rest)
		if block == nil {
			return errors.New("CA bundle contains invalid PEM data")
		}
		if block.Type != "CERTIFICATE" {
			return fmt.Errorf("CA bundle contains unsupported PEM block %q", block.Type)
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return fmt.Errorf("CA bundle contains an invalid certificate: %w", err)
		}
		certificates++
		rest = remaining
	}
	if certificates == 0 {
		return errors.New("CA bundle contains no certificates")
	}
	return nil
}

func ValidateTLSServerName(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || net.ParseIP(value) != nil {
		return nil
	}
	if errs := utilvalidation.IsDNS1123Subdomain(value); len(errs) != 0 {
		return fmt.Errorf("TLS server name must be an IP address or DNS name: %s", strings.Join(errs, "; "))
	}
	return nil
}
