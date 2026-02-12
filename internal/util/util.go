package util

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"jetbrainsai2api/internal/core"

	"github.com/bytedance/sonic"
)

// MarshalJSON wraps Sonic for performance
func MarshalJSON(v any) ([]byte, error) {
	return sonic.Marshal(v)
}

// GenerateID generates a prefixed unique ID (based on nanosecond timestamp)
func GenerateID(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
}

// GenerateRandomID generates a prefixed random ID (crypto-secure)
func GenerateRandomID(prefix string) string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s%s", prefix, hex.EncodeToString(b))
}

// CreateJetbrainsRequest creates a JetBrains API HTTP request with standard headers
func CreateJetbrainsRequest(method, url string, payload any, authorization string) (*http.Request, error) {
	var body io.Reader

	if payload != nil {
		payloadBytes, err := MarshalJSON(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewBuffer(payloadBytes)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set(core.HeaderContentType, core.ContentTypeJSON)
	if authorization != "" {
		req.Header.Set("authorization", core.AuthBearerPrefix+authorization)
	}

	return req, nil
}

// ExtractTextContent extracts text from message content field
func ExtractTextContent(content any) string {
	if content == nil {
		return ""
	}

	switch v := content.(type) {
	case string:
		return v
	case []any:
		var textParts []string
		for _, item := range v {
			if itemMap, ok := item.(map[string]any); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == core.ContentBlockTypeText {
					if text, ok := itemMap["text"].(string); ok {
						textParts = append(textParts, text)
					}
				}
			}
		}
		return strings.Join(textParts, " ")
	}
	return ""
}

// TruncateString truncates string and adds replacement text in the middle
func TruncateString(s string, prefixLen, suffixLen int, replacement string) string {
	if len(s) > prefixLen+suffixLen {
		return s[:prefixLen] + replacement + s[len(s)-suffixLen:]
	}
	return s
}

// EstimateTokenCount provides a rough token count estimation.
// Uses rune count for better accuracy with multi-byte characters.
func EstimateTokenCount(text string) int {
	runeCount := utf8.RuneCountInString(text)
	if runeCount == 0 {
		return 0
	}
	// Rough estimation: ~0.6 tokens per rune for mixed CJK/Latin text
	return max(1, runeCount*3/5)
}

// ParseJWTExpiry parses JWT expiry time from the token's exp claim.
func ParseJWTExpiry(tokenStr string) (time.Time, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("could not parse JWT: %w", err)
	}

	claims := make(map[string]any)
	if err := sonic.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, fmt.Errorf("could not parse JWT: %w", err)
	}

	var expUnix int64
	switch v := claims["exp"].(type) {
	case float64:
		expUnix = int64(v)
	case float32:
		expUnix = int64(v)
	case int:
		expUnix = int64(v)
	case int64:
		expUnix = v
	case int32:
		expUnix = int64(v)
	default:
		return time.Time{}, fmt.Errorf("JWT missing exp claim")
	}

	return time.Unix(expUnix, 0), nil
}

// ParseEnvList parses comma-separated env var to trimmed slice
func ParseEnvList(envVar string) []string {
	if envVar == "" {
		return nil
	}
	parts := strings.Split(envVar, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// GetEnvWithDefault gets env var with default value
func GetEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetTokenDisplayName gets account display name for logging
func GetTokenDisplayName(account *core.JetbrainsAccount) string {
	if account == nil {
		return "Token Unknown"
	}

	account.Lock()
	jwt := account.JWT
	licenseID := account.LicenseID
	account.Unlock()

	if jwt != "" {
		return TruncateString(jwt, 0, 6, "Token ...")
	}
	if licenseID != "" {
		return TruncateString(licenseID, 0, 6, "Token ...")
	}
	return "Token Unknown"
}

// GetLicenseDisplayName gets license display name for stats page
func GetLicenseDisplayName(account *core.JetbrainsAccount) string {
	if account == nil {
		return "Unknown"
	}

	account.Lock()
	authorization := account.Authorization
	account.Unlock()

	if authorization != "" {
		return TruncateString(authorization, 3, 3, "*")
	}
	return "Unknown"
}

// GetTokenInfoFromAccount gets account token info for stats page
func GetTokenInfoFromAccount(account *core.JetbrainsAccount, quotaData *core.JetbrainsQuotaResponse, err error) *core.TokenInfo {
	if err != nil {
		return &core.TokenInfo{
			Name:   GetTokenDisplayName(account),
			Status: "错误",
		}
	}

	dailyUsed := 0.0
	dailyTotal := 0.0
	if quotaData != nil {
		_, _ = fmt.Sscanf(quotaData.Current.Current.Amount, "%f", &dailyUsed)
		_, _ = fmt.Sscanf(quotaData.Current.Maximum.Amount, "%f", &dailyTotal)
	}

	var usageRate float64
	if dailyTotal > 0 {
		usageRate = (dailyUsed / dailyTotal) * 100
	}

	account.Lock()
	hasQuota := account.HasQuota
	expiryTime := account.ExpiryTime
	account.Unlock()

	status := core.AccountStatusNormal
	if !hasQuota {
		status = core.AccountStatusNoQuota
	} else if time.Now().Add(core.AccountExpiryWarningTime).After(expiryTime) {
		status = core.AccountStatusExpiring
	}

	return &core.TokenInfo{
		Name:       GetTokenDisplayName(account),
		License:    GetLicenseDisplayName(account),
		Used:       dailyUsed,
		Total:      dailyTotal,
		UsageRate:  usageRate,
		ExpiryDate: expiryTime,
		Status:     status,
		HasQuota:   hasQuota,
	}
}
