package core

import "time"

// Timeout and time constants
const (
	QuotaCacheTime = 1 * time.Hour
	JWTRefreshTime = 12 * time.Hour
)

// HTTP client config constants
const (
	HTTPMaxIdleConns          = 500
	HTTPMaxIdleConnsPerHost   = 100
	HTTPMaxConnsPerHost       = 200
	HTTPIdleConnTimeout       = 600 * time.Second
	HTTPTLSHandshakeTimeout   = 30 * time.Second
	HTTPResponseHeaderTimeout = 30 * time.Second
	HTTPExpectContinueTimeout = 5 * time.Second
	HTTPRequestTimeout        = 5 * time.Minute
)

// Cache config constants
const (
	CacheDefaultCapacity      = 1000
	CacheCleanupInterval      = 5 * time.Minute
	MessageConversionCacheTTL = 10 * time.Minute
	ToolsValidationCacheTTL   = 30 * time.Minute
	CacheKeyVersion           = "v1"
)

// Stats and monitoring constants
const (
	StatsFilePath        = "stats.json"
	MinSaveInterval      = 5 * time.Second
	HistoryBufferSize    = 1000
	HistoryBatchSize     = 100
	HistoryFlushInterval = 100 * time.Millisecond
)

// Account management constants
const (
	AccountAcquireTimeout    = 60 * time.Second
	AccountExpiryWarningTime = 24 * time.Hour
	JWTExpiryCheckTime       = 1 * time.Hour
	MaxUpstreamRetries       = 3
)

// Image validation constants
const (
	MaxImageSizeBytes = 10 * 1024 * 1024
	ImageFormatPNG    = "image/png"
	ImageFormatJPEG   = "image/jpeg"
	ImageFormatGIF    = "image/gif"
	ImageFormatWebP   = "image/webp"
)

// Response body size limits
const (
	MaxResponseBodySize  = 10 * 1024 * 1024
	MaxScannerBufferSize = 1024 * 1024
)

// SupportedImageFormats supported image format list
var SupportedImageFormats = []string{ImageFormatPNG, ImageFormatJPEG, ImageFormatGIF, ImageFormatWebP}

// Tool validation constants
const (
	MaxParamNameLength                       = 64
	ParamNamePattern                         = "^[a-zA-Z0-9_.-]{1,64}$"
	MaxPropertiesBeforeSimplification        = 15
	MaxNestingDepth                          = 5
	MaxPreservedPropertiesInSimplifiedSchema = 5
)

// JSON Schema type constants
const (
	SchemaTypeObject = "object"
	SchemaTypeArray  = "array"
	SchemaTypeString = "string"
)

// Logging config constants
const (
	MaxDebugFilePathLength = 260
)

// File permission constants
const (
	FilePermissionReadWrite = 0644
)

// HTTP status code constants
const (
	HTTPStatusUnauthorized = 401
)

// Account status display constants
const (
	AccountStatusNormal   = "正常"
	AccountStatusNoQuota  = "配额不足"
	AccountStatusExpiring = "即将过期"
)

// Time format constants
const (
	TimeFormatDateTime = "2006-01-02 15:04:05"
)
