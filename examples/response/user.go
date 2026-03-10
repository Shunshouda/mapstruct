package response

import "github.com/shunshouda/mapstruct/examples/dto"

// UserResponse 用户API响应
type UserResponse struct {
	dto.UserDTO
	//ID     int    `json:"id"`
	//Name   string `json:"name"`
	Email  string        `json:"email"`
	Age    int           `json:"age"`
	Status string        `json:"status"`
	UU     *UserResponse `json:"uu"`
}
