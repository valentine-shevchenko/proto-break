package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestCompareFields tests the compareFields function
func TestCompareFields(t *testing.T) {
	// Create test cases for breaking changes
	tests := []struct {
		name           string
		prevProto      string
		currProto      string
		expectedErrors []string
	}{
		{
			name: "Field type change",
			prevProto: `
				syntax = "proto3";
				package test;
				message TestMessage {
					string name = 1;
				}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message TestMessage {
					int64 name = 1;
				}
			`,
			expectedErrors: []string{
				`Field "name" type changed from string to int64 in message "TestMessage"`,
			},
		},
		{
			name: "Field removal",
			prevProto: `
				syntax = "proto3";
				package test;
				message TestMessage {
					string name = 1;
					int32 age = 2;
				}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message TestMessage {
					string name = 1;
				}
			`,
			expectedErrors: []string{
				`Field "age" (number 2) was removed from message "TestMessage"`,
			},
		},
		{
			name: "Field rename",
			prevProto: `
				syntax = "proto3";
				package test;
				message TestMessage {
					string name = 1;
				}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message TestMessage {
					string full_name = 1;
				}
			`,
			expectedErrors: []string{
				`Field renamed from "name" to "full_name" in message "TestMessage"`,
			},
		},
		{
			name: "Cardinality change (repeated to singular)",
			prevProto: `
				syntax = "proto3";
				package test;
				message TestMessage {
					repeated string names = 1;
				}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message TestMessage {
					string names = 1;
				}
			`,
			expectedErrors: []string{
				`Field "names" cardinality changed from repeated to singular in message "TestMessage"`,
			},
		},
		{
			name: "Multiple breaking changes",
			prevProto: `
				syntax = "proto3";
				package test;
				message TestMessage {
					string name = 1;
					int32 age = 2;
					repeated string hobbies = 3;
				}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message TestMessage {
					int64 name = 1;
					string hobbies = 3;
				}
			`,
			expectedErrors: []string{
				`Field "name" type changed from string to int64 in message "TestMessage"`,
				`Field "age" (number 2) was removed from message "TestMessage"`,
				`Field "hobbies" cardinality changed from repeated to singular in message "TestMessage"`,
			},
		},
		// Non-breaking changes
		{
			name: "Adding new field (non-breaking)",
			prevProto: `
				syntax = "proto3";
				package test;
				message TestMessage {
					string name = 1;
				}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message TestMessage {
					string name = 1;
					int32 age = 2;
				}
			`,
			expectedErrors: []string{},
		},
		{
			name: "Changing field from singular to repeated (non-breaking)",
			prevProto: `
				syntax = "proto3";
				package test;
				message TestMessage {
					string name = 1;
				}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message TestMessage {
					repeated string name = 1;
				}
			`,
			expectedErrors: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary proto files
			prevFile, err := createTempProtoFile(tt.prevProto)
			if err != nil {
				t.Fatalf("Failed to create previous proto file: %v", err)
			}
			defer os.Remove(prevFile)

			currFile, err := createTempProtoFile(tt.currProto)
			if err != nil {
				t.Fatalf("Failed to create current proto file: %v", err)
			}
			defer os.Remove(currFile)

			// Parse proto files directly using protoparse
			prevFileDesc, err := parseProtoFileToReflect(prevFile)
			if err != nil {
				t.Fatalf("Failed to parse previous proto file: %v", err)
			}

			currFileDesc, err := parseProtoFileToReflect(currFile)
			if err != nil {
				t.Fatalf("Failed to parse current proto file: %v", err)
			}

			// Get file descriptors
			prevFile1 := prevFileDesc
			currFile1 := currFileDesc

			// Compare messages
			var actualErrors []string
			prevMsgs := prevFile1.Messages()
			currMsgs := currFile1.Messages()

			for i := 0; i < prevMsgs.Len(); i++ {
				prevMsg := prevMsgs.Get(i)
				msgName := string(prevMsg.Name())

				// Find corresponding message in current file
				var currMsg protoreflect.MessageDescriptor
				for j := 0; j < currMsgs.Len(); j++ {
					if string(currMsgs.Get(j).Name()) == msgName {
						currMsg = currMsgs.Get(j)
						break
					}
				}

				if currMsg != nil {
					errors := compareFields(prevMsg, currMsg)
					actualErrors = append(actualErrors, errors...)
				}
			}

			// Sort errors for consistent comparison
			sort.Strings(actualErrors)
			sort.Strings(tt.expectedErrors)

			// Compare results
			if len(actualErrors) == 0 && len(tt.expectedErrors) == 0 {
				// Both are empty, test passes
			} else if !reflect.DeepEqual(actualErrors, tt.expectedErrors) {
				t.Errorf("Expected errors %v, got %v", tt.expectedErrors, actualErrors)
			}
		})
	}
}

// TestCompareEnums tests the compareEnums function
func TestCompareEnums(t *testing.T) {
	tests := []struct {
		name           string
		prevProto      string
		currProto      string
		expectedErrors []string
	}{
		{
			name: "Enum removal",
			prevProto: `
				syntax = "proto3";
				package test;
				enum Status {
					UNKNOWN = 0;
					ACTIVE = 1;
					INACTIVE = 2;
				}
				message TestMessage {}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message TestMessage {}
			`,
			expectedErrors: []string{
				`Enum "Status" was removed`,
			},
		},
		{
			name: "Enum value removal",
			prevProto: `
				syntax = "proto3";
				package test;
				enum Status {
					UNKNOWN = 0;
					ACTIVE = 1;
					INACTIVE = 2;
				}
				message TestMessage {}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				enum Status {
					UNKNOWN = 0;
					ACTIVE = 1;
				}
				message TestMessage {}
			`,
			expectedErrors: []string{
				`Enum value "INACTIVE" (number 2) was removed from enum "Status"`,
			},
		},
		{
			name: "Enum value rename",
			prevProto: `
				syntax = "proto3";
				package test;
				enum Status {
					UNKNOWN = 0;
					ACTIVE = 1;
				}
				message TestMessage {}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				enum Status {
					UNKNOWN = 0;
					ENABLED = 1;
				}
				message TestMessage {}
			`,
			expectedErrors: []string{
				`Enum value renamed from "ACTIVE" to "ENABLED" in enum "Status"`,
			},
		},
		// Non-breaking changes
		{
			name: "Adding new enum value (non-breaking)",
			prevProto: `
				syntax = "proto3";
				package test;
				enum Status {
					UNKNOWN = 0;
					ACTIVE = 1;
				}
				message TestMessage {}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				enum Status {
					UNKNOWN = 0;
					ACTIVE = 1;
					INACTIVE = 2;
				}
				message TestMessage {}
			`,
			expectedErrors: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary proto files
			prevFile, err := createTempProtoFile(tt.prevProto)
			if err != nil {
				t.Fatalf("Failed to create previous proto file: %v", err)
			}
			defer os.Remove(prevFile)

			currFile, err := createTempProtoFile(tt.currProto)
			if err != nil {
				t.Fatalf("Failed to create current proto file: %v", err)
			}
			defer os.Remove(currFile)

			// Parse proto files directly using protoparse
			prevFileDesc, err := parseProtoFileToReflect(prevFile)
			if err != nil {
				t.Fatalf("Failed to parse previous proto file: %v", err)
			}

			currFileDesc, err := parseProtoFileToReflect(currFile)
			if err != nil {
				t.Fatalf("Failed to parse current proto file: %v", err)
			}

			// Get file descriptors
			prevFile1 := prevFileDesc
			currFile1 := currFileDesc

			// Compare enums
			actualErrors := compareEnums(prevFile1, currFile1)

			// Sort errors for consistent comparison
			sort.Strings(actualErrors)
			sort.Strings(tt.expectedErrors)

			// Compare results
			if len(actualErrors) == 0 && len(tt.expectedErrors) == 0 {
				// Both are empty, test passes
			} else if !reflect.DeepEqual(actualErrors, tt.expectedErrors) {
				t.Errorf("Expected errors %v, got %v", tt.expectedErrors, actualErrors)
			}
		})
	}
}

// TestCompareServices tests the compareServices function
func TestCompareServices(t *testing.T) {
	tests := []struct {
		name           string
		prevProto      string
		currProto      string
		expectedErrors []string
	}{
		{
			name: "Service removal",
			prevProto: `
				syntax = "proto3";
				package test;
				message Empty {}
				service TestService {
					rpc DoSomething(Empty) returns (Empty);
				}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message Empty {}
			`,
			expectedErrors: []string{
				`Service "TestService" was removed`,
			},
		},
		{
			name: "Method removal",
			prevProto: `
				syntax = "proto3";
				package test;
				message Empty {}
				service TestService {
					rpc DoSomething(Empty) returns (Empty);
					rpc DoSomethingElse(Empty) returns (Empty);
				}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message Empty {}
				service TestService {
					rpc DoSomething(Empty) returns (Empty);
				}
			`,
			expectedErrors: []string{
				`Method "DoSomethingElse" was removed from service "TestService"`,
			},
		},
		{
			name: "Method input type change",
			prevProto: `
				syntax = "proto3";
				package test;
				message Request1 {}
				message Request2 {}
				message Response {}
				service TestService {
					rpc DoSomething(Request1) returns (Response);
				}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message Request1 {}
				message Request2 {}
				message Response {}
				service TestService {
					rpc DoSomething(Request2) returns (Response);
				}
			`,
			expectedErrors: []string{
				`Method "DoSomething" input type changed from test.Request1 to test.Request2 in service "TestService"`,
			},
		},
		{
			name: "Method output type change",
			prevProto: `
				syntax = "proto3";
				package test;
				message Request {}
				message Response1 {}
				message Response2 {}
				service TestService {
					rpc DoSomething(Request) returns (Response1);
				}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message Request {}
				message Response1 {}
				message Response2 {}
				service TestService {
					rpc DoSomething(Request) returns (Response2);
				}
			`,
			expectedErrors: []string{
				`Method "DoSomething" output type changed from test.Response1 to test.Response2 in service "TestService"`,
			},
		},
		{
			name: "Method streaming change",
			prevProto: `
				syntax = "proto3";
				package test;
				message Request {}
				message Response {}
				service TestService {
					rpc DoSomething(stream Request) returns (Response);
				}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message Request {}
				message Response {}
				service TestService {
					rpc DoSomething(Request) returns (Response);
				}
			`,
			expectedErrors: []string{
				`Method "DoSomething" client streaming changed from true to false in service "TestService"`,
			},
		},
		// Non-breaking changes
		{
			name: "Adding new method (non-breaking)",
			prevProto: `
				syntax = "proto3";
				package test;
				message Empty {}
				service TestService {
					rpc DoSomething(Empty) returns (Empty);
				}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message Empty {}
				service TestService {
					rpc DoSomething(Empty) returns (Empty);
					rpc DoSomethingElse(Empty) returns (Empty);
				}
			`,
			expectedErrors: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary proto files
			prevFile, err := createTempProtoFile(tt.prevProto)
			if err != nil {
				t.Fatalf("Failed to create previous proto file: %v", err)
			}
			defer os.Remove(prevFile)

			currFile, err := createTempProtoFile(tt.currProto)
			if err != nil {
				t.Fatalf("Failed to create current proto file: %v", err)
			}
			defer os.Remove(currFile)

			// Parse proto files directly using protoparse
			prevFileDesc, err := parseProtoFileToReflect(prevFile)
			if err != nil {
				t.Fatalf("Failed to parse previous proto file: %v", err)
			}

			currFileDesc, err := parseProtoFileToReflect(currFile)
			if err != nil {
				t.Fatalf("Failed to parse current proto file: %v", err)
			}

			// Get file descriptors
			prevFile1 := prevFileDesc
			currFile1 := currFileDesc

			// Compare services
			actualErrors := compareServices(prevFile1, currFile1)

			// Sort errors for consistent comparison
			sort.Strings(actualErrors)
			sort.Strings(tt.expectedErrors)

			// Compare results
			if len(actualErrors) == 0 && len(tt.expectedErrors) == 0 {
				// Both are empty, test passes
			} else if !reflect.DeepEqual(actualErrors, tt.expectedErrors) {
				t.Errorf("Expected errors %v, got %v", tt.expectedErrors, actualErrors)
			}
		})
	}
}

// TestCompareMessages tests the compareMessages function
func TestCompareMessages(t *testing.T) {
	tests := []struct {
		name           string
		prevProto      string
		currProto      string
		expectedErrors []string
	}{
		{
			name: "Message removal",
			prevProto: `
				syntax = "proto3";
				package test;
				message Message1 {}
				message Message2 {}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message Message1 {}
			`,
			expectedErrors: []string{
				`Message "Message2" was removed`,
			},
		},
		{
			name: "Nested message removal",
			prevProto: `
				syntax = "proto3";
				package test;
				message Outer {
					message Inner1 {}
					message Inner2 {}
				}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message Outer {
					message Inner1 {}
				}
			`,
			expectedErrors: []string{
				`Message "Outer.Inner2" was removed`,
			},
		},
		// Non-breaking changes
		{
			name: "Adding new message (non-breaking)",
			prevProto: `
				syntax = "proto3";
				package test;
				message Message1 {}
			`,
			currProto: `
				syntax = "proto3";
				package test;
				message Message1 {}
				message Message2 {}
			`,
			expectedErrors: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary proto files
			prevFile, err := createTempProtoFile(tt.prevProto)
			if err != nil {
				t.Fatalf("Failed to create previous proto file: %v", err)
			}
			defer os.Remove(prevFile)

			currFile, err := createTempProtoFile(tt.currProto)
			if err != nil {
				t.Fatalf("Failed to create current proto file: %v", err)
			}
			defer os.Remove(currFile)

			// Parse proto files directly using protoparse
			prevFileDesc, err := parseProtoFileToReflect(prevFile)
			if err != nil {
				t.Fatalf("Failed to parse previous proto file: %v", err)
			}

			currFileDesc, err := parseProtoFileToReflect(currFile)
			if err != nil {
				t.Fatalf("Failed to parse current proto file: %v", err)
			}

			// Get file descriptors
			prevFile1 := prevFileDesc
			currFile1 := currFileDesc

			// Compare messages
			actualErrors := compareMessages(prevFile1, currFile1)

			// Sort errors for consistent comparison
			sort.Strings(actualErrors)
			sort.Strings(tt.expectedErrors)

			// Compare results
			if len(actualErrors) == 0 && len(tt.expectedErrors) == 0 {
				// Both are empty, test passes
			} else if !reflect.DeepEqual(actualErrors, tt.expectedErrors) {
				t.Errorf("Expected errors %v, got %v", tt.expectedErrors, actualErrors)
			}
		})
	}
}

// Helper function to create a temporary proto file
func createTempProtoFile(content string) (string, error) {
	// Create a temporary file
	tmpFile, err := ioutil.TempFile("", "test_*.proto")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	// Write the content to the file
	content = strings.TrimSpace(content)
	if _, err := tmpFile.WriteString(content); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	// Get the absolute path
	absPath, err := filepath.Abs(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return absPath, nil
}
