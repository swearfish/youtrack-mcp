package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tailscale/hujson"

	"youtrack-mcp/internal/config"
	"youtrack-mcp/internal/envfile"
)

type RegisterOptions struct {
	Target      string
	Workspace   string
	Binary      string
	DryRun      bool
	EnvFile     string
	OverrideEnv bool
}

type Result struct {
	ConfigPath string
	Message    string
}

func Register(options RegisterOptions) (Result, error) {
	commandPath, err := resolveBinaryPath(options.Binary)
	if err != nil {
		return Result{}, err
	}

	serverConfig, err := buildServerConfig(options.Target, commandPath, options.EnvFile, options.OverrideEnv)
	if err != nil {
		return Result{}, err
	}

	configPath, key, err := resolveConfigPath(options.Target, options.Workspace)
	if err != nil {
		return Result{}, err
	}

	payload := map[string]any{
		key: map[string]any{
			config.ServerName: serverConfig,
		},
	}

	if options.DryRun {
		rendered, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return Result{}, fmt.Errorf("render dry-run payload: %w", err)
		}
		return Result{
			ConfigPath: configPath,
			Message:    fmt.Sprintf("Would write MCP registration:\n%s", string(rendered)),
		}, nil
	}

	changed, err := upsertConfig(configPath, key, config.ServerName, serverConfig)
	if err != nil {
		return Result{}, err
	}

	status := "Already up to date"
	if changed {
		status = "Updated"
	}

	return Result{
		ConfigPath: configPath,
		Message:    fmt.Sprintf("%s MCP registration for %s.", status, options.Target),
	}, nil
}

func buildServerConfig(target string, binaryPath string, envFile string, overrideEnv bool) (map[string]any, error) {
	args := []string{"mcp"}
	if envFile != "" {
		resolved, err := envfile.Resolve(envFile)
		if err != nil {
			return nil, err
		}
		args = append(args, "--env-file", resolved)
	}
	if overrideEnv {
		args = append(args, "--override-env")
	}

	switch target {
	case "vscode":
		return map[string]any{
			"type":    "stdio",
			"command": binaryPath,
			"args":    args,
		}, nil
	case "copilot":
		return map[string]any{
			"type":    "local",
			"command": binaryPath,
			"args":    args,
			"tools":   []string{"*"},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported install target %q", target)
	}
}

func resolveConfigPath(target string, workspace string) (string, string, error) {
	if workspace != "" {
		absWorkspace, err := filepath.Abs(workspace)
		if err != nil {
			return "", "", fmt.Errorf("resolve workspace path: %w", err)
		}
		info, err := os.Stat(absWorkspace)
		if err != nil {
			return "", "", fmt.Errorf("stat workspace path: %w", err)
		}
		if !info.IsDir() {
			return "", "", fmt.Errorf("workspace path is not a directory: %s", absWorkspace)
		}

		switch target {
		case "vscode":
			return filepath.Join(absWorkspace, ".vscode", "mcp.json"), "servers", nil
		case "copilot":
			return filepath.Join(absWorkspace, ".copilot", "mcp.json"), "mcpServers", nil
		default:
			return "", "", fmt.Errorf("unsupported install target %q", target)
		}
	}

	switch target {
	case "vscode":
		return defaultVSCodeConfigPath(), "servers", nil
	case "copilot":
		return defaultCopilotConfigPath(), "mcpServers", nil
	default:
		return "", "", fmt.Errorf("unsupported install target %q", target)
	}
}

func defaultVSCodeConfigPath() string {
	if override := os.Getenv(config.EnvVSCodeMCPConfigPath); override != "" {
		return override
	}

	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Code", "User", "mcp.json")
	case "darwin":
		return filepath.Join(userHomeDir(), "Library", "Application Support", "Code", "User", "mcp.json")
	default:
		return filepath.Join(userHomeDir(), ".config", "Code", "User", "mcp.json")
	}
}

func defaultCopilotConfigPath() string {
	if override := os.Getenv(config.EnvCopilotMCPConfigPath); override != "" {
		return override
	}
	return filepath.Join(userHomeDir(), ".copilot", "mcp-config.json")
}

func userHomeDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return os.Getenv("HOME")
}

func resolveBinaryPath(override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}

	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve current executable: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(executable)
	if err == nil {
		executable = resolved
	}

	if strings.Contains(executable, "go-build") {
		return "", fmt.Errorf("install was run from a temporary go run binary; build the CLI first or pass --binary")
	}

	return executable, nil
}

func upsertConfig(configPath string, serversKey string, serverName string, serverConfig map[string]any) (bool, error) {
	current, rawContent, hasServersKey, err := loadConfig(configPath, serversKey)
	if err != nil {
		return false, err
	}

	servers, _ := current[serversKey].(map[string]any)
	existing, _ := servers[serverName]
	changed := !deepEqual(existing, serverConfig)
	if !changed {
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return false, fmt.Errorf("create config directory: %w", err)
	}

	var data []byte
	if len(rawContent) == 0 {
		servers[serverName] = serverConfig
		current[serversKey] = servers

		var err error
		data, err = json.MarshalIndent(current, "", "  ")
		if err != nil {
			return false, fmt.Errorf("marshal config: %w", err)
		}
		data = append(data, '\n')
	} else {
		data, err = patchConfig(rawContent, serversKey, serverName, serverConfig, hasServersKey, existing != nil)
		if err != nil {
			return false, err
		}
	}

	if err := writeFileAtomic(configPath, data, 0o644); err != nil {
		return false, fmt.Errorf("write config: %w", err)
	}

	return changed, nil
}

func loadConfig(configPath string, serversKey string) (map[string]any, []byte, bool, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{serversKey: map[string]any{}}, nil, false, nil
		}
		return nil, nil, false, fmt.Errorf("read config: %w", err)
	}
	if len(content) == 0 {
		return map[string]any{serversKey: map[string]any{}}, nil, false, nil
	}

	standardized, err := hujson.Standardize(append([]byte(nil), content...))
	if err != nil {
		return nil, nil, false, fmt.Errorf("parse config %s: %w", configPath, err)
	}

	var data map[string]any
	if err := json.Unmarshal(standardized, &data); err != nil {
		return nil, nil, false, fmt.Errorf("parse config %s: %w", configPath, err)
	}
	if data == nil {
		data = map[string]any{}
	}

	value, ok := data[serversKey]
	if !ok {
		data[serversKey] = map[string]any{}
		return data, content, false, nil
	}

	if _, ok := value.(map[string]any); !ok {
		return nil, nil, false, fmt.Errorf("invalid config %s: %q must be an object", configPath, serversKey)
	}

	return data, content, true, nil
}

func patchConfig(content []byte, serversKey string, serverName string, serverConfig map[string]any, hasServersKey bool, replace bool) ([]byte, error) {
	root, err := hujson.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("parse hujson config: %w", err)
	}

	rootObject, ok := root.Value.(*hujson.Object)
	if !ok {
		return nil, fmt.Errorf("invalid config: root value must be an object")
	}

	if !hasServersKey {
		memberIndent, indentUnit := detectObjectIndentation(rootObject)
		serverBlock, err := marshalIndentedValue(map[string]any{serverName: serverConfig}, memberIndent, indentUnit)
		if err != nil {
			return nil, err
		}
		appendObjectMember(rootObject, serversKey, serverBlock)
		root.UpdateOffsets()
		return root.Pack(), nil
	}

	serversIndex := findObjectMemberIndex(rootObject, serversKey)
	if serversIndex < 0 {
		return nil, fmt.Errorf("invalid config: missing %q object", serversKey)
	}
	serversObject, ok := rootObject.Members[serversIndex].Value.Value.(*hujson.Object)
	if !ok {
		return nil, fmt.Errorf("invalid config: %q must be an object", serversKey)
	}

	memberIndent, indentUnit := detectObjectIndentation(serversObject)
	serverValue, err := marshalIndentedValue(serverConfig, memberIndent, indentUnit)
	if err != nil {
		return nil, err
	}

	serverIndex := findObjectMemberIndex(serversObject, serverName)
	if replace {
		if serverIndex < 0 {
			return nil, fmt.Errorf("invalid config: missing %q entry under %q", serverName, serversKey)
		}
		serversObject.Members[serverIndex].Value.Value = serverValue.Value
	} else {
		appendObjectMember(serversObject, serverName, serverValue)
	}

	root.UpdateOffsets()
	return root.Pack(), nil
}

func marshalIndentedValue(value any, prefix string, indent string) (hujson.Value, error) {
	if indent == "" {
		indent = "  "
	}

	data, err := json.MarshalIndent(value, prefix, indent)
	if err != nil {
		return hujson.Value{}, fmt.Errorf("marshal config value: %w", err)
	}

	parsed, err := hujson.Parse(data)
	if err != nil {
		return hujson.Value{}, fmt.Errorf("parse config value: %w", err)
	}

	return parsed, nil
}

func appendObjectMember(object *hujson.Object, name string, value hujson.Value) {
	before := hujson.Extra(" ")
	if isMultilineObject(object) {
		memberIndent, _ := detectObjectIndentation(object)
		before = hujson.Extra("\n" + memberIndent)
	}

	value.BeforeExtra = hujson.Extra(" ")
	value.AfterExtra = nil

	object.Members = append(object.Members, hujson.ObjectMember{
		Name: hujson.Value{
			BeforeExtra: before,
			Value:       hujson.String(name),
		},
		Value: value,
	})
}

func detectObjectIndentation(object *hujson.Object) (string, string) {
	closingIndent := trailingIndent(string(object.AfterExtra))
	memberIndent := ""
	if len(object.Members) > 0 {
		memberIndent = trailingIndent(string(object.Members[len(object.Members)-1].Name.BeforeExtra))
	}

	indentUnit := "  "
	if strings.HasPrefix(memberIndent, closingIndent) && len(memberIndent) > len(closingIndent) {
		indentUnit = memberIndent[len(closingIndent):]
	}

	if memberIndent == "" && isMultilineObject(object) {
		memberIndent = closingIndent + indentUnit
	}

	return memberIndent, indentUnit
}

func isMultilineObject(object *hujson.Object) bool {
	if strings.Contains(string(object.AfterExtra), "\n") {
		return true
	}
	for _, member := range object.Members {
		if strings.Contains(string(member.Name.BeforeExtra), "\n") || strings.Contains(string(member.Value.AfterExtra), "\n") {
			return true
		}
	}
	return false
}

func trailingIndent(text string) string {
	index := strings.LastIndex(text, "\n")
	if index < 0 {
		return ""
	}
	return text[index+1:]
}

func findObjectMemberIndex(object *hujson.Object, name string) int {
	for index, member := range object.Members {
		if literal, ok := member.Name.Value.(hujson.Literal); ok && literal.String() == name {
			return index
		}
	}
	return -1
}

func writeFileAtomic(path string, data []byte, defaultMode os.FileMode) (err error) {
	mode := defaultMode
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("stat config: %w", statErr)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		if tempFile != nil {
			closeErr := tempFile.Close()
			if err == nil && closeErr != nil {
				err = fmt.Errorf("close temp file: %w", closeErr)
			}
		}
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()

	if err := tempFile.Chmod(mode); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tempFile.Write(data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		tempFile = nil
		return fmt.Errorf("close temp file: %w", err)
	}
	tempFile = nil

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

func deepEqual(left any, right any) bool {
	leftJSON, err := json.Marshal(left)
	if err != nil {
		return false
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		return false
	}
	return string(leftJSON) == string(rightJSON)
}
