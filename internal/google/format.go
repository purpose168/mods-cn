package google

import "github.com/charmbracelet/mods/internal/proto"

// fromProtoMessages 将协议层的消息列表转换为 Google API 的 Content 格式。
// 该函数处理系统消息和用户消息，将它们统一转换为用户角色的内容。
// 参数：
//   - input: 协议层的消息列表
// 返回：
//   - []Content: 转换后的 Google API Content 列表
func fromProtoMessages(input []proto.Message) []Content {
	// 预分配结果切片，提高性能
	result := make([]Content, 0, len(input))
	// 遍历输入消息列表
	for _, in := range input {
		// 根据消息角色进行处理
		switch in.Role {
		case proto.RoleSystem, proto.RoleUser:
			// 将系统消息和用户消息都转换为用户角色的内容
			result = append(result, Content{
				Role:  proto.RoleUser,
				Parts: []Part{{Text: in.Content}},
			})
		}
	}
	return result
}
