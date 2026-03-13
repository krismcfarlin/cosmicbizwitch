package pbstore

import (
	"github.com/pocketbase/pocketbase/core"
)

// CreateCollections creates the wf_workflows, wf_activity_instances, wf_graphs, and wf_triggers
// collections if they don't already exist. Call this from your storage.SetupCollections.
func CreateCollections(app core.App) error {
	if err := createWorkflowsCollection(app); err != nil {
		return err
	}
	if err := createActivityInstancesCollection(app); err != nil {
		return err
	}
	if err := createGraphsCollection(app); err != nil {
		return err
	}
	return createTriggersCollection(app)
}

func createWorkflowsCollection(app core.App) error {
	if existing, _ := app.FindCollectionByNameOrId("wf_workflows"); existing != nil {
		return nil
	}

	col := core.NewBaseCollection("wf_workflows")
	col.Fields.Add(
		&core.TextField{Name: "name", Max: 255},
		&core.TextField{Name: "graph_name", Required: true, Max: 100},
		&core.TextField{Name: "status", Required: true, Max: 50},
		&core.DateField{Name: "not_before"},
		&core.DateField{Name: "started_at"},
		&core.DateField{Name: "finished_at"},
		&core.TextField{Name: "current_node", Max: 100},
		&core.JSONField{Name: "context"},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)

	return app.Save(col)
}

func createActivityInstancesCollection(app core.App) error {
	if existing, _ := app.FindCollectionByNameOrId("wf_activity_instances"); existing != nil {
		return nil
	}

	wfCol, err := app.FindCollectionByNameOrId("wf_workflows")
	if err != nil {
		return err
	}

	col := core.NewBaseCollection("wf_activity_instances")
	col.Fields.Add(
		&core.RelationField{Name: "workflow_id", CollectionId: wfCol.Id, Required: true, MaxSelect: 1},
		&core.TextField{Name: "node_id", Required: true, Max: 100},
		&core.TextField{Name: "activity_name", Required: true, Max: 100},
		&core.TextField{Name: "status", Required: true, Max: 50},
		&core.JSONField{Name: "input"},
		&core.JSONField{Name: "output"},
		&core.TextField{Name: "error_msg", Max: 5000},
		&core.NumberField{Name: "error_count"},
		&core.NumberField{Name: "max_retries"},
		&core.DateField{Name: "started_at"},
		&core.DateField{Name: "finished_at"},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)

	return app.Save(col)
}

func createTriggersCollection(app core.App) error {
	if existing, _ := app.FindCollectionByNameOrId("wf_triggers"); existing != nil {
		return nil
	}
	col := core.NewBaseCollection("wf_triggers")
	col.Fields.Add(
		&core.TextField{Name: "name", Required: true, Max: 255},
		&core.TextField{Name: "type", Required: true, Max: 50},      // webhook|record_hook|cron
		&core.TextField{Name: "graph_name", Required: true, Max: 100},
		&core.JSONField{Name: "config"},                               // type-specific config
		&core.TextField{Name: "token", Max: 100},                     // webhook: secret token
		&core.BoolField{Name: "enabled"},
		&core.DateField{Name: "last_fired"},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	return app.Save(col)
}

func createGraphsCollection(app core.App) error {
	if existing, _ := app.FindCollectionByNameOrId("wf_graphs"); existing != nil {
		return nil
	}

	col := core.NewBaseCollection("wf_graphs")
	col.Fields.Add(
		&core.TextField{Name: "name", Required: true, Max: 100},
		&core.JSONField{Name: "definition", Required: true},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)

	return app.Save(col)
}
