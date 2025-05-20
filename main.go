package main

import (
	"fmt"
	"os"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

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

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: go run main.go <prev_fds.pb> <curr_fds.pb>")
		fmt.Println("")
		fmt.Println("To generate descriptor files, use protoc:")
		fmt.Println("  protoc --include_imports --descriptor_set_out=prev_fds.pb your_proto_file.proto")
		fmt.Println("  protoc --include_imports --descriptor_set_out=curr_fds.pb your_updated_proto_file.proto")
		os.Exit(1)
	}

	prevPath := os.Args[1]
	currPath := os.Args[2]

	// Load file descriptor sets
	prevSet, err := loadFileDescriptorSet(prevPath)
	if err != nil {
		fmt.Printf("Error loading previous descriptor set: %v\n", err)
		os.Exit(1)
	}

	currSet, err := loadFileDescriptorSet(currPath)
	if err != nil {
		fmt.Printf("Error loading current descriptor set: %v\n", err)
		os.Exit(1)
	}

	// Create file descriptor registries
	prevFiles, err := protodesc.NewFiles(prevSet)
	if err != nil {
		fmt.Printf("Error creating previous file registry: %v\n", err)
		os.Exit(1)
	}

	currFiles, err := protodesc.NewFiles(currSet)
	if err != nil {
		fmt.Printf("Error creating current file registry: %v\n", err)
		os.Exit(1)
	}

	// Collect all breaking changes
	var allBreakingChanges []string

	// Print information about the files
	fmt.Println("Previous files:")
	prevFiles.RangeFiles(func(file protoreflect.FileDescriptor) bool {
		fmt.Printf("  - %s (package: %s)\n", file.Path(), file.Package())
		return true
	})

	fmt.Println("Current files:")
	currFiles.RangeFiles(func(file protoreflect.FileDescriptor) bool {
		fmt.Printf("  - %s (package: %s)\n", file.Path(), file.Package())
		return true
	})

	// Create maps of files by package
	prevFilesByPackage := make(map[string][]protoreflect.FileDescriptor)
	currFilesByPackage := make(map[string][]protoreflect.FileDescriptor)

	prevFiles.RangeFiles(func(file protoreflect.FileDescriptor) bool {
		pkg := string(file.Package())
		prevFilesByPackage[pkg] = append(prevFilesByPackage[pkg], file)
		return true
	})

	currFiles.RangeFiles(func(file protoreflect.FileDescriptor) bool {
		pkg := string(file.Package())
		currFilesByPackage[pkg] = append(currFilesByPackage[pkg], file)
		return true
	})

	// Compare files by package
	for pkg, prevFileList := range prevFilesByPackage {
		currFileList, ok := currFilesByPackage[pkg]
		if !ok {
			// Package was removed
			for _, prevFile := range prevFileList {
				allBreakingChanges = append(allBreakingChanges,
					fmt.Sprintf("Package %q was removed (file: %s)", pkg, prevFile.Path()))
			}
			continue
		}

		// For simplicity, just compare the first file in each package
		// In a real implementation, you'd need to handle multiple files per package
		if len(prevFileList) > 0 && len(currFileList) > 0 {
			prevFile := prevFileList[0]
			currFile := currFileList[0]

			fmt.Printf("Comparing files: %s and %s (package: %s)\n",
				prevFile.Path(), currFile.Path(), pkg)

			// Compare messages, enums, and services
			msgChanges := compareMessages(prevFile, currFile)
			enumChanges := compareEnums(prevFile, currFile)
			serviceChanges := compareServices(prevFile, currFile)
			allBreakingChanges = append(allBreakingChanges, msgChanges...)
			allBreakingChanges = append(allBreakingChanges, enumChanges...)
			allBreakingChanges = append(allBreakingChanges, serviceChanges...)
		}
	}

	// Print results
	if len(allBreakingChanges) == 0 {
		fmt.Println("✅ No breaking changes detected")
	} else {
		fmt.Printf("🔴 Detected %d breaking changes:\n", len(allBreakingChanges))
		for _, change := range allBreakingChanges {
			fmt.Printf("  - %s\n", change)
		}
	}
}
