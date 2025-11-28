package user

import "time"

// User 用户实体
type User struct {
	ID        int       `json:"id" mapstruct:"ID"`
	Name      string    `json:"name" mapstruct:"Name"`
	Email     string    `json:"email" mapstruct:"Email"`
	Age       int       `json:"age" mapstruct:"Age"`
	CreatedAt time.Time `json:"created_at" mapstruct:"CreatedAt"`
	Status    string    `json:"status"`
	Tags      []string  `json:"tags"`
}
