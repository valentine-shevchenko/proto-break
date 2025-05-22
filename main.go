package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// parseProtoFileToReflect parses a proto file and returns a protoreflect.FileDescriptor
func parseProtoFileToReflect(filePath string) (protoreflect.FileDescriptor, error) {
	// Use the ParseProtoFile function from parser.go
	fileDesc, err := ParseProtoFile(filePath)
	if err != nil {
		return nil, err
	}

	// Convert to protoreflect.FileDescriptor
	return fileDesc.UnwrapFile(), nil
}

// loadFileDescriptorSet loads a FileDescriptorSet from a file
func loadFileDescriptorSet(path string) (*descriptorpb.FileDescriptorSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(data, &fds); err != nil {
		return nil, err
	}
	return &fds, nil
}

// compareFields compares fields between previous and current messages
func compareFields(prevMsg, currMsg protoreflect.MessageDescriptor) []string {
	msgName := string(prevMsg.Name())
	var breakingChanges []string
	prevFields := prevMsg.Fields()
	currFields := currMsg.Fields()

	// Check field map for quick lookup by number
	currFieldsByNumber := make(map[protoreflect.FieldNumber]protoreflect.FieldDescriptor)
	for i := 0; i < currFields.Len(); i++ {
		field := currFields.Get(i)
		currFieldsByNumber[field.Number()] = field
	}

	// Check each previous field
	for i := 0; i < prevFields.Len(); i++ {
		prevField := prevFields.Get(i)
		fieldName := string(prevField.Name())
		fieldNumber := prevField.Number()

		// Check if field was removed by number
		currField, ok := currFieldsByNumber[fieldNumber]
		if !ok {
			breakingChanges = append(breakingChanges,
				fmt.Sprintf("Field %q (number %d) was removed from message %q", fieldName, fieldNumber, msgName))
			continue
		}

		// Check if field was renamed
		if prevField.Name() != currField.Name() {
			breakingChanges = append(breakingChanges,
				fmt.Sprintf("Field renamed from %q to %q in message %q", prevField.Name(), currField.Name(), msgName))
		}

		// Check field type changes
		prevKind := prevField.Kind()
		currKind := currField.Kind()
		if prevKind != currKind {
			breakingChanges = append(breakingChanges,
				fmt.Sprintf("Field %q type changed from %s to %s in message %q", fieldName, prevKind, currKind, msgName))
		}

		// Check cardinality changes
		prevCardinality := prevField.Cardinality()
		currCardinality := currField.Cardinality()
		if prevCardinality != currCardinality {
			// Changing from repeated to singular is breaking
			if prevCardinality == protoreflect.Repeated && currCardinality != protoreflect.Repeated {
				breakingChanges = append(breakingChanges,
					fmt.Sprintf("Field %q cardinality changed from repeated to singular in message %q", fieldName, msgName))
			}
		}
	}

	return breakingChanges
}

// collectNestedEnums collects all nested enums from message descriptors
func collectNestedEnums(msgs protoreflect.MessageDescriptors, prefix string, output map[string]protoreflect.EnumDescriptor) {
	for i := 0; i < msgs.Len(); i++ {
		msg := msgs.Get(i)
		msgPrefix := prefix + string(msg.Name()) + "."

		// Collect enums in this message
		enums := msg.Enums()
		for j := 0; j < enums.Len(); j++ {
			enum := enums.Get(j)
			name := msgPrefix + string(enum.Name())
			output[name] = enum
		}

		// Recursively collect enums in nested messages
		collectNestedEnums(msg.Messages(), msgPrefix, output)
	}
}

// collectNestedMessages collects all nested messages from message descriptors
func collectNestedMessages(msgs protoreflect.MessageDescriptors, prefix string, output map[string]protoreflect.MessageDescriptor) {
	for i := 0; i < msgs.Len(); i++ {
		msg := msgs.Get(i)
		name := prefix + string(msg.Name())
		output[name] = msg

		// Recursively collect nested messages
		collectNestedMessages(msg.Messages(), name+".", output)
	}
}

// compareEnums compares enums between previous and current files
func compareEnums(prevFile, currFile protoreflect.FileDescriptor) []string {
	var breakingChanges []string

	// Collect all enums (including nested ones)
	prevEnumsByName := make(map[string]protoreflect.EnumDescriptor)
	currEnumsByName := make(map[string]protoreflect.EnumDescriptor)

	// Collect top-level enums
	prevEnums := prevFile.Enums()
	currEnums := currFile.Enums()

	for i := 0; i < prevEnums.Len(); i++ {
		enum := prevEnums.Get(i)
		prevEnumsByName[string(enum.Name())] = enum
	}

	for i := 0; i < currEnums.Len(); i++ {
		enum := currEnums.Get(i)
		currEnumsByName[string(enum.Name())] = enum
	}

	// Collect nested enums
	collectNestedEnums(prevFile.Messages(), "", prevEnumsByName)
	collectNestedEnums(currFile.Messages(), "", currEnumsByName)

	// Check each previous enum
	for enumName, prevEnum := range prevEnumsByName {
		// Check if enum was removed
		currEnum, ok := currEnumsByName[enumName]
		if !ok {
			breakingChanges = append(breakingChanges,
				fmt.Sprintf("Enum %q was removed", enumName))
			continue
		}

		// Compare enum values
		prevValues := prevEnum.Values()
		currValuesByNumber := make(map[protoreflect.EnumNumber]protoreflect.EnumValueDescriptor)

		// Build map of current enum values by number
		currValues := currEnum.Values()
		for j := 0; j < currValues.Len(); j++ {
			value := currValues.Get(j)
			currValuesByNumber[value.Number()] = value
		}

		// Check each previous enum value
		for j := 0; j < prevValues.Len(); j++ {
			prevValue := prevValues.Get(j)
			valueName := string(prevValue.Name())
			valueNumber := prevValue.Number()

			// Check if enum value was removed
			currValue, ok := currValuesByNumber[valueNumber]
			if !ok {
				breakingChanges = append(breakingChanges,
					fmt.Sprintf("Enum value %q (number %d) was removed from enum %q",
						valueName, valueNumber, enumName))
				continue
			}

			// Check if enum value was renamed
			if prevValue.Name() != currValue.Name() {
				breakingChanges = append(breakingChanges,
					fmt.Sprintf("Enum value renamed from %q to %q in enum %q",
						prevValue.Name(), currValue.Name(), enumName))
			}
		}
	}

	return breakingChanges
}

// compareServices compares services between previous and current files
func compareServices(prevFile, currFile protoreflect.FileDescriptor) []string {
	var breakingChanges []string

	// Get services from both files
	prevServices := prevFile.Services()
	currServices := currFile.Services()

	// Create maps for quick lookup
	currServicesByName := make(map[string]protoreflect.ServiceDescriptor)
	for i := 0; i < currServices.Len(); i++ {
		service := currServices.Get(i)
		currServicesByName[string(service.Name())] = service
	}

	// Check each previous service
	for i := 0; i < prevServices.Len(); i++ {
		prevService := prevServices.Get(i)
		serviceName := string(prevService.Name())

		// Check if service was removed
		currService, ok := currServicesByName[serviceName]
		if !ok {
			breakingChanges = append(breakingChanges,
				fmt.Sprintf("Service %q was removed", serviceName))
			continue
		}

		// Compare methods
		prevMethods := prevService.Methods()
		currMethodsByName := make(map[string]protoreflect.MethodDescriptor)

		// Build map of current methods by name
		currMethods := currService.Methods()
		for j := 0; j < currMethods.Len(); j++ {
			method := currMethods.Get(j)
			currMethodsByName[string(method.Name())] = method
		}

		// Check each previous method
		for j := 0; j < prevMethods.Len(); j++ {
			prevMethod := prevMethods.Get(j)
			methodName := string(prevMethod.Name())

			// Check if method was removed
			currMethod, ok := currMethodsByName[methodName]
			if !ok {
				breakingChanges = append(breakingChanges,
					fmt.Sprintf("Method %q was removed from service %q", methodName, serviceName))
				continue
			}

			// Check input type changes
			prevInput := prevMethod.Input().FullName()
			currInput := currMethod.Input().FullName()
			if prevInput != currInput {
				breakingChanges = append(breakingChanges,
					fmt.Sprintf("Method %q input type changed from %s to %s in service %q",
						methodName, prevInput, currInput, serviceName))
			}

			// Check output type changes
			prevOutput := prevMethod.Output().FullName()
			currOutput := currMethod.Output().FullName()
			if prevOutput != currOutput {
				breakingChanges = append(breakingChanges,
					fmt.Sprintf("Method %q output type changed from %s to %s in service %q",
						methodName, prevOutput, currOutput, serviceName))
			}

			// Check streaming changes
			if prevMethod.IsStreamingClient() != currMethod.IsStreamingClient() {
				breakingChanges = append(breakingChanges,
					fmt.Sprintf("Method %q client streaming changed from %v to %v in service %q",
						methodName, prevMethod.IsStreamingClient(), currMethod.IsStreamingClient(), serviceName))
			}

			if prevMethod.IsStreamingServer() != currMethod.IsStreamingServer() {
				breakingChanges = append(breakingChanges,
					fmt.Sprintf("Method %q server streaming changed from %v to %v in service %q",
						methodName, prevMethod.IsStreamingServer(), currMethod.IsStreamingServer(), serviceName))
			}
		}
	}

	return breakingChanges
}

// compareMessages compares messages between previous and current files
func compareMessages(prevFile, currFile protoreflect.FileDescriptor) []string {
	var breakingChanges []string

	// Collect all messages (including nested ones)
	prevMsgsByName := make(map[string]protoreflect.MessageDescriptor)
	currMsgsByName := make(map[string]protoreflect.MessageDescriptor)

	// Collect top-level messages
	prevMsgs := prevFile.Messages()
	currMsgs := currFile.Messages()

	for i := 0; i < prevMsgs.Len(); i++ {
		msg := prevMsgs.Get(i)
		prevMsgsByName[string(msg.Name())] = msg
	}

	for i := 0; i < currMsgs.Len(); i++ {
		msg := currMsgs.Get(i)
		currMsgsByName[string(msg.Name())] = msg
	}

	// Collect nested messages
	collectNestedMessages(prevFile.Messages(), "", prevMsgsByName)
	collectNestedMessages(currFile.Messages(), "", currMsgsByName)

	// Check each previous message
	for msgName, prevMsg := range prevMsgsByName {
		// Check if message was removed
		currMsg, ok := currMsgsByName[msgName]
		if !ok {
			breakingChanges = append(breakingChanges,
				fmt.Sprintf("Message %q was removed", msgName))
			continue
		}

		// Compare fields
		fieldChanges := compareFields(prevMsg, currMsg)
		breakingChanges = append(breakingChanges, fieldChanges...)
	}

	return breakingChanges
}

// getModifiedProtoFiles returns a list of proto files with changes compared to the specified commit
func getModifiedProtoFiles(compareCommit string) ([]string, error) {
	// First check if the commit exists
	checkCmd := exec.Command("git", "rev-parse", "--verify", compareCommit)
	if err := checkCmd.Run(); err != nil {
		return nil, fmt.Errorf("error: commit '%s' does not exist or is invalid", compareCommit)
	}

	// Get changes compared to the specified commit
	cmd := exec.Command("git", "diff", "--name-only", compareCommit)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error running git diff: %v", err)
	}

	// Filter for .proto files
	var protoFiles []string
	files := strings.Split(string(output), "\n")
	for _, file := range files {
		if strings.TrimSpace(file) == "" {
			continue
		}
		if filepath.Ext(file) == ".proto" {
			// Check if the file exists (it might have been deleted)
			if _, err := os.Stat(file); err == nil {
				protoFiles = append(protoFiles, file)
			}
		}
	}

	return protoFiles, nil
}

// getPreviousVersionOfFile gets the previous version of a file from git
func getPreviousVersionOfFile(file, compareCommit string) (string, error) {
	// Create a temporary file to store the previous version
	tmpFile, err := ioutil.TempFile("", "prev_*.proto")
	if err != nil {
		return "", fmt.Errorf("error creating temporary file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Get the previous version from git
	cmd := exec.Command("git", "show", compareCommit+":"+file)
	output, err := cmd.Output()
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("error getting previous version from git: %v", err)
	}

	// Write the previous version to the temporary file
	if err := ioutil.WriteFile(tmpPath, output, 0644); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("error writing to temporary file: %v", err)
	}

	return tmpPath, nil
}

// compareProtoFile compares the current and previous versions of a proto file
func compareProtoFile(protoFile, compareCommit string) ([]string, error) {
	fmt.Printf("Analyzing changes in %s...\n", protoFile)

	// Get the previous version of the file
	prevProtoPath, err := getPreviousVersionOfFile(protoFile, compareCommit)
	if err != nil {
		return nil, fmt.Errorf("error getting previous version: %v", err)
	}
	defer os.Remove(prevProtoPath)

	// Parse proto files directly using protoparse
	prevFileDesc, err := parseProtoFileToReflect(prevProtoPath)
	if err != nil {
		return nil, fmt.Errorf("error parsing previous proto file: %v", err)
	}

	currFileDesc, err := parseProtoFileToReflect(protoFile)
	if err != nil {
		return nil, fmt.Errorf("error parsing current proto file: %v", err)
	}

	// Compare the files directly
	var allBreakingChanges []string

	// Compare messages
	msgChanges := compareMessages(prevFileDesc, currFileDesc)
	allBreakingChanges = append(allBreakingChanges, msgChanges...)

	// Compare enums
	enumChanges := compareEnums(prevFileDesc, currFileDesc)
	allBreakingChanges = append(allBreakingChanges, enumChanges...)

	// Compare services
	serviceChanges := compareServices(prevFileDesc, currFileDesc)
	allBreakingChanges = append(allBreakingChanges, serviceChanges...)

	return allBreakingChanges, nil
}

func main() {
	// Define command-line flags
	compareCommitFlag := flag.String("commit", "HEAD", "Git commit to compare against (default: HEAD)")
	helpFlag := flag.Bool("help", false, "Show help message")
	flag.Parse()

	// Show help message if requested
	if *helpFlag {
		fmt.Println("Proto Breaking Change Detector")
		fmt.Println("Automatically detects breaking changes in Protocol Buffer files")
		fmt.Println("")
		fmt.Println("Usage:")
		fmt.Println("  go run main.go [options]")
		fmt.Println("")
		fmt.Println("Options:")
		flag.PrintDefaults()
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  go run main.go                   # Compare with HEAD (current state vs. last commit)")
		fmt.Println("  go run main.go --commit HEAD~1   # Compare with the commit before the last one")
		fmt.Println("  go run main.go --commit abc123   # Compare with a specific commit hash")
		os.Exit(0)
	}

	// No need to check for protoc installation since we're using protoparse directly

	// Get modified proto files
	modifiedProtoFiles, err := getModifiedProtoFiles(*compareCommitFlag)
	if err != nil {
		fmt.Printf("Error getting modified proto files: %v\n", err)
		os.Exit(1)
	}

	if len(modifiedProtoFiles) == 0 {
		fmt.Println("No modified proto files found")
		os.Exit(0)
	}

	fmt.Printf("Found %d modified proto files compared to %s\n", len(modifiedProtoFiles), *compareCommitFlag)

	// Process each modified proto file
	hasBreakingChanges := false
	for _, protoFile := range modifiedProtoFiles {
		breakingChanges, err := compareProtoFile(protoFile, *compareCommitFlag)
		if err != nil {
			fmt.Printf("Error processing %s: %v\n", protoFile, err)
			continue
		}

		// Print results for this file
		if len(breakingChanges) == 0 {
			fmt.Printf("✅ No breaking changes detected in %s\n", protoFile)
		} else {
			hasBreakingChanges = true
			fmt.Printf("🔴 Detected %d breaking changes in %s:\n", len(breakingChanges), protoFile)
			for _, change := range breakingChanges {
				fmt.Printf("  - %s\n", change)
			}
		}
	}

	// Exit with error code if breaking changes were found
	if hasBreakingChanges {
		os.Exit(1)
	}
}
