package dto

// UserDTO 用户数据传输对象
type UserDTO struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Email     string   `json:"email_address"`
	Age       int32    `json:"age"`
	CreatedAt string   `json:"create_time"`
	Status    string   `json:"status"`
	Tags      []string `json:"tags"`
}
