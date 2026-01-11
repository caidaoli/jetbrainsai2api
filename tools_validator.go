package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bytedance/sonic"
)

// ============================================================================
// JetBrains API 工具验证和转换
// ============================================================================
//
// 为什么需要这些转换？
//
// JetBrains AI API 对工具定义有严格的限制：
// 1. 参数名称限制：最多 64 字符，仅允许 [a-zA-Z0-9_.-]
// 2. 不支持复杂 JSON Schema：anyOf/oneOf/allOf 必须简化为基础类型
// 3. 深度嵌套对象：超过 2 层嵌套会导致解析失败
// 4. 过多属性：超过 15 个属性会影响模型理解能力
//
// 本模块的职责：
// - 验证并规范化参数名称
// - 简化复杂 Schema 为 JetBrains 兼容格式
// - 缓存验证结果避免重复计算
//
// ============================================================================

var (
	paramNameRegex = regexp.MustCompile(ParamNamePattern)
)

// ============================================================================
// 内部 API
// ============================================================================

// validateAndTransformTools 验证并转换工具定义
// 纯验证函数，不涉及缓存逻辑（缓存由 RequestProcessor.ProcessTools 管理）
func validateAndTransformTools(tools []Tool, logger Logger) ([]Tool, error) {
	if len(tools) == 0 {
		return tools, nil
	}

	validatedTools := make([]Tool, 0, len(tools))
	skippedCount := 0

	for _, tool := range tools {
		// 验证工具名称
		if !isValidParamName(tool.Function.Name) {
			logger.Warn("Skipping tool with invalid name: %s (must match %s)", tool.Function.Name, ParamNamePattern)
			skippedCount++
			continue
		}

		// 转换参数 Schema
		transformedParams, err := transformParameters(tool.Function.Parameters)
		if err != nil {
			logger.Warn("Skipping tool %s: parameter transformation failed: %v", tool.Function.Name, err)
			skippedCount++
			continue
		}

		// 创建验证后的工具
		validatedTool := Tool{
			Type: tool.Type,
			Function: ToolFunction{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  transformedParams,
			},
		}

		validatedTools = append(validatedTools, validatedTool)
	}

	// 如果有工具被跳过，记录汇总信息
	if skippedCount > 0 {
		logger.Warn("Tool validation: %d/%d tools skipped due to validation errors", skippedCount, len(tools))
	}

	return validatedTools, nil
}

// ============================================================================
// 内部实现
// ============================================================================

// transformParameters 转换参数 Schema 为 JetBrains 兼容格式
func transformParameters(params map[string]any) (map[string]any, error) {
	if params == nil {
		return map[string]any{
			"type":                 SchemaTypeObject,
			"properties":           map[string]any{},
			"additionalProperties": false,
		}, nil
	}

	// 复制基础 Schema 属性
	result := make(map[string]any)
	if schemaType, ok := params["type"]; ok {
		result["type"] = schemaType
	}

	// 转换 properties
	if properties, ok := params["properties"].(map[string]any); ok {
		propCount := len(properties)

		// 超过阈值：极简化处理
		if propCount > MaxPropertiesBeforeSimplification {
			result["properties"] = simplifyComplexTool(properties)
		} else {
			// 正常转换
			transformedProps, err := transformProperties(properties)
			if err != nil {
				return nil, err
			}
			result["properties"] = transformedProps
		}
	}

	// 处理 required 字段：验证并规范化参数名
	if required, ok := params["required"].([]any); ok {
		result["required"] = validateRequiredFields(required, result["properties"])
	}

	// 禁止额外属性（更严格的验证）
	result["additionalProperties"] = false

	return result, nil
}

// simplifyComplexTool 简化复杂工具（超过 15 个属性）
// 策略：合并为单个 "data" JSON 字符串参数 + 保留前 5 个原始参数用于兼容性
func simplifyComplexTool(properties map[string]any) map[string]any {
	resultProps := map[string]any{
		"data": map[string]any{
			"type":        SchemaTypeString,
			"description": fmt.Sprintf("Provide all %d required fields as a single JSON string", len(properties)),
		},
	}

	// 保留部分原始参数以辅助 LLM 理解参数结构
	count := 0
	for propName, propSchema := range properties {
		if count >= MaxPreservedPropertiesInSimplifiedSchema {
			break
		}
		validName := propName
		if !isValidParamName(propName) {
			validName = transformParamName(propName)
		}
		if isValidParamName(validName) {
			simplified, _ := transformPropertySchema(propSchema, 0)
			resultProps[validName] = simplified
			count++
		}
	}

	return resultProps
}

// transformProperties 转换属性定义，验证名称并简化复杂 Schema
func transformProperties(properties map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for propName, propSchema := range properties {
		// 验证并转换属性名
		validName := propName
		if !isValidParamName(propName) {
			validName = transformParamName(propName)
			if !isValidParamName(validName) {
				// 跳过无法转换的属性名
				continue
			}
		}

		// 转换属性 Schema
		transformedSchema, err := transformPropertySchema(propSchema, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to transform property '%s': %v", propName, err)
		}

		result[validName] = transformedSchema
	}

	return result, nil
}

// transformPropertySchema 转换单个属性 Schema（递归处理嵌套）
func transformPropertySchema(schema any, depth int) (map[string]any, error) {
	schemaMap, ok := schema.(map[string]any)
	if !ok {
		// 非 map 类型，默认为 string
		return map[string]any{"type": SchemaTypeString}, nil
	}

	// 防止过深嵌套
	if depth > MaxNestingDepth {
		return map[string]any{
			"type":        SchemaTypeString,
			"description": "Deeply nested object - provide as JSON string",
		}, nil
	}

	result := make(map[string]any)

	// 处理 anyOf/oneOf/allOf：简化为 string + 描述
	// DRY: 统一处理所有联合类型
	for _, unionType := range []string{"anyOf", "oneOf", "allOf"} {
		if _, hasUnion := schemaMap[unionType]; hasUnion {
			return simplifyUnionType(schemaMap, unionType), nil
		}
	}

	// 获取类型
	schemaType, hasType := schemaMap["type"]
	if !hasType {
		schemaType = SchemaTypeString // 默认类型
	}
	result["type"] = schemaType

	// 根据类型处理
	switch schemaType {
	case SchemaTypeObject:
		transformObject(schemaMap, result, depth)
	case SchemaTypeArray:
		transformArray(schemaMap, result)
	default:
		// 基础类型：复制简单属性
		copySimpleProperties(schemaMap, result)
	}

	return result, nil
}

// simplifyUnionType 简化联合类型（anyOf/oneOf/allOf）为 string
func simplifyUnionType(schemaMap map[string]any, unionType string) map[string]any {
	result := map[string]any{"type": SchemaTypeString}

	// 尝试从原 Schema 提取描述
	if desc, hasDesc := schemaMap["description"]; hasDesc {
		result["description"] = desc
	} else {
		result["description"] = fmt.Sprintf("Complex type (%s) simplified to string", unionType)
	}

	return result
}

// transformObject 转换 object 类型
func transformObject(schemaMap, result map[string]any, depth int) {
	properties, hasProps := schemaMap["properties"].(map[string]any)
	if !hasProps {
		// 无 properties 定义的对象：转为 string
		result["type"] = SchemaTypeString
		result["description"] = "Object without properties - provide as JSON string"
		return
	}

	propCount := len(properties)
	if propCount > MaxPropertiesBeforeSimplification {
		// 属性过多：转为 string
		result["type"] = SchemaTypeString
		result["description"] = fmt.Sprintf("Complex object with %d properties - provide as JSON string", propCount)
		return
	}

	// 递归转换嵌套属性
	simplifiedProps := make(map[string]any)
	for propName, propSchema := range properties {
		validName := propName
		if !isValidParamName(propName) {
			validName = transformParamName(propName)
		}
		if isValidParamName(validName) {
			simplified, _ := transformPropertySchema(propSchema, depth+1)
			simplifiedProps[validName] = simplified
		}
	}

	result["type"] = SchemaTypeObject
	result["properties"] = simplifiedProps
	result["additionalProperties"] = false

	// 处理嵌套 required 字段
	if req, hasReq := schemaMap["required"].([]any); hasReq {
		result["required"] = validateRequiredFields(req, simplifiedProps)
	}
}

// transformArray 转换 array 类型
func transformArray(schemaMap, result map[string]any) {
	result["type"] = SchemaTypeArray

	// 简化 items 定义
	if items, ok := schemaMap["items"].(map[string]any); ok {
		if itemType, ok := items["type"]; ok {
			result["items"] = map[string]any{"type": itemType}
		} else {
			result["items"] = map[string]any{"type": SchemaTypeString}
		}
	} else {
		result["items"] = map[string]any{"type": SchemaTypeString}
	}
}

// copySimpleProperties 复制简单 Schema 属性
func copySimpleProperties(schemaMap, result map[string]any) {
	simpleProps := []string{
		"description", "enum", "pattern",
		"minimum", "maximum", "minLength", "maxLength",
		"minItems", "maxItems",
	}

	for _, key := range simpleProps {
		if value, ok := schemaMap[key]; ok {
			result[key] = value
		}
	}

	// 仅复制支持的 format
	if formatStr, ok := schemaMap["format"].(string); ok {
		supportedFormats := []string{"email", "uri", "date", "date-time"}
		for _, supported := range supportedFormats {
			if formatStr == supported {
				result["format"] = formatStr
				break
			}
		}
	}

	// 复制 enum
	if enum, ok := schemaMap["enum"]; ok {
		result["enum"] = enum
	}
}

// validateRequiredFields 验证 required 字段，确保所有引用的属性存在且有效
func validateRequiredFields(required []any, properties any) []string {
	var validRequired []string

	propsMap, ok := properties.(map[string]any)
	if !ok {
		return validRequired
	}

	for _, req := range required {
		reqStr, ok := req.(string)
		if !ok {
			continue
		}

		// 验证参数名
		validName := reqStr
		if !isValidParamName(reqStr) {
			validName = transformParamName(reqStr)
		}

		// 确保属性存在
		if isValidParamName(validName) && propsMap[validName] != nil {
			validRequired = append(validRequired, validName)
		}
	}

	return validRequired
}

// ============================================================================
// 参数名称验证和转换
// ============================================================================

// isValidParamName 检查参数名是否符合 JetBrains API 要求
func isValidParamName(name string) bool {
	return len(name) <= MaxParamNameLength && paramNameRegex.MatchString(name)
}

// transformParamName 转换无效参数名为有效格式
// 规则：移除非法字符，截断至 64 字符
func transformParamName(name string) string {
	var builder strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '.' || r == '-' {
			if builder.Len() < MaxParamNameLength {
				builder.WriteRune(r)
			}
		}
	}

	result := builder.String()
	if result == "" {
		result = "param" // 默认名称
	}

	// 确保不超过长度限制
	if len(result) > MaxParamNameLength {
		result = result[:MaxParamNameLength]
	}

	return result
}

// validateToolCallResponse 验证工具调用响应的完整性
func validateToolCallResponse(tc ToolCall) error {
	if tc.ID == "" {
		return fmt.Errorf("tool call ID is empty")
	}
	if tc.Function.Name == "" {
		return fmt.Errorf("tool call function name is empty")
	}
	// 可选：验证 Arguments 是有效的 JSON
	if tc.Function.Arguments != "" {
		var js any
		if err := sonic.Unmarshal([]byte(tc.Function.Arguments), &js); err != nil {
			return fmt.Errorf("tool call arguments is not valid JSON: %w", err)
		}
	}
	return nil
}

// truncateCacheKey 安全地截取缓存键用于日志显示
func truncateCacheKey(key string, maxLen int) string {
	if len(key) <= maxLen {
		return key
	}
	return key[:maxLen]
}
