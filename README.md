

# Go MapStruct

A powerful code generation tool for Go that automatically generates type-safe mapping functions between structs and maps, inspired by Java MapStruct.

## What is MapStruct?

MapStruct is a code generator that simplifies the mapping between different struct types in Go. It eliminates the need for writing repetitive boilerplate code, while providing type-safe and efficient mappings.

### Key Benefits
- **Eliminates boilerplate code** for struct-to-struct and struct-to-map conversions
- **Type-safe** at compile time, catching errors early
- **Zero runtime overhead** with generated pure Go code
- **Flexible** with multiple mapping strategies
- **Extensible** with custom type conversions

## Features

- 🚀 **Zero Runtime Overhead**: Generated code is pure Go, no reflection
- 🔒 **Type Safety**: Compile-time checked mappings
- 🔄 **Automatic Field Mapping**: Maps fields by name, JSON tags, or custom tags
- 📦 **Cross-Package Support**: Map between structs in different packages
- 📚 **Dependency Package Support**: Parse and map structs from external dependencies
- 🗺️ **Struct-Map Conversion**: Bidirectional conversion between structs and maps
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
  -verbose
```

### Batch Mode (recommended)

One `//go:generate` per package spawns one process per directive, and every
process re-parses the whole include set (typically a large ent package). With
`-config` the tool reads a typed `MapstructConfig` variable from the same file,
parses the union of all `Include` dirs once, and writes every group's output —
byte-identical to the per-directive runs, but N processes become 1.

**generate.go**
```go
package converter

// MapstructConfig 字段写错会直接编译失败；值只接受字符串字面量。
//
//go:generate mapstruct -config=generate.go
var MapstructConfig = []struct {
	Types   []string // "pkg.Source:pkg.Dest" or "...:map[string]any"
	Include []string // dirs relative to this file
	Output  string   // output file name
}{
	{
		Types:   []string{"user.User:dto.UserDTO", "req.UserCreateReq:map[string]any"},
		Include: []string{"./user", "./dto", "./req"},
		Output:  "user_generated.go",
	},
	{
		Types:   []string{"user.Order:dto.OrderDTO"},
		Include: []string{"./user", "./dto"},
		Output:  "order_generated.go",
	},
}
```

`Package` is an optional per-group field; it falls back to `-package`, then to
the `GOPACKAGE` env var set by `go generate`. Unknown fields are rejected at
generate time with a file:line position.

## Command Line Options

| Option | Description | Default |
|--------|-------------|---------|
| `-config` | Batch mode: Go file containing the `MapstructConfig` variable | Empty (single-group mode) |
| `-type` | Comma-separated type pairs (Source:Dest) | Required unless `-config` |
| `-include` | Directories to scan for structs | Current directory |
| `-package` | Package name for generated code | Auto-detected |
| `-output` | Output file name | `generated_mapstruct.go` |
| `-module` | Go module path | Auto-detected from go.mod |
| `-dependency` | Comma-separated dependency package paths | Empty |
| `-verbose` | Enable verbose logging | `false` |
| `-map-direction` | Map conversion direction: `to-map`, `from-map`, `both` | `both` |
| `-map-value-type` | Map value type: `any`, `interface{}` | `any` |

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

## Struct-Map Conversion

### Struct to Map

Generate functions to convert structs to maps:

```bash
mapstruct -type=user.User:map -map-direction=to-map
```

Generated code example:

```go
// UserToMap converts user.User to map[string]any
func UserToMap(src *user.User) map[string]any {
    if src == nil {
        return nil
    }
    return map[string]any{
        "id":         src.ID,
        "name":       src.Name,
        "email":      src.Email,
        "age":        src.Age,
        "created_at": src.CreatedAt,
        "status":     src.Status,
    }
}
```

### Map to Struct

Generate functions to convert maps to structs:

```bash
mapstruct -type=map:user.User -map-direction=from-map
```

Generated code example:

```go
// MapToUser converts map[string]any to user.User
func MapToUser(src map[string]any) *user.User {
    if src == nil {
        return nil
    }
    dst := &user.User{}
    if val, ok := src["id"]; ok {
        dst.ID = toInt(val)
    }
    if val, ok := src["name"]; ok {
        if s, ok := val.(string); ok {
            dst.Name = s
        }
    }
    // ... other fields
    return dst
}
```

### Both Directions

Generate both conversion directions:

```bash
mapstruct -type=user.User:map -map-direction=both
```

### Custom Map Value Type

Use a custom map value type:

```bash
mapstruct -type=user.User:map -map-value-type=interface{}
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
  
mapstruct -type=github.com/pkg/errors.Error:local.MyError \
          -dependency=github.com/pkg/errors \
          -include=./local \
          -output=mappers/generated.go

mapstruct -type=time.Time:custom.Timestamp \
          -include=./custom \
          -output=mappers/time_mapper.go

mapstruct -type=user.User:response.UserResponse,dto.OrderDTO:model.Order \
          -dependency=github.com/some/module/dto,github.com/some/module/model \
          -include=./user,./response \
          -output=mappers/generated.go

```

## Project Structure

```
mapstruct/
├── cmd/
│   └── mapstruct/
│       └── main.go          # Command-line interface
├── generator/
│   └── generator.go         # Code generation logic
├── parser/
│   └── parser.go            # AST parsing and struct analysis
├── examples/
│   ├── user/                # Example user struct
│   │   └── user.go
│   ├── dto/                 # Example DTO struct
│   │   └── user_dto.go
│   ├── response/            # Example response struct
│   │   └── user.go
│   ├── g.go                 # Example generate directive
│   ├── user_mappers.go      # Generated mappers
│   └── uu.go                # Additional example
├── .gitignore
├── README.md
├── go.mod
└── go.sum
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
    - Use `-verbose` flag to see mapping attempts
    - Check field names and tags match
    - Ensure fields are exported

### Debug Mode

For detailed debugging information:
```bash
mapstruct -type=... -verbose
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

- No support for embedded structs (yet)
- Custom type conversions require manual implementation
- Circular dependencies between packages may cause issues

## Acknowledgments

- Inspired by Java [MapStruct](https://mapstruct.org/)
- Built with the Go [AST](https://golang.org/pkg/go/ast/) package
