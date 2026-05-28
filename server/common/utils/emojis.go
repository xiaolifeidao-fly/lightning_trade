package utils

import (
	"regexp"
	"strings"
)

// 匹配 Emoji 的正则表达式（简单示例）
var emojiRegex = regexp.MustCompile(`[^\w\s,;.!?'"()-]`)

// 匹配图片标签的正则表达式 (假设是 HTML 或 Markdown 格式的图片标签)
var imgRegex = regexp.MustCompile(`(!?\[.*?\]\(.*?\))|(<img\s+.*?src=".*?".*?>)`)

// 替换文本中的 Emoji 表情和图片标签，保留文字
func RemoveEmojis(input string) string {
	// 先替换掉 Emoji 表情
	noEmojis := emojiRegex.ReplaceAllString(input, "")
	// 再替换掉图片标签
	noImages := imgRegex.ReplaceAllString(noEmojis, "")
	// 可以选择去除多余的空格
	return strings.Join(strings.Fields(noImages), " ")
}
