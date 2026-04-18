package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cosmicbizwitch/pkg/workflow"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// newPBQueryFn returns a testable ActivityFunc for pb_query.
func newPBQueryFn(app core.App) workflow.ActivityFunc {
	return func(_ context.Context, input map[string]any) (map[string]any, error) {
		tableName, _ := input["table_name"].(string)
		if tableName == "" {
			return nil, fmt.Errorf("pb_query: table_name is required")
		}

		limitF, _ := input["limit"].(float64)
		limit := int(limitF)
		if limit <= 0 {
			limit = 50
		}

		filterStr := ""
		params := dbx.Params{}
		rawFilters, _ := input["filters"].([]any)
		if len(rawFilters) > 0 {
			filterMode, _ := input["filter_mode"].(string)
			sep := " && "
			if filterMode == "or" {
				sep = " || "
			}
			parts := []string{}
			for i, rf := range rawFilters {
				rule, ok := rf.(map[string]any)
				if !ok {
					continue
				}
				field, _ := rule["field"].(string)
				operator, _ := rule["operator"].(string)
				value := rule["value"]
				if field == "" || operator == "" {
					continue
				}
				key := fmt.Sprintf("v%d", i)
				parts = append(parts, fmt.Sprintf("%s %s {:%s}", field, pbQueryOp(operator), key))
				params[key] = workflow.FmtVal(value)
			}
			if len(parts) > 0 {
				filterStr = strings.Join(parts, sep)
			}
		}
		if filterStr == "" {
			rawFilter, _ := input["filter"].(string)
			filterStr = rawFilter
		}
		if filterStr == "" {
			filterStr = "1=1"
		}

		sort := ""
		sortField, _ := input["sort_field"].(string)
		sortDir, _ := input["sort_dir"].(string)
		if sortField != "" {
			if sortDir == "desc" {
				sort = "-" + sortField
			} else {
				sort = sortField
			}
		}

		records, err := app.FindRecordsByFilter(tableName, filterStr, sort, limit, 0, params)
		if err != nil {
			return nil, fmt.Errorf("pb_query: %w", err)
		}

		out := map[string]any{
			"found": len(records) > 0,
			"count": len(records),
		}

		recList := make([]any, 0, len(records))
		for _, rec := range records {
			m := map[string]any{"id": rec.Id}
			for _, col := range rec.Collection().Fields {
				m[col.GetName()] = rec.Get(col.GetName())
			}
			recList = append(recList, m)
		}
		out["records"] = recList

		if len(records) > 0 {
			rec := records[0]
			out["id"] = rec.Id
			for _, col := range rec.Collection().Fields {
				out[col.GetName()] = rec.Get(col.GetName())
			}
		}

		resultKey, _ := input["result_key"].(string)
		if resultKey != "" {
			return map[string]any{resultKey: out}, nil
		}
		return out, nil
	}
}

// newPBCreateFn returns a testable ActivityFunc for pb_create.
func newPBCreateFn(app core.App) workflow.ActivityFunc {
	return func(_ context.Context, input map[string]any) (map[string]any, error) {
		tableName, _ := input["table_name"].(string)
		if tableName == "" {
			return nil, fmt.Errorf("pb_create: table_name is required")
		}
		data, ok := input["data"].(map[string]any)
		if !ok || data == nil {
			return nil, fmt.Errorf("pb_create: data is required and must be an object")
		}

		col, err := app.FindCollectionByNameOrId(tableName)
		if err != nil {
			return nil, fmt.Errorf("pb_create: find collection %q: %w", tableName, err)
		}

		rec := core.NewRecord(col)
		for k, v := range data {
			rec.Set(k, v)
		}

		if err := app.Save(rec); err != nil {
			return nil, fmt.Errorf("pb_create: save: %w", err)
		}

		recMap := make(map[string]any)
		for _, f := range rec.Collection().Fields {
			recMap[f.GetName()] = rec.Get(f.GetName())
		}
		out := map[string]any{"id": rec.Id, "record": recMap}
		if rk, _ := input["result_key"].(string); rk != "" {
			return map[string]any{rk: out}, nil
		}
		return out, nil
	}
}

// newPBUpdateFn returns a testable ActivityFunc for pb_update.
func newPBUpdateFn(app core.App) workflow.ActivityFunc {
	return func(_ context.Context, input map[string]any) (map[string]any, error) {
		tableName, _ := input["table_name"].(string)
		if tableName == "" {
			return nil, fmt.Errorf("pb_update: table_name is required")
		}
		id, _ := input["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("pb_update: id is required")
		}
		data, ok := input["data"].(map[string]any)
		if !ok || data == nil {
			return nil, fmt.Errorf("pb_update: data is required and must be an object")
		}

		rec, err := app.FindRecordById(tableName, id)
		if err != nil {
			return nil, fmt.Errorf("pb_update: find record %q in %q: %w", id, tableName, err)
		}

		for k, v := range data {
			rec.Set(k, v)
		}

		if err := app.Save(rec); err != nil {
			return nil, fmt.Errorf("pb_update: save: %w", err)
		}

		recMap := make(map[string]any)
		for _, f := range rec.Collection().Fields {
			recMap[f.GetName()] = rec.Get(f.GetName())
		}
		out := map[string]any{"id": rec.Id, "record": recMap}
		if rk, _ := input["result_key"].(string); rk != "" {
			return map[string]any{rk: out}, nil
		}
		return out, nil
	}
}

// newPBDeleteFn returns a testable ActivityFunc for pb_delete.
func newPBDeleteFn(app core.App) workflow.ActivityFunc {
	return func(_ context.Context, input map[string]any) (map[string]any, error) {
		tableName, _ := input["table_name"].(string)
		if tableName == "" {
			return nil, fmt.Errorf("pb_delete: table_name is required")
		}
		id, _ := input["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("pb_delete: id is required")
		}

		rec, err := app.FindRecordById(tableName, id)
		if err != nil {
			return nil, fmt.Errorf("pb_delete: find record %q in %q: %w", id, tableName, err)
		}

		if err := app.Delete(rec); err != nil {
			return nil, fmt.Errorf("pb_delete: delete: %w", err)
		}

		out := map[string]any{"deleted": true, "id": id}
		if rk, _ := input["result_key"].(string); rk != "" {
			return map[string]any{rk: out}, nil
		}
		return out, nil
	}
}

// newPBUpsertFn returns a testable ActivityFunc for pb_upsert.
func newPBUpsertFn(app core.App) workflow.ActivityFunc {
	return func(_ context.Context, input map[string]any) (map[string]any, error) {
		tableName, _ := input["table_name"].(string)
		if tableName == "" {
			return nil, fmt.Errorf("pb_upsert: table_name is required")
		}
		data, ok := input["data"].(map[string]any)
		if !ok || data == nil {
			return nil, fmt.Errorf("pb_upsert: data is required and must be an object (got %T: %v)", input["data"], input["data"])
		}

		id, _ := input["id"].(string)

		dataJSON, _ := json.Marshal(data)
		fmt.Printf("[pb_upsert] table=%q id=%q data=%s\n", tableName, id, dataJSON)

		var rec *core.Record
		whereFilters, _ := input["where_filters"].([]any)
		if len(whereFilters) > 0 {
			filterMode, _ := input["where_filter_mode"].(string)
			sep := " && "
			if filterMode == "or" {
				sep = " || "
			}
			parts := []string{}
			params := dbx.Params{}
			for i, rf := range whereFilters {
				rule, ok := rf.(map[string]any)
				if !ok {
					fmt.Printf("[pb_upsert] where_filters[%d] is not a map: %T %v\n", i, rf, rf)
					continue
				}
				field, _ := rule["field"].(string)
				operator, _ := rule["operator"].(string)
				value := rule["value"]
				fmt.Printf("[pb_upsert] where rule[%d]: field=%q op=%q value=%v (type %T)\n", i, field, operator, value, value)
				if field == "" || operator == "" {
					continue
				}
				key := fmt.Sprintf("v%d", i)
				parts = append(parts, fmt.Sprintf("%s %s {:%s}", field, pbQueryOp(operator), key))
				params[key] = workflow.FmtVal(value)
			}
			if len(parts) == 0 {
				return nil, fmt.Errorf("pb_upsert: where_filters provided but no valid rules")
			}
			filterStr := strings.Join(parts, sep)
			fmt.Printf("[pb_upsert] filter=%q params=%v\n", filterStr, params)
			col, err := app.FindCollectionByNameOrId(tableName)
			if err != nil {
				return nil, fmt.Errorf("pb_upsert: find collection %q: %w", tableName, err)
			}
			records, err := app.FindRecordsByFilter(tableName, filterStr, "", 1, 0, params)
			if err != nil {
				return nil, fmt.Errorf("pb_upsert: where query: %w", err)
			}
			fmt.Printf("[pb_upsert] found %d existing records\n", len(records))
			if len(records) > 0 {
				rec = records[0]
			} else {
				rec = core.NewRecord(col)
			}
		} else if id != "" {
			existing, err := app.FindRecordById(tableName, id)
			if err != nil {
				return nil, fmt.Errorf("pb_upsert: find record %q in %q: %w", id, tableName, err)
			}
			rec = existing
		} else {
			col, err := app.FindCollectionByNameOrId(tableName)
			if err != nil {
				return nil, fmt.Errorf("pb_upsert: find collection %q: %w", tableName, err)
			}
			rec = core.NewRecord(col)
		}

		fmt.Printf("[pb_upsert] setting %d data fields on record id=%q:\n", len(data), rec.Id)
		for k, v := range data {
			fmt.Printf("[pb_upsert]   data[%q] type=%T value=%#v\n", k, v, v)
			rec.Set(k, v)
		}
		fmt.Printf("[pb_upsert] pre-save record fields:\n")
		for _, f := range rec.Collection().Fields {
			fv := rec.Get(f.GetName())
			fmt.Printf("[pb_upsert]   field[%q] type=%T value=%#v\n", f.GetName(), fv, fv)
		}
		if err := app.Save(rec); err != nil {
			details := make([]string, 0, len(data))
			for k, v := range data {
				details = append(details, fmt.Sprintf("%s=%T(%#v)", k, v, v))
			}
			return nil, fmt.Errorf("pb_upsert: save: %w | data: %s", err, strings.Join(details, ", "))
		}

		recMap := make(map[string]any)
		for _, f := range rec.Collection().Fields {
			recMap[f.GetName()] = rec.Get(f.GetName())
		}
		out := map[string]any{"id": rec.Id, "created": id == "", "record": recMap}
		if rk, _ := input["result_key"].(string); rk != "" {
			return map[string]any{rk: out}, nil
		}
		return out, nil
	}
}
