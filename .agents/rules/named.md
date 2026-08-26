# Go Naming Best Practices

## 1. Package Naming

- **All lowercase, no underscores**: `package user`, not `package userService` or `package user_service`
- **Short and meaningful**: `package http`, `package json`, `package dao`
- **Avoid plurals**: `package user` not `package users`
- **Avoid generic names**: Avoid `package util`, `package common`, `package base`

```go
// Recommended
package user
package handler
package service

// Not recommended
package UserService
package user_service
package utils
```

## 2. File Naming

- **All lowercase, underscore separated**: `user_handler.go`, `user_service.go`
- **Test files**: `user_handler_test.go`
- **Platform-specific**: `user_linux.go`, `user_windows.go`

```
user/
├── user_handler.go
├── user_service.go
├── user_dao.go
└── user_test.go
```

## 3. Directory Naming

- **All lowercase, no underscores or hyphens**: `internal/`, `pkg/`, `cmd/`
- **Short and descriptive**: `handler/`, `service/`, `dao/`

```
project/
├── cmd/                    # Main entry point
│   └── server_main.go
├── internal/               # Private code
│   ├── handler/
│   ├── service/
│   ├── dao/
│   ├── model/
│   └── middleware/
├── pkg/                    # Public code
└── api/                    # API definitions
```

## 4. Interface Naming

- **Single-method interfaces end with "-er"**: `Reader`, `Writer`, `Handler`
- **Verb form**: `Reader`, `Executor`, `Validator`

```go
// Recommended
type Reader interface {
    Read(p []byte) (n int, err error)
}

type UserService interface {
    Register(req *RegisterRequest) (*User, error)
    Login(req *LoginRequest) (*User, error)
}

// Not recommended
type UserInterface interface {}
type IUserService interface {}
```

## 5. Struct Naming

- **CamelCase**: `UserService`, `UserHandler`
- **Avoid redundant prefixes**: `User` not `UserModel`

```go
// Recommended
type UserService struct {}
type UserHandler struct {}
type RegisterRequest struct {}

// Not recommended
type user_service struct {}
type SUserService struct {}
type UserModel struct {}
```

## 6. Method/Function Naming

- **CamelCase**
- **Start with verb**: `GetUser`, `CreateUser`, `DeleteUser`
- **Boolean returns use Is/Has/Can prefix**: `IsValid`, `HasPermission`

```go
// Recommended
func (s *UserService) Register(req *RegisterRequest) (*User, error)
func (s *UserService) GetUserByID(id uint) (*User, error)
func (s *UserService) IsEmailExists(email string) bool

// Not recommended
func (s *UserService) register_user()
func (s *UserService) get_user_by_id()
func (s *UserService) CheckEmailExists() // Should use Is/Has
```

## 7. Constant Naming

- **CamelCase**: `const MaxRetryCount = 3`
- **Enum constants**: `const StatusActive = "active"`

```go
// Recommended
const (
    StatusActive   = "1"
    StatusInactive = "0"
    MaxRetryCount  = 3
)

// Not recommended
const (
    STATUS_ACTIVE = "1"  // Not all uppercase
    status_active = "1"  // Not all lowercase
)
```

## 8. Error Variable Naming

- **Start with "Err"**: `ErrNotFound`, `ErrInvalidInput`

```go
// Recommended
var (
    ErrNotFound      = errors.New("not found")
    ErrInvalidInput  = errors.New("invalid input")
    ErrUnauthorized  = errors.New("unauthorized")
)
```

## 9. Acronyms Keep Consistent Case

```go
// Recommended
type HTTPHandler struct {}
var URL string
func GetHTTPClient() {}
func ParseJSON() {}

// Not recommended
type HttpHandler struct {}
var Url string
func GetHttpClient() {}
```

## 10. Project Structure Naming

```
project-name/
├── cmd/                    # Main programs
│   └── app_name/
│       └── main.go
├── internal/               # Private code
│   ├── handler/           # HTTP handlers
│   ├── service/           # Business logic
│   ├── repository/        # Data access
│   ├── model/             # Data models
│   └── config/            # Configuration
├── pkg/                    # Public code
├── api/                    # API definitions
├── configs/               # Config files
├── scripts/               # Scripts
├── docs/                  # Documentation
├── go.mod
└── go.sum
```

## Summary Table

| Type           | Rule                                | Example             |
| -------------- | ----------------------------------- | ------------------- |
| Package        | All lowercase, no underscores       | `package user`      |
| File           | All lowercase, underscore separated | `user_service.go`   |
| Directory      | All lowercase, no separators        | `internal/handler/` |
| Struct         | CamelCase, capitalized first letter | `UserService`       |
| Interface      | CamelCase, -er suffix               | `Reader`, `Writer`  |
| Method         | CamelCase, verb prefix              | `GetUserByID`       |
| Constant       | CamelCase                           | `MaxRetryCount`     |
| Error Variable | Err prefix                          | `ErrNotFound`       |
