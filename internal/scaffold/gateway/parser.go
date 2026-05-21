package gateway

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	packageRegex = regexp.MustCompile(`^package\s+(\w+)\s*;`)
	goPkgRegex   = regexp.MustCompile(`^option\s+go_package\s*=\s*"([^"]+)"`)
	messageOpen  = regexp.MustCompile(`^message\s+(\w+)\s*\{`)
	enumOpen     = regexp.MustCompile(`^enum\s+(\w+)\s*\{`)
	enumValue    = regexp.MustCompile(`^\s*(\w+)\s*=\s*\d+\s*;`)
	serviceOpen  = regexp.MustCompile(`^service\s+\w+\s*\{`)
	rpcLine      = regexp.MustCompile(`rpc\s+(\w+)\s*\(\s*(\w+)\s*\)\s*returns\s*\(\s*(\w+)\s*\)`)
	// optional? type name = tag [annotations]?;  — also handle repeated.
	fieldLine = regexp.MustCompile(`^\s*(repeated\s+|optional\s+)?(\w+(?:\.\w+)*)\s+(\w+)\s*=\s*\d+\s*(?:\[[^\]]*\])?\s*;`)
)

// ParseAllProtos reads every .proto file under protoRoot and returns one
// ProtoInfo per service (i.e. per subfolder). Skips proto/common since that's
// shared infra.
func ParseAllProtos(protoRoot string) ([]ProtoInfo, error) {
	entries, err := os.ReadDir(protoRoot)
	if err != nil {
		return nil, fmt.Errorf("read proto dir: %w", err)
	}

	var out []ProtoInfo
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "common" {
			continue
		}
		svc := e.Name()
		// expect proto/<svc>/<svc>.proto by tool convention
		path := filepath.Join(protoRoot, svc, svc+".proto")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		pi, err := ParseProtoFile(path)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		pi.ServiceName = svc
		out = append(out, pi)
	}
	return out, nil
}

// ParseProtoFile reads one .proto file.
func ParseProtoFile(path string) (ProtoInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return ProtoInfo{}, err
	}
	defer f.Close()

	pi := ProtoInfo{FilePath: path}
	scanner := bufio.NewScanner(f)

	var (
		curMsg    *MessageInfo
		curEnum   *EnumInfo
		inService bool
	)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Top-level
		if m := packageRegex.FindStringSubmatch(trimmed); len(m) == 2 {
			pi.PackageName = m[1]
			continue
		}
		if m := goPkgRegex.FindStringSubmatch(trimmed); len(m) == 2 {
			pi.GoPackage = m[1]
			continue
		}

		// Closing brace for the current block.
		if strings.Contains(line, "}") {
			if curMsg != nil {
				pi.Messages = append(pi.Messages, *curMsg)
				curMsg = nil
				continue
			}
			if curEnum != nil {
				pi.Enums = append(pi.Enums, *curEnum)
				curEnum = nil
				continue
			}
			if inService {
				inService = false
				continue
			}
		}

		if m := messageOpen.FindStringSubmatch(trimmed); len(m) == 2 {
			curMsg = &MessageInfo{Name: m[1]}
			continue
		}
		if m := enumOpen.FindStringSubmatch(trimmed); len(m) == 2 {
			curEnum = &EnumInfo{Name: m[1]}
			continue
		}
		if serviceOpen.MatchString(trimmed) {
			inService = true
			continue
		}

		// Inside service: capture rpcs.
		if inService {
			if m := rpcLine.FindStringSubmatch(trimmed); len(m) == 4 {
				pi.RPCs = append(pi.RPCs, RPCInfo{Name: m[1], RequestType: m[2], ResponseType: m[3]})
			}
			continue
		}

		// Inside enum: capture values.
		if curEnum != nil {
			if m := enumValue.FindStringSubmatch(line); len(m) == 2 {
				curEnum.Values = append(curEnum.Values, m[1])
			}
			continue
		}

		// Inside message: capture fields.
		if curMsg != nil {
			if m := fieldLine.FindStringSubmatch(line); len(m) == 4 {
				modifier := strings.TrimSpace(m[1])
				curMsg.Fields = append(curMsg.Fields, FieldInfo{
					Name:       m[3],
					Type:       m[2],
					IsOptional: modifier == "optional",
					IsRepeated: modifier == "repeated",
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return pi, err
	}

	// Pass 2: mark enum fields by looking up Type in declared enums of the same proto.
	enumSet := map[string][]string{}
	for _, en := range pi.Enums {
		enumSet[en.Name] = en.Values
	}
	for mi := range pi.Messages {
		for fi := range pi.Messages[mi].Fields {
			f := &pi.Messages[mi].Fields[fi]
			if vals, ok := enumSet[f.Type]; ok {
				f.IsEnum = true
				f.EnumValues = vals
			}
		}
	}
	return pi, nil
}
