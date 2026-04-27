package youtrack

import (
	"encoding/json"
	"fmt"
	"strings"
)

func normalizeCustomFields(raw any) ([]map[string]any, error) {
	if raw == nil {
		return nil, nil
	}

	switch value := raw.(type) {
	case CustomFieldsInput:
		if len(value) == 0 {
			return nil, nil
		}
		return []map[string]any(value), nil
	case string:
		if strings.TrimSpace(value) == "" {
			return nil, nil
		}
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			return nil, fmt.Errorf("custom_fields is not valid JSON: %w", err)
		}
		return normalizeCustomFields(parsed)
	case map[string]any:
		return []map[string]any{value}, nil
	case []map[string]any:
		if len(value) == 0 {
			return nil, nil
		}
		return value, nil
	case []any:
		result := make([]map[string]any, 0, len(value))
		for _, item := range value {
			obj, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("custom_fields must be a JSON object or array of objects")
			}
			result = append(result, obj)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("custom_fields must be a JSON object or array of objects")
	}
}

func serializeIssue(issue map[string]any) Issue {
	customFields := asMapSlice(issue["customFields"])
	return Issue{
		ID:                 textValue(issue["id"]),
		Ticket:             firstNonEmpty(textValue(issue["idReadable"]), textValue(issue["id"])),
		Summary:            textValue(issue["summary"]),
		Description:        textValue(issue["description"]),
		Project:            projectFromMap(asMap(issue["project"])),
		Status:             extractStatus(customFields),
		AcceptanceCriteria: extractAcceptanceCriteria(customFields),
		LinkedTickets:      extractLinkedTickets(asMapSlice(issue["links"])),
	}
}

func projectFromMap(data map[string]any) Project {
	return Project{
		ID:        textValue(data["id"]),
		Name:      textValue(data["name"]),
		ShortName: textValue(data["shortName"]),
	}
}

func findStatusField(customFields []map[string]any) map[string]any {
	for _, field := range customFields {
		fieldType := strings.ToLower(strings.TrimSpace(textValue(field["$type"])))
		if fieldType == "stateissuecustomfield" {
			return field
		}
	}

	for _, field := range customFields {
		fieldName := strings.ToLower(strings.TrimSpace(textValue(field["name"])))
		if fieldName == "state" || fieldName == "status" {
			return field
		}
	}

	return nil
}

func extractStatus(customFields []map[string]any) string {
	field := findStatusField(customFields)
	if field == nil {
		return ""
	}
	return fieldValueToText(field["value"])
}

func extractAcceptanceCriteria(customFields []map[string]any) string {
	for _, field := range customFields {
		name := strings.ToLower(strings.ReplaceAll(textValue(field["name"]), " ", ""))
		if name != "acceptancecriteria" {
			continue
		}
		return fieldValueToText(field["value"])
	}
	return ""
}

func extractLinkedTickets(links []map[string]any) []string {
	result := []string{}
	for _, link := range links {
		relation := displayRelationName(link)
		for _, issue := range asMapSlice(link["issues"]) {
			issueID := firstNonEmpty(textValue(issue["idReadable"]), textValue(issue["id"]), "unknown")
			line := "- " + relation + ": " + issueID
			if summary := textValue(issue["summary"]); summary != "" {
				line += " - " + summary
			}
			result = append(result, line)
		}
	}
	return result
}

func displayRelationName(link map[string]any) string {
	linkType := asMap(link["linkType"])
	direction := textValue(link["direction"])
	switch direction {
	case "OUTWARD":
		return firstNonEmpty(textValue(linkType["sourceToTarget"]), textValue(linkType["name"]), "linked to")
	case "INWARD":
		return firstNonEmpty(textValue(linkType["targetToSource"]), textValue(linkType["name"]), "linked from")
	default:
		return firstNonEmpty(textValue(linkType["name"]), textValue(linkType["sourceToTarget"]), "linked to")
	}
}

func fieldValueToText(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		// YouTrack commonly exposes a user-facing "presentation" string before the raw field value.
		for _, key := range []string{"presentation", "name", "text", "fullName", "login"} {
			if candidate := textValue(typed[key]); candidate != "" {
				return candidate
			}
		}
	case string:
		return strings.TrimSpace(typed)
	}
	return ""
}

func asMap(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func asMapSlice(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]map[string]any); ok {
			return typed
		}
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if obj, ok := item.(map[string]any); ok {
			result = append(result, obj)
		}
	}
	return result
}

func textValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int8:
		return int64(typed)
	case int16:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		return int64(typed)
	case uint8:
		return int64(typed)
	case uint16:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint64:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed
		}
	}
	return 0
}
