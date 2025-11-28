

# Go MapStruct

A powerful code generation tool for Go that automatically generates type-safe mapping functions between structs, inspired by Java MapStruct.

## Features

- 🚀 **Zero Runtime Overhead**: Generated code is pure Go, no reflection
- 🔒 **Type Safety**: Compile-time checked mappings
- 🔄 **Automatic Field Mapping**: Maps fields by name, JSON tags, or custom tags
- 📦 **Cross-Package Support**: Map between structs in different packages
- ⚡ **Custom Type Conversion**: Built-in support for common type conversions
- 🎯 **Flexible Configuration**: Multiple mapping strategies and customizations
- 🔍 **Comprehensive Debugging**: Verbose and debug modes for troubleshooting

## Installation

### Using go install
```bash
go install github.com/shunshouda/mapstruct/cmd/mapstruct@latest
```

### From source
```bash
git clone https://github.com/shunshouda/mapstruct
cd mapstruct
go install ./cmd/mapstruct
```

## Quick Start

### 1. Define Your Structs

**user/user.go**
```go
package user

import "time"

type User struct {
    ID        int       `json:"id"`
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    Age       int       `json:"age"`
    CreatedAt time.Time `json:"created_at"`
    Status    string    `json:"status"`
}
```

**dto/user_dto.go**
```go
package dto

type UserDTO struct {
    ID        int    `json:"id"`
    Name      string `json:"name"`
    Email     string `json:"email"`
    Age       int32  `json:"age"`
    CreatedAt string `json:"created_at"`
}
```

### 2. Add Generate Directive

Create a file with go generate directive:

**mappers/generate.go**
```go
package mappers

//go:generate mapstruct -type=user.User:dto.UserDTO -include=../user,../dto -package=mappers -output=generated.go
```

### 3. Generate Mappers
```bash
go generate ./...
```

### 4. Use Generated Mappers
```go
package main

import (
    "fmt"
    "time"
    
    "your-module/user"
    "your-module/dto" 
    "your-module/mappers"
)

func main() {
    user := &user.User{
        ID:        1,
        Name:      "John Doe",
        Email:     "john@example.com",
        Age:       30,
        CreatedAt: time.Now(),
        Status:    "active",
    }

    // Use the generated mapper
    userDTO := mappers.UserToUserDTO(user)
    
    fmt.Printf("UserDTO: %+v\n", userDTO)
}
```

## Usage

### Basic Usage

Generate mappers between structs in the same package:
```bash
mapstruct -type=Source:Destination
```

### Cross-Package Mapping

Generate mappers between structs in different packages:
```bash
mapstruct -type=package1.Source:package2.Destination -include=./pkg1,./pkg2
```

### Multiple Mappings

Generate multiple mapper functions at once:
```bash
mapstruct -type=user.User:dto.UserDTO,user.User:response.UserResponse -include=user,dto,response
```

### Full Options

```bash
mapstruct \
  -type=user.User:dto.UserDTO \
  -include=./internal/user,./internal/dto \
  -package=mappers \
  -output=./mappers/generated.go \
  -verbose \
  -debug
```

## Command Line Options

| Option | Description | Default |
|--------|-------------|---------|
| `-type` | Comma-separated type pairs (Source:Dest) | **Required** |
| `-include` | Directories to scan for structs | Current directory |
| `-package` | Package name for generated code | Auto-detected |
| `-output` | Output file name | `generated_mapstruct.go` |
| `-module` | Go module path | Auto-detected from go.mod |
| `-verbose` | Enable verbose logging | `false` |
| `-debug` | Enable debug output | `false` |

## Mapping Strategies

The tool uses multiple strategies to map fields:

### 1. Tag-based Mapping
```go
type Source struct {
    UserID int `mapstruct:"ID"`
}

type Destination struct {
    ID int `mapstruct:"UserID"` // Maps to Source.UserID
}
```

### 2. JSON Tag Mapping
```go
type Source struct {
    UserID int `json:"user_id"`
}

type Destination struct {
    UserID int `json:"user_id"` // Automatically mapped by JSON tag
}
```

### 3. Field Name Mapping
```go
type Source struct {
    Name string
}

type Destination struct {
    Name string // Automatically mapped by field name
}
```

## Type Conversions

### Automatic Conversions
- `int` ↔ `int8`, `int16`, `int32`, `int64`
- `float32` ↔ `float64`
- `string` ↔ `[]byte`
- `time.Time` ↔ `string` (using RFC3339 format)
- Pointer ↔ Non-pointer types

### Custom Type Converters
The generated code includes common type conversions. For complex conversions, you can extend the generated code:

```go
// Custom conversion logic can be added after generation
func customUserToDTO(user *user.User) *dto.UserDTO {
    dto := UserToUserDTO(user)
    // Add custom logic
    dto.CustomField = calculateCustomValue(user)
    return dto
}
```

## Advanced Examples

### Multiple Source Types
```bash
mapstruct -type=user.User:dto.UserDTO,admin.Admin:dto.AdminDTO
```

### Mixed Package Mapping
```bash
mapstruct -type=user.User:UserResponse,internal.Data:api.Response
```

### Complex Project Structure
```bash
mapstruct \
  -type=domain.User:transport.UserResponse,domain.Product:transport.ProductResponse \
  -include=./internal/domain,./internal/transport \
  -package=transport \
  -output=./internal/transport/mappers.go \
  -module=github.com/your-company/your-project
```

## Project Structure

```
mapstruct/
├── cmd/
│   └── mapstruct/
│       └── main.go
├── generator/
│   └── generator.go
├── parser/
│   └── parser.go
├── examples/
│   ├── user/
│   │   └── user.go
│   ├── dto/
│   │   └── user_dto.go
│   └── mappers/
│       └── generate.go
└── go.mod
```

## Troubleshooting

### Common Issues

1. **"Structure not found" errors**
    - Use `-verbose` flag to see scanned directories
    - Ensure `-include` paths are correct
    - Check that structs are exported (capitalized)

2. **Import path issues**
    - Specify `-module` if auto-detection fails
    - Ensure go.mod file exists in project root

3. **Field mapping not working**
    - Use `-debug` flag to see mapping attempts
    - Check field names and tags match
    - Ensure fields are exported

### Debug Mode

For detailed debugging information:
```bash
mapstruct -type=... -verbose -debug
```

This will show:
- Scanned directories and files
- Found structs and their packages
- Mapping attempts and results
- Generated import statements

## Best Practices

1. **Use Explicit Tags**: Prefer `mapstruct` tags for unambiguous mapping
2. **Organize by Domain**: Group related mappers together
3. **Version Control**: Commit generated files for reproducible builds
4. **CI Integration**: Run `go generate` in your CI pipeline
5. **Custom Extensions**: Add custom logic in separate files, not in generated code

## Limitations

- Only supports struct-to-struct mapping
- No support for embedded structs (yet)
- Custom type conversions require manual implementation
- Circular dependencies between packages may cause issues

## Acknowledgments

- Inspired by Java [MapStruct](https://mapstruct.org/)
- Built with the Go [AST](https://golang.org/pkg/go/ast/) package
