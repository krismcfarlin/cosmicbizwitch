package workflows

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"cosmicbizwitch/pkg/workflow"
)

var wordRe = regexp.MustCompile(`[A-Za-z]+`)

func registerAstrologyActivities(eng *workflow.Engine) {
	eng.RegisterActivityWithMeta("money_mode",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			raw, _ := input["sign"].(string)

			type signInfo struct {
				mode  string
				label string
			}

			signs := map[string]signInfo{
				"aries":       {"cardinal", "Serial Monogamist"},
				"taurus":      {"fixed", "Lifer"},
				"gemini":      {"mutable", "Polyamorist"},
				"cancer":      {"cardinal", "Serial Monogamist"},
				"leo":         {"fixed", "Lifer"},
				"virgo":       {"mutable", "Polyamorist"},
				"libra":       {"cardinal", "Serial Monogamist"},
				"scorpio":     {"fixed", "Lifer"},
				"sagittarius": {"mutable", "Polyamorist"},
				"capricorn":   {"cardinal", "Serial Monogamist"},
				"aquarius":    {"fixed", "Lifer"},
				"pisces":      {"mutable", "Polyamorist"},
			}

			// Find the first word in the string that matches a zodiac sign.
			var sign string
			var info signInfo
			for _, word := range wordRe.FindAllString(raw, -1) {
				if si, ok := signs[strings.ToLower(word)]; ok {
					sign = word
					info = si
					break
				}
			}
			if sign == "" {
				return nil, fmt.Errorf("money_mode: no zodiac sign found in %q", raw)
			}

			out := map[string]any{
				"sign":       sign,
				"mode":       info.mode,
				"money_mode": info.label,
			}

			// Optional: store result under a user-defined key.
			if outputKey := strings.TrimSpace(func() string { s, _ := input["output_key"].(string); return s }()); outputKey != "" {
				out[outputKey] = info.label
			}

			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "Astrology",
			Description: "Determines money mode (Serial Monogamist / Polyamorist / Lifer) from the sign on the cusp of the 2nd house. Parses the first zodiac sign found in the input string.",
			InputFields: []workflow.FieldMeta{
				{
					Name:        "sign",
					Type:        "context_field",
					Required:    true,
					Description: `Select the context field holding the 2H cusp description, e.g. "Sagittarius on 2H cusp, ruled by Jupiter...". The first zodiac sign word is extracted automatically.`,
				},
				{
					Name:        "output_key",
					Type:        "string",
					Description: "Optional. Name of an extra context key to store the money mode result under (e.g. chart_money_mode).",
				},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "sign", Type: "string", Description: "The zodiac sign extracted from the input."},
				{Name: "mode", Type: "string", Description: "cardinal, fixed, or mutable."},
				{Name: "money_mode", Type: "string", Description: "Serial Monogamist, Polyamorist, or Lifer."},
				{Name: "(output_key)", Type: "string", Description: "Additional key storing money_mode if output_key is set."},
			},
		},
	)
}
