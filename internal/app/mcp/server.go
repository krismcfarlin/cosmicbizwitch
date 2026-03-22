package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"cosmicbizwitch/internal/app/clickfunnels"
	"cosmicbizwitch/internal/app/storage"
	"github.com/pocketbase/pocketbase/core"
)

// resolveUserCollection looks up a collection by name and rejects system collections.
func (s *MCPServer) resolveUserCollection(name string) (*core.Collection, error) {
	if name == "" {
		return nil, fmt.Errorf("collection name is required")
	}
	col, err := s.store.App().FindCollectionByNameOrId(name)
	if err != nil {
		return nil, fmt.Errorf("collection not found: %s", name)
	}
	if col.System {
		return nil, fmt.Errorf("collection %q is a system collection", name)
	}
	return col, nil
}

// MCPServer implements the Model Context Protocol for LLM agents
type MCPServer struct {
	store    *storage.Store
	logger   *log.Logger
	cfClient *clickfunnels.Client
}

// NewMCPServer creates a new MCP server. cfClient may be nil when ClickFunnels
// is not configured; all CF tool calls will return an error in that case.
func NewMCPServer(store *storage.Store, logger *log.Logger, cfClient *clickfunnels.Client) *MCPServer {
	if logger == nil {
		logger = log.Default()
	}
	return &MCPServer{
		store:    store,
		logger:   logger,
		cfClient: cfClient,
	}
}

// Tool represents an MCP tool definition
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ToolCallRequest represents an MCP tool call request
type ToolCallRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolCallResponse represents an MCP tool call response
type ToolCallResponse struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock represents a piece of content in the response
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ListTools returns all available MCP tools
func (s *MCPServer) ListTools() []Tool {
	return []Tool{
		{
			Name:        "pb_schema",
			Description: "List all non-system PocketBase collections and their field schemas",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "pb_list",
			Description: "List records from a PocketBase collection with optional filtering and sorting",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{
						"type":        "string",
						"description": "Any non-system PocketBase collection name",
					},
					"filter": map[string]interface{}{
						"type":        "string",
						"description": "PocketBase filter expression, e.g. \"status = 'pending'\" or \"client_id = 'abc'\"",
					},
					"sort": map[string]interface{}{
						"type":        "string",
						"description": "Sort field with optional direction prefix, e.g. \"-created\" or \"+name\" (default: -created)",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Max records to return (default: 50, max: 500)",
						"default":     50,
					},
					"offset": map[string]interface{}{
						"type":        "integer",
						"description": "Number of records to skip for pagination (default: 0)",
						"default":     0,
					},
				},
				"required": []string{"collection"},
			},
		},
		{
			Name:        "pb_get",
			Description: "Get a single record by ID from a PocketBase collection",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{
						"type":        "string",
						"description": "Collection name",
					},
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Record ID",
					},
				},
				"required": []string{"collection", "id"},
			},
		},
		{
			Name:        "pb_create",
			Description: "Create a new record in a PocketBase collection",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{
						"type":        "string",
						"description": "Collection name",
					},
					"fields": map[string]interface{}{
						"type":        "object",
						"description": "Field values as a JSON object, e.g. {\"name\": \"Alice\", \"status\": \"pending\"}",
					},
				},
				"required": []string{"collection", "fields"},
			},
		},
		{
			Name:        "pb_update",
			Description: "Update fields on an existing PocketBase record",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{
						"type":        "string",
						"description": "Collection name",
					},
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Record ID to update",
					},
					"fields": map[string]interface{}{
						"type":        "object",
						"description": "Fields to update as a JSON object (only specified fields are changed)",
					},
				},
				"required": []string{"collection", "id", "fields"},
			},
		},
		{
			Name:        "pb_delete",
			Description: "Delete a record from a PocketBase collection",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{
						"type":        "string",
						"description": "Collection name",
					},
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Record ID to delete",
					},
				},
				"required": []string{"collection", "id"},
			},
		},
		{
			Name:        "cf_list_contacts",
			Description: "List contacts from ClickFunnels. Supports cursor pagination and tag filtering.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"after_id": map[string]interface{}{
						"type":        "integer",
						"description": "Cursor ID for pagination — returns contacts after this contact ID (20 per page)",
					},
					"tag_ids": map[string]interface{}{
						"type":        "string",
						"description": "Comma-separated list of tag IDs to filter by, e.g. \"1,2,3\"",
					},
				},
			},
		},
		{
			Name:        "cf_get_contact",
			Description: "Get a single ClickFunnels contact by ID.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "integer",
						"description": "Numeric ClickFunnels contact ID",
					},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "cf_create_contact",
			Description: "Create a new ClickFunnels contact.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"email_address":     map[string]interface{}{"type": "string", "description": "Contact email address"},
					"first_name":        map[string]interface{}{"type": "string", "description": "First name"},
					"last_name":         map[string]interface{}{"type": "string", "description": "Last name"},
					"phone_number":      map[string]interface{}{"type": "string", "description": "Phone number"},
					"time_zone":         map[string]interface{}{"type": "string", "description": "IANA time zone identifier, e.g. \"America/New_York\""},
					"fb_url":            map[string]interface{}{"type": "string", "description": "Facebook profile URL"},
					"twitter_url":       map[string]interface{}{"type": "string", "description": "Twitter profile URL"},
					"instagram_url":     map[string]interface{}{"type": "string", "description": "Instagram profile URL"},
					"linkedin_url":      map[string]interface{}{"type": "string", "description": "LinkedIn profile URL"},
					"website_url":       map[string]interface{}{"type": "string", "description": "Website URL"},
					"tag_ids":           map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "integer"}, "description": "Array of tag IDs to assign"},
					"custom_attributes": map[string]interface{}{"type": "object", "description": "Key/value custom attributes"},
				},
			},
		},
		{
			Name:        "cf_sync_contacts",
			Description: "Sync all ClickFunnels contacts into the local PocketBase cf_contacts collection. Returns a count of added and updated records.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "cf_update_contact",
			Description: "Update an existing ClickFunnels contact by ID.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":                map[string]interface{}{"type": "integer", "description": "Numeric ClickFunnels contact ID to update"},
					"email_address":     map[string]interface{}{"type": "string", "description": "Contact email address"},
					"first_name":        map[string]interface{}{"type": "string", "description": "First name"},
					"last_name":         map[string]interface{}{"type": "string", "description": "Last name"},
					"phone_number":      map[string]interface{}{"type": "string", "description": "Phone number"},
					"time_zone":         map[string]interface{}{"type": "string", "description": "IANA time zone identifier, e.g. \"America/New_York\""},
					"fb_url":            map[string]interface{}{"type": "string", "description": "Facebook profile URL"},
					"twitter_url":       map[string]interface{}{"type": "string", "description": "Twitter profile URL"},
					"instagram_url":     map[string]interface{}{"type": "string", "description": "Instagram profile URL"},
					"linkedin_url":      map[string]interface{}{"type": "string", "description": "LinkedIn profile URL"},
					"website_url":       map[string]interface{}{"type": "string", "description": "Website URL"},
					"tag_ids":           map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "integer"}, "description": "Array of tag IDs to assign"},
					"custom_attributes": map[string]interface{}{"type": "object", "description": "Key/value custom attributes"},
				},
				"required": []string{"id"},
			},
		},
	}
}

// CallTool executes an MCP tool call
func (s *MCPServer) CallTool(ctx context.Context, req ToolCallRequest) (ToolCallResponse, error) {
	s.logger.Printf("MCP tool call: %s", req.Name)

	switch req.Name {
	case "pb_schema":
		return s.pbSchema(ctx)
	case "pb_list":
		return s.pbList(ctx, req.Arguments)
	case "pb_get":
		return s.pbGet(ctx, req.Arguments)
	case "pb_create":
		return s.pbCreate(ctx, req.Arguments)
	case "pb_update":
		return s.pbUpdate(ctx, req.Arguments)
	case "pb_delete":
		return s.pbDelete(ctx, req.Arguments)
	case "cf_sync_contacts":
		return s.cfSyncContacts(ctx)
	case "cf_list_contacts":
		return s.cfListContacts(ctx, req.Arguments)
	case "cf_get_contact":
		return s.cfGetContact(ctx, req.Arguments)
	case "cf_create_contact":
		return s.cfCreateContact(ctx, req.Arguments)
	case "cf_update_contact":
		return s.cfUpdateContact(ctx, req.Arguments)
	default:
		return ToolCallResponse{
			Content: []ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Unknown tool: %s", req.Name),
			}},
			IsError: true,
		}, fmt.Errorf("unknown tool: %s", req.Name)
	}
}

// pbSchema lists all non-system PocketBase collections and their field schemas.
func (s *MCPServer) pbSchema(ctx context.Context) (ToolCallResponse, error) {
	collections, err := s.store.App().FindAllCollections()
	if err != nil {
		return errorResponse(fmt.Sprintf("failed to list collections: %v", err)), err
	}

	type fieldInfo struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	type collectionInfo struct {
		Name   string      `json:"name"`
		Fields []fieldInfo `json:"fields"`
	}

	var result []collectionInfo
	for _, col := range collections {
		if col.System {
			continue
		}
		info := collectionInfo{Name: col.Name}
		for _, f := range col.Fields {
			info.Fields = append(info.Fields, fieldInfo{
				Name: f.GetName(),
				Type: f.Type(),
			})
		}
		result = append(result, info)
	}

	if len(result) == 0 {
		return textResponse("No non-system collections found."), nil
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return textResponse(fmt.Sprintf("Found %d collection(s):\n\n%s", len(result), string(out))), nil
}

// pbList lists records from a PocketBase collection.
func (s *MCPServer) pbList(ctx context.Context, args map[string]interface{}) (ToolCallResponse, error) {
	collection, _ := args["collection"].(string)
	if _, err := s.resolveUserCollection(collection); err != nil {
		return errorResponse(err.Error()), nil
	}

	filter, _ := args["filter"].(string)
	if filter == "" {
		filter = "id != ''"
	}
	sort, _ := args["sort"].(string)
	if sort == "" {
		sort = "-created"
	}
	limit := 50
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 500 {
			limit = 500
		}
	}
	offset := 0
	if o, ok := args["offset"].(float64); ok && o > 0 {
		offset = int(o)
	}

	records, err := s.store.App().FindRecordsByFilter(collection, filter, sort, limit, offset)
	if err != nil {
		return errorResponse(fmt.Sprintf("query failed: %v", err)), err
	}

	out, _ := json.MarshalIndent(recordsToMaps(records), "", "  ")
	return textResponse(fmt.Sprintf("Found %d record(s) in %s:\n\n%s", len(records), collection, string(out))), nil
}

// pbGet retrieves a single record by ID.
func (s *MCPServer) pbGet(ctx context.Context, args map[string]interface{}) (ToolCallResponse, error) {
	collection, _ := args["collection"].(string)
	if _, err := s.resolveUserCollection(collection); err != nil {
		return errorResponse(err.Error()), nil
	}
	id, _ := args["id"].(string)
	if id == "" {
		return errorResponse("id is required"), nil
	}

	rec, err := s.store.App().FindRecordById(collection, id)
	if err != nil {
		return errorResponse(fmt.Sprintf("record not found: %v", err)), err
	}

	out, _ := json.MarshalIndent(recordToMap(rec), "", "  ")
	return textResponse(string(out)), nil
}

// pbCreate creates a new record in a collection.
func (s *MCPServer) pbCreate(ctx context.Context, args map[string]interface{}) (ToolCallResponse, error) {
	collection, _ := args["collection"].(string)
	col, err := s.resolveUserCollection(collection)
	if err != nil {
		return errorResponse(err.Error()), nil
	}
	fields, ok := args["fields"].(map[string]interface{})
	if !ok || len(fields) == 0 {
		return errorResponse("fields is required"), nil
	}

	rec := core.NewRecord(col)
	for k, v := range fields {
		rec.Set(k, v)
	}
	if err := s.store.App().Save(rec); err != nil {
		return errorResponse(fmt.Sprintf("create failed: %v", err)), err
	}

	out, _ := json.MarshalIndent(recordToMap(rec), "", "  ")
	return textResponse(fmt.Sprintf("Created record in %s:\n\n%s", collection, string(out))), nil
}

// pbUpdate updates fields on an existing record.
func (s *MCPServer) pbUpdate(ctx context.Context, args map[string]interface{}) (ToolCallResponse, error) {
	collection, _ := args["collection"].(string)
	if _, err := s.resolveUserCollection(collection); err != nil {
		return errorResponse(err.Error()), nil
	}
	id, _ := args["id"].(string)
	if id == "" {
		return errorResponse("id is required"), nil
	}
	fields, ok := args["fields"].(map[string]interface{})
	if !ok || len(fields) == 0 {
		return errorResponse("fields is required"), nil
	}

	rec, err := s.store.App().FindRecordById(collection, id)
	if err != nil {
		return errorResponse(fmt.Sprintf("record not found: %v", err)), err
	}
	for k, v := range fields {
		rec.Set(k, v)
	}
	if err := s.store.App().Save(rec); err != nil {
		return errorResponse(fmt.Sprintf("update failed: %v", err)), err
	}

	out, _ := json.MarshalIndent(recordToMap(rec), "", "  ")
	return textResponse(fmt.Sprintf("Updated record in %s:\n\n%s", collection, string(out))), nil
}

// pbDelete deletes a record.
func (s *MCPServer) pbDelete(ctx context.Context, args map[string]interface{}) (ToolCallResponse, error) {
	collection, _ := args["collection"].(string)
	if _, err := s.resolveUserCollection(collection); err != nil {
		return errorResponse(err.Error()), nil
	}
	id, _ := args["id"].(string)
	if id == "" {
		return errorResponse("id is required"), nil
	}

	rec, err := s.store.App().FindRecordById(collection, id)
	if err != nil {
		return errorResponse(fmt.Sprintf("record not found: %v", err)), err
	}
	if err := s.store.App().Delete(rec); err != nil {
		return errorResponse(fmt.Sprintf("delete failed: %v", err)), err
	}

	return textResponse(fmt.Sprintf("Deleted record %s from %s", id, collection)), nil
}

// cfSyncContacts fetches all ClickFunnels contacts and upserts them into cf_contacts.
func (s *MCPServer) cfSyncContacts(ctx context.Context) (ToolCallResponse, error) {
	if s.cfClient == nil {
		return errorResponse("ClickFunnels not configured"), nil
	}

	added, updated, err := s.store.SyncCFContacts(ctx, s.cfClient)
	if err != nil {
		return errorResponse(fmt.Sprintf("sync contacts failed: %v", err)), err
	}

	total := added + updated
	return textResponse(fmt.Sprintf("Synced %d contacts: %d added, %d updated", total, added, updated)), nil
}

// cfListContacts lists contacts from ClickFunnels with optional cursor and tag filters.
func (s *MCPServer) cfListContacts(ctx context.Context, args map[string]interface{}) (ToolCallResponse, error) {
	if s.cfClient == nil {
		return errorResponse("ClickFunnels not configured"), nil
	}

	afterID := 0
	if v, ok := args["after_id"].(float64); ok {
		afterID = int(v)
	}

	var tagIDs []int
	if v, ok := args["tag_ids"].(string); ok && v != "" {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if id, err := strconv.Atoi(part); err == nil {
				tagIDs = append(tagIDs, id)
			}
		}
	}

	contacts, err := s.cfClient.ListContacts(ctx, afterID, 100, tagIDs)
	if err != nil {
		return errorResponse(fmt.Sprintf("list contacts failed: %v", err)), err
	}

	out, _ := json.MarshalIndent(contacts, "", "  ")
	return textResponse(fmt.Sprintf("Found %d contact(s):\n\n%s", len(contacts), string(out))), nil
}

// cfGetContact retrieves a single ClickFunnels contact by numeric ID.
func (s *MCPServer) cfGetContact(ctx context.Context, args map[string]interface{}) (ToolCallResponse, error) {
	if s.cfClient == nil {
		return errorResponse("ClickFunnels not configured"), nil
	}

	idVal, ok := args["id"].(float64)
	if !ok || idVal <= 0 {
		return errorResponse("id is required and must be a positive integer"), nil
	}

	contact, err := s.cfClient.GetContact(ctx, int(idVal))
	if err != nil {
		return errorResponse(fmt.Sprintf("get contact failed: %v", err)), err
	}

	out, _ := json.MarshalIndent(contact, "", "  ")
	return textResponse(string(out)), nil
}

// cfContactParamsFromArgs extracts ContactParams from the MCP arguments map.
func cfContactParamsFromArgs(args map[string]interface{}) clickfunnels.ContactParams {
	strPtr := func(key string) *string {
		if v, ok := args[key].(string); ok && v != "" {
			return &v
		}
		return nil
	}

	var params clickfunnels.ContactParams
	params.EmailAddress = strPtr("email_address")
	params.FirstName = strPtr("first_name")
	params.LastName = strPtr("last_name")
	params.PhoneNumber = strPtr("phone_number")
	params.TimeZone = strPtr("time_zone")
	params.FbURL = strPtr("fb_url")
	params.TwitterURL = strPtr("twitter_url")
	params.InstagramURL = strPtr("instagram_url")
	params.LinkedinURL = strPtr("linkedin_url")
	params.WebsiteURL = strPtr("website_url")

	if raw, ok := args["tag_ids"].([]interface{}); ok {
		for _, v := range raw {
			if f, ok := v.(float64); ok {
				params.TagIDs = append(params.TagIDs, int(f))
			}
		}
	}

	if raw, ok := args["custom_attributes"].(map[string]interface{}); ok {
		params.CustomAttributes = make(map[string]string, len(raw))
		for k, v := range raw {
			if s, ok := v.(string); ok {
				params.CustomAttributes[k] = s
			}
		}
	}

	return params
}

// cfCreateContact creates a new ClickFunnels contact.
func (s *MCPServer) cfCreateContact(ctx context.Context, args map[string]interface{}) (ToolCallResponse, error) {
	if s.cfClient == nil {
		return errorResponse("ClickFunnels not configured"), nil
	}

	params := cfContactParamsFromArgs(args)
	contact, err := s.cfClient.CreateContact(ctx, params)
	if err != nil {
		return errorResponse(fmt.Sprintf("create contact failed: %v", err)), err
	}

	out, _ := json.MarshalIndent(contact, "", "  ")
	return textResponse(fmt.Sprintf("Created contact:\n\n%s", string(out))), nil
}

// cfUpdateContact updates an existing ClickFunnels contact.
func (s *MCPServer) cfUpdateContact(ctx context.Context, args map[string]interface{}) (ToolCallResponse, error) {
	if s.cfClient == nil {
		return errorResponse("ClickFunnels not configured"), nil
	}

	idVal, ok := args["id"].(float64)
	if !ok || idVal <= 0 {
		return errorResponse("id is required and must be a positive integer"), nil
	}

	params := cfContactParamsFromArgs(args)
	contact, err := s.cfClient.UpdateContact(ctx, int(idVal), params)
	if err != nil {
		return errorResponse(fmt.Sprintf("update contact failed: %v", err)), err
	}

	out, _ := json.MarshalIndent(contact, "", "  ")
	return textResponse(fmt.Sprintf("Updated contact:\n\n%s", string(out))), nil
}

// recordToMap serializes a PocketBase record to a flat map for JSON output.
func recordToMap(rec *core.Record) map[string]any {
	m := map[string]any{"id": rec.Id}
	for _, f := range rec.Collection().Fields {
		name := f.GetName()
		m[name] = rec.Get(name)
	}
	return m
}

func recordsToMaps(records []*core.Record) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, rec := range records {
		out = append(out, recordToMap(rec))
	}
	return out
}

// Helper functions

func textResponse(text string) ToolCallResponse {
	return ToolCallResponse{
		Content: []ContentBlock{{
			Type: "text",
			Text: text,
		}},
	}
}

func errorResponse(message string) ToolCallResponse {
	return ToolCallResponse{
		Content: []ContentBlock{{
			Type: "text",
			Text: fmt.Sprintf("Error: %s", message),
		}},
		IsError: true,
	}
}
