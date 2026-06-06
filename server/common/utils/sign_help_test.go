package utils

import "testing"

func TestEh(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int32
	}{
		{
			name:     "空字符串",
			input:    "",
			expected: 0,
		},
		{
			name:     "单个字符",
			input:    "a",
			expected: 97, // 'a' 的 ASCII 码
		},
		{
			name:     "简单字符串",
			input:    "hello",
			expected: 99162322, // 根据算法计算的哈希值
		},
		{
			name:     "数字字符串",
			input:    "123",
			expected: 48690, // 根据算法计算的哈希值
		},
		{
			name:     "包含特殊字符",
			input:    "test@123",
			expected: -1422447899, // 根据算法计算的哈希值
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetExt(tt.input)
			if result != tt.expected {
				t.Errorf("Eh(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

// 手动验证计算过程
func TestEhManualCalculation(t *testing.T) {
	// 测试 "ab" 的哈希值
	// 第一个字符 'a' (97): i = 0 * 31 + 97 = 97
	// 第二个字符 'b' (98): i = 97 * 31 + 98 = 3007 + 98 = 3105
	input := "ab"
	expected := int32(3105)
	result := GetExt(input)
	if result != expected {
		t.Errorf("Eh(%q) = %d, want %d", input, result, expected)
	}
}
