package examples

//go:generate go run ../cmd/mapstruct -type=user.User:dto.UserDTO,user.User:response.UserResponse,user.User:UserResponse1 -include=user,dto,response,./ -package=examples -output=user_mappers.go -verbose
