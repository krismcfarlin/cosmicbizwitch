package storage

import (
	"github.com/pocketbase/pocketbase/core"
)

func createContacts(app core.App) error {
	col := core.NewBaseCollection("contacts")
	col.Fields.Add(
		&core.TextField{Name: "first_name"},
		&core.TextField{Name: "last_name"},
		&core.EmailField{Name: "email"},
		&core.TextField{Name: "phone"},
	)
	addAutodateFields(col)
	return app.Save(col)
}

func createBirthChartRecords(app core.App) error {
	col := core.NewBaseCollection("birth_chart_records")
	col.Fields.Add(
		&core.TextField{Name: "cf_contact_id"},
		&core.JSONField{Name: "chart_data"},
	)
	addAutodateFields(col)
	return app.Save(col)
}

func createNewMoonIntentions(app core.App) error {
	col := core.NewBaseCollection("new_moon_intentions")
	col.Fields.Add(
		&core.TextField{Name: "cf_contact_id"},
		&core.TextField{Name: "intention"},
	)
	addAutodateFields(col)
	return app.Save(col)
}

func createShares(app core.App) error {
	col := core.NewBaseCollection("shares")
	col.Fields.Add(
		&core.TextField{Name: "token", Required: true},
		&core.TextField{Name: "resource_type"},
		&core.TextField{Name: "resource_id"},
	)
	addAutodateFields(col)
	return app.Save(col)
}

func createReportLinks(app core.App) error {
	col := core.NewBaseCollection("report_links")
	col.Fields.Add(
		&core.TextField{Name: "cf_contact_id"},
		&core.TextField{Name: "url"},
		&core.TextField{Name: "report_type"},
	)
	addAutodateFields(col)
	return app.Save(col)
}

func createUtmSource(app core.App) error {
	col := core.NewBaseCollection("utm_source")
	col.Fields.Add(
		&core.TextField{Name: "source"},
		&core.TextField{Name: "medium"},
		&core.TextField{Name: "campaign"},
		&core.TextField{Name: "cf_contact_id"},
	)
	addAutodateFields(col)
	return app.Save(col)
}
