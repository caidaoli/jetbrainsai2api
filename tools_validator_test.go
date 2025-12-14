package main

import (
	"testing"
)

// TestSimplifyComplexTool 测试复杂工具简化
func TestSimplifyComplexTool(t *testing.T) {
	tests := []struct {
		name          string
		properties    map[string]any
		wantDataField bool
	}{
		{
			name: "少于5个属性",
			properties: map[string]any{
				"name": map[string]any{"type": "string"},
				"age":  map[string]any{"type": "integer"},
			},
			wantDataField: true,
		},
		{
			name: "正好5个属性",
			properties: map[string]any{
				"prop1": map[string]any{"type": "string"},
				"prop2": map[string]any{"type": "string"},
				"prop3": map[string]any{"type": "string"},
				"prop4": map[string]any{"type": "string"},
				"prop5": map[string]any{"type": "string"},
			},
			wantDataField: true,
		},
		{
			name: "超过5个属性只保留前5个",
			properties: map[string]any{
				"prop1": map[string]any{"type": "string"},
				"prop2": map[string]any{"type": "string"},
				"prop3": map[string]any{"type": "string"},
				"prop4": map[string]any{"type": "string"},
				"prop5": map[string]any{"type": "string"},
				"prop6": map[string]any{"type": "string"},
				"prop7": map[string]any{"type": "string"},
			},
			wantDataField: true,
		},
		{
			name: "包含非法参数名",
			properties: map[string]any{
				"valid_name":   map[string]any{"type": "string"},
				"invalid name": map[string]any{"type": "string"}, // 含空格
			},
			wantDataField: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := simplifyComplexTool(tt.properties)

			// 验证 data 字段存在
			if tt.wantDataField {
				if _, ok := result["data"]; !ok {
					t.Error("期望有 data 字段")
				}
			}

			// data字段应该是 string 类型
			if dataField, ok := result["data"].(map[string]any); ok {
				if dataField["type"] != SchemaTypeString {
					t.Errorf("data 字段类型应为 string，实际为 %v", dataField["type"])
				}
			}

			// 验证不超过6个字段（data + 最多5个原始字段）
			if len(result) > 6 {
				t.Errorf("简化后的属性数量不应超过6个，实际有 %d 个", len(result))
			}
		})
	}
}

// TestSimplifyUnionType 测试联合类型简化
func TestSimplifyUnionType(t *testing.T) {
	tests := []struct {
		name      string
		schemaMap map[string]any
		unionType string
		wantDesc  bool
	}{
		{
			name: "anyOf类型简化",
			schemaMap: map[string]any{
				"anyOf": []any{
					map[string]any{"type": "string"},
					map[string]any{"type": "integer"},
				},
			},
			unionType: "anyOf",
			wantDesc:  true,
		},
		{
			name: "oneOf类型简化",
			schemaMap: map[string]any{
				"oneOf": []any{
					map[string]any{"type": "string"},
					map[string]any{"type": "boolean"},
				},
			},
			unionType: "oneOf",
			wantDesc:  true,
		},
		{
			name: "保留原有description",
			schemaMap: map[string]any{
				"description": "原始描述",
				"anyOf": []any{
					map[string]any{"type": "string"},
				},
			},
			unionType: "anyOf",
			wantDesc:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := simplifyUnionType(tt.schemaMap, tt.unionType)

			// 验证类型被简化为 string
			if result["type"] != SchemaTypeString {
				t.Errorf("简化后类型应为 string，实际为 %v", result["type"])
			}

			// 验证有描述
			if tt.wantDesc {
				if _, ok := result["description"]; !ok {
					t.Error("应该有 description 字段")
				}
			}

			// 如果原始有描述，应该保留
			if origDesc, hasOrig := tt.schemaMap["description"]; hasOrig {
				if result["description"] != origDesc {
					t.Errorf("应保留原始描述 %v，实际为 %v", origDesc, result["description"])
				}
			}
		})
	}
}

// TestTransformParamName 测试参数名转换
func TestTransformParamName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "合法名称保持不变",
			input:    "valid_name",
			expected: "valid_name",
		},
		{
			name:     "移除空格",
			input:    "name with spaces",
			expected: "namewithspaces",
		},
		{
			name:     "移除特殊字符",
			input:    "name@#$%test",
			expected: "nametest",
		},
		{
			name:     "保留点和横线",
			input:    "name.test-value",
			expected: "name.test-value",
		},
		{
			name:     "空字符串返回默认值",
			input:    "!!!",
			expected: "param",
		},
		{
			name:     "截断超长名称",
			input:    "this_is_a_very_long_parameter_name_that_exceeds_the_maximum_length_limit_of_64_characters",
			expected: "this_is_a_very_long_parameter_name_that_exceeds_the_maximum_leng",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transformParamName(tt.input)
			if result != tt.expected {
				t.Errorf("transformParamName(%q) = %q，期望 %q", tt.input, result, tt.expected)
			}
			// 验证结果长度不超过64
			if len(result) > MaxParamNameLength {
				t.Errorf("结果长度 %d 超过最大限制 %d", len(result), MaxParamNameLength)
			}
		})
	}
}

// TestIsValidParamName 测试参数名验证
func TestIsValidParamName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"合法名称", "valid_name", true},
		{"带数字", "name123", true},
		{"带点", "name.test", true},
		{"带横线", "name-test", true},
		{"带空格", "name space", false},
		{"带特殊字符", "name@test", false},
		{"中文字符", "参数名", false},
		{"超长名称", "this_is_a_very_long_parameter_name_that_exceeds_the_maximum_length_limit", false},
		{"空字符串", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidParamName(tt.input)
			if result != tt.expected {
				t.Errorf("isValidParamName(%q) = %v，期望 %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestTransformObject 测试对象类型转换
func TestTransformObject(t *testing.T) {
	tests := []struct {
		name     string
		schema   map[string]any
		depth    int
		wantType string
	}{
		{
			name: "简单对象",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
			},
			depth:    0,
			wantType: "object",
		},
		{
			name: "无properties的对象简化为string",
			schema: map[string]any{
				"type": "object",
			},
			depth:    0,
			wantType: "string",
		},
		{
			name: "超过20个属性简化为string",
			schema: func() map[string]any {
				props := make(map[string]any)
				for i := 0; i < 25; i++ {
					props[string('a'+rune(i%26))+string('0'+rune(i/26))] = map[string]any{"type": "string"}
				}
				return map[string]any{
					"type":       "object",
					"properties": props,
				}
			}(),
			depth:    0,
			wantType: "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make(map[string]any)
			transformObject(tt.schema, result, tt.depth)
			if result["type"] != tt.wantType {
				t.Errorf("期望类型 %s，实际 %v", tt.wantType, result["type"])
			}
		})
	}
}

// TestTransformArray 测试数组类型转换
func TestTransformArray(t *testing.T) {
	tests := []struct {
		name   string
		schema map[string]any
	}{
		{
			name: "带items的数组",
			schema: map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
		{
			name: "无items的数组",
			schema: map[string]any{
				"type": "array",
			},
		},
		{
			name: "items无type字段",
			schema: map[string]any{
				"type":  "array",
				"items": map[string]any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make(map[string]any)
			transformArray(tt.schema, result)
			if result["type"] != "array" {
				t.Errorf("期望类型 array，实际 %v", result["type"])
			}
			// 验证items存在
			if _, ok := result["items"]; !ok {
				t.Error("应该有 items 字段")
			}
		})
	}
}

// TestCopySimpleProperties 测试简单属性复制
func TestCopySimpleProperties(t *testing.T) {
	tests := []struct {
		name     string
		src      map[string]any
		wantKeys []string
	}{
		{
			name: "复制description",
			src: map[string]any{
				"description": "测试描述",
				"type":        "string",
			},
			wantKeys: []string{"description"},
		},
		{
			name: "复制多个字段",
			src: map[string]any{
				"description": "描述",
				"enum":        []string{"a", "b"},
				"pattern":     "^[a-z]+$",
			},
			wantKeys: []string{"description", "enum", "pattern"},
		},
		{
			name: "复制支持的format",
			src: map[string]any{
				"format": "email",
			},
			wantKeys: []string{"format"},
		},
		{
			name: "忽略不支持的format",
			src: map[string]any{
				"format": "unsupported-format",
			},
			wantKeys: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := make(map[string]any)
			copySimpleProperties(tt.src, dest)

			for _, key := range tt.wantKeys {
				if _, ok := dest[key]; !ok {
					t.Errorf("期望字段 %s 存在", key)
				}
			}
		})
	}
}

// TestValidateRequiredFields 测试必填字段验证
func TestValidateRequiredFields(t *testing.T) {
	tests := []struct {
		name       string
		required   []any
		properties map[string]any
		wantCount  int
	}{
		{
			name:     "全部有效的必填字段",
			required: []any{"name", "age"},
			properties: map[string]any{
				"name": map[string]any{"type": "string"},
				"age":  map[string]any{"type": "integer"},
			},
			wantCount: 2,
		},
		{
			name:     "部分字段不存在",
			required: []any{"name", "missing"},
			properties: map[string]any{
				"name": map[string]any{"type": "string"},
			},
			wantCount: 1,
		},
		{
			name:       "空必填列表",
			required:   []any{},
			properties: map[string]any{"name": map[string]any{"type": "string"}},
			wantCount:  0,
		},
		{
			name:     "非法参数名需转换",
			required: []any{"valid_name", "invalid name"},
			properties: map[string]any{
				"valid_name":   map[string]any{"type": "string"},
				"invalidname":  map[string]any{"type": "string"},
				"invalid name": map[string]any{"type": "string"},
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateRequiredFields(tt.required, tt.properties)
			if len(result) != tt.wantCount {
				t.Errorf("期望 %d 个有效必填字段，实际 %d 个: %v", tt.wantCount, len(result), result)
			}
		})
	}
}

// TestTransformPropertySchema 测试属性Schema转换
func TestTransformPropertySchema(t *testing.T) {
	tests := []struct {
		name     string
		schema   any
		depth    int
		wantType string
	}{
		{
			name:     "字符串类型",
			schema:   map[string]any{"type": "string"},
			depth:    0,
			wantType: "string",
		},
		{
			name:     "整数类型",
			schema:   map[string]any{"type": "integer"},
			depth:    0,
			wantType: "integer",
		},
		{
			name:     "布尔类型",
			schema:   map[string]any{"type": "boolean"},
			depth:    0,
			wantType: "boolean",
		},
		{
			name: "anyOf类型简化",
			schema: map[string]any{
				"anyOf": []any{
					map[string]any{"type": "string"},
					map[string]any{"type": "integer"},
				},
			},
			depth:    0,
			wantType: "string",
		},
		{
			name: "oneOf类型简化",
			schema: map[string]any{
				"oneOf": []any{
					map[string]any{"type": "string"},
				},
			},
			depth:    0,
			wantType: "string",
		},
		{
			name:     "nil输入",
			schema:   nil,
			depth:    0,
			wantType: "string",
		},
		{
			name:     "非map输入",
			schema:   "invalid",
			depth:    0,
			wantType: "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := transformPropertySchema(tt.schema, tt.depth)
			if result["type"] != tt.wantType {
				t.Errorf("期望类型 %s，实际 %v", tt.wantType, result["type"])
			}
		})
	}
}

// TestTransformProperties 测试属性集合转换
func TestTransformProperties(t *testing.T) {
	tests := []struct {
		name       string
		properties map[string]any
		wantKeys   []string
	}{
		{
			name: "简单属性",
			properties: map[string]any{
				"name": map[string]any{"type": "string"},
				"age":  map[string]any{"type": "integer"},
			},
			wantKeys: []string{"name", "age"},
		},
		{
			name: "非法参数名会被转换",
			properties: map[string]any{
				"valid_name":   map[string]any{"type": "string"},
				"invalid name": map[string]any{"type": "string"},
			},
			wantKeys: []string{"valid_name", "invalidname"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transformProperties(tt.properties)
			if err != nil {
				t.Fatalf("不期望错误: %v", err)
			}
			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("期望键 %s 存在", key)
				}
			}
		})
	}
}

// TestTransformParameters 测试参数转换
func TestTransformParameters(t *testing.T) {
	tests := []struct {
		name     string
		params   map[string]any
		wantType string
		wantErr  bool
	}{
		{
			name: "简单参数",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
			},
			wantType: "object",
			wantErr:  false,
		},
		{
			name:     "空参数",
			params:   nil,
			wantType: "object",
			wantErr:  false,
		},
		{
			name: "包含anyOf的参数",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"value": map[string]any{
						"anyOf": []any{
							map[string]any{"type": "string"},
							map[string]any{"type": "integer"},
						},
					},
				},
			},
			wantType: "object",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transformParameters(tt.params)
			if tt.wantErr {
				if err == nil {
					t.Error("期望有错误")
				}
				return
			}
			if err != nil {
				t.Fatalf("不期望错误: %v", err)
			}
			if result["type"] != tt.wantType {
				t.Errorf("期望类型 %s，实际 %v", tt.wantType, result["type"])
			}
		})
	}
}
