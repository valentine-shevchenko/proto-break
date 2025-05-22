# Proto-Break: Protocol Buffer Breaking Change Detector

Proto-Break is a tool for automatically detecting breaking changes in Protocol Buffer (protobuf) files. It helps ensure backward compatibility when evolving your protobuf definitions by identifying changes that could break existing clients.

## Installation

### Prerequisites

- Go 1.16 or higher

### Global Installation

```bash
# Install globally
go install github.com/valentine-shevchenko/proto-break@latest

# Now you can use the 'proto-break' command from anywhere
proto-break --help
```

### Building from Source

```bash
git clone https://github.com/valentine-shevchenko/proto-break.git
cd proto-break
go build
```

## Usage

```bash
# Validate all proto files in a directory
proto-break ./protos

# Compare with the last commit
proto-break

# Compare with a specific commit
proto-break --commit HEAD~1

# Compare with a specific commit hash
proto-break --commit abc123

# Show help
proto-break --help
```

## CI Integration

You can easily integrate Proto-Break into your CI/CD pipeline to automatically check for breaking changes:

### GitHub Actions Example

```yaml
name: Check Proto Breaking Changes

on:
  pull_request:
    paths:
      - '**/*.proto'

jobs:
  proto-break:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
        with:
          fetch-depth: 0  # Fetch all history for all branches and tags
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.20'
      
      - name: Install proto-break
        run: go install github.com/valentine-shevchenko/proto-break@latest
      
      - name: Check for breaking changes
        run: proto-break --commit ${{ github.event.pull_request.base.sha }}
        # This compares against the base branch of the PR
```

### GitLab CI Example

```yaml
proto-breaking-changes:
  stage: test
  image: golang:1.20
  before_script:
    - go install github.com/valentine-shevchenko/proto-break@latest
  script:
    - proto-break --commit $CI_MERGE_REQUEST_DIFF_BASE_SHA
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
      changes:
        - "**/*.proto"
```

## Breaking Changes Detected

Proto-Break detects the following types of breaking changes:

| Category | Breaking Change | Description | Example |
|----------|-----------------|-------------|---------|
| **Messages** | Message removal | Removing a message definition | Removing `message User {}` |
| | Nested message removal | Removing a nested message | Removing `message Inner {}` from within another message |
| **Fields** | Field removal | Removing a field from a message | Removing `string name = 1;` |
| | Field type change | Changing the type of a field | Changing `string name = 1;` to `int32 name = 1;` |
| | Field rename | Renaming a field | Changing `string name = 1;` to `string full_name = 1;` |
| | Cardinality change (repeated to singular) | Changing a repeated field to a singular field | Changing `repeated string names = 1;` to `string names = 1;` |
| **Enums** | Enum removal | Removing an enum definition | Removing `enum Status {}` |
| | Enum value removal | Removing a value from an enum | Removing `ACTIVE = 1;` from an enum |
| | Enum value rename | Renaming an enum value | Changing `ACTIVE = 1;` to `ENABLED = 1;` |
| **Services** | Service removal | Removing a service definition | Removing `service UserService {}` |
| | Method removal | Removing a method from a service | Removing `rpc GetUser(GetUserRequest) returns (User);` |
| | Method input type change | Changing the input type of a method | Changing `rpc GetUser(GetUserRequest)` to `rpc GetUser(UserRequest)` |
| | Method output type change | Changing the output type of a method | Changing `returns (User)` to `returns (UserResponse)` |
| | Method streaming change | Changing the streaming mode of a method | Changing `rpc GetUsers(GetUsersRequest) returns (stream User);` to `rpc GetUsers(GetUsersRequest) returns (User);` |
| **Packages** | Package removal | Removing a package | Removing a file that defines a unique package |

## Non-Breaking Changes

The following changes are considered safe and will not trigger warnings:

- Adding new messages, fields, enums, enum values, services, or methods
- Changing a field from singular to repeated
- Adding new packages

## Example Output

```
Found 2 modified proto files compared to HEAD
Analyzing changes in user.proto...
🔴 Detected 2 breaking changes in user.proto:
  - Field "age" (number 2) was removed from message "User"
  - Field "name" type changed from string to int32 in message "User"
Analyzing changes in service.proto...
✅ No breaking changes detected in service.proto
```

## How It Works

Proto-Break uses the jhump/protoreflect library to:

1. Parse proto files directly without requiring the protoc compiler
2. Compare the parsed file descriptors to identify breaking changes
3. Report any breaking changes found
