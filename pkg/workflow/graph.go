package workflow

import (
	"fmt"
	"strings"
)

// ActivityGraph defines the directed graph of nodes and transitions.
// Registered graphs are immutable after registration.
type ActivityGraph struct {
	Name      string           `json:"name"`
	StartNode string           `json:"start_node"`
	Nodes     map[string]*Node `json:"nodes"`
}

// Node is one vertex in the graph — one activity execution step.
type Node struct {
	ID           string         `json:"id"`
	ActivityName string         `json:"activity_name"` // key in Engine.registry
	MaxRetries   int            `json:"max_retries"`
	// InputMapping: if set, only these keys from Workflow.Context are passed as input.
	// nil = pass full context.
	InputMapping []string       `json:"input_mapping,omitempty"`
	// Input: static key/value pairs baked into this node. Merged on top of the
	// workflow context when building the activity's input map. Values support
	// {{key}} template interpolation against the workflow context.
	Input        map[string]any `json:"input,omitempty"`
	Transitions      []Transition   `json:"transitions"`
	IsHuman          bool           `json:"is_human"`           // true = human-in-the-middle pause
	NeedsConversion  bool           `json:"needs_conversion,omitempty"` // flagged to be rewritten as a native Go activity
}

// Transition is a directed edge from this node to another, gated by conditions.
// Transitions are evaluated in order; first match wins.
// Empty Conditions = unconditional match (use as the last/default transition).
type Transition struct {
	Conditions []Condition `json:"conditions"`
	NextNode   string      `json:"next_node"` // "" = end workflow
	Label      string      `json:"label,omitempty"` // display label for graph viz
}

// Condition evaluates one assertion against the activity output map.
type Condition struct {
	Key      string `json:"key"`
	// Operators: eq, neq, gt, lt, contains, exists, not_exists
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

// Validate checks that the graph is self-consistent (all next_node refs exist).
func (g *ActivityGraph) Validate(registry map[string]ActivityFunc) error {
	if g.StartNode == "" {
		return fmt.Errorf("%w: start_node is empty", ErrGraphValidation)
	}
	if _, ok := g.Nodes[g.StartNode]; !ok {
		return fmt.Errorf("%w: start_node %q not in nodes", ErrGraphValidation, g.StartNode)
	}
	for id, node := range g.Nodes {
		if node.ActivityName == "" {
			return fmt.Errorf("%w: node %q has empty activity_name", ErrGraphValidation, id)
		}
		if _, ok := registry[node.ActivityName]; !ok {
			return fmt.Errorf("%w: node %q references unregistered activity %q", ErrGraphValidation, id, node.ActivityName)
		}
		for i, t := range node.Transitions {
			if t.NextNode != "" {
				if _, ok := g.Nodes[t.NextNode]; !ok {
					return fmt.Errorf("%w: node %q transition %d references unknown node %q", ErrGraphValidation, id, i, t.NextNode)
				}
			}
		}
	}
	return nil
}

// NextNode evaluates transitions for the given node against activityOutput.
// Returns (nextNodeID, true) if a transition matched, or ("", false) if no match
// (treated as end-of-workflow). An empty nextNodeID with true also means end-of-workflow.
func (g *ActivityGraph) NextNode(nodeID string, output map[string]any) (string, bool) {
	node, ok := g.Nodes[nodeID]
	if !ok {
		return "", false
	}
	for _, t := range node.Transitions {
		if matchAll(t.Conditions, output) {
			return t.NextNode, true
		}
	}
	return "", false
}

// matchAll returns true if all conditions match the output map.
// An empty conditions slice always matches (unconditional/default).
func matchAll(conditions []Condition, output map[string]any) bool {
	for _, c := range conditions {
		if !evalCondition(c, output) {
			return false
		}
	}
	return true
}

func evalCondition(c Condition, output map[string]any) bool {
	raw, exists := output[c.Key]

	switch c.Operator {
	case "exists":
		return exists && raw != nil
	case "not_exists":
		return !exists || raw == nil
	case "eq":
		return coerceEqual(raw, c.Value)
	case "neq":
		return !coerceEqual(raw, c.Value)
	case "gt":
		a, b, ok := toFloat64Pair(raw, c.Value)
		return ok && a > b
	case "lt":
		a, b, ok := toFloat64Pair(raw, c.Value)
		return ok && a < b
	case "gte":
		a, b, ok := toFloat64Pair(raw, c.Value)
		return ok && a >= b
	case "lte":
		a, b, ok := toFloat64Pair(raw, c.Value)
		return ok && a <= b
	case "contains":
		return strings.Contains(fmt.Sprintf("%v", raw), fmt.Sprintf("%v", c.Value))
	}
	return false
}

func coerceEqual(a, b any) bool {
	// JSON numbers come in as float64; handle int/float comparison gracefully.
	af, aok := toFloat64(a)
	bf, bok := toFloat64(b)
	if aok && bok {
		return af == bf
	}
	// Boolean comparison
	if ab, ok := a.(bool); ok {
		if bb, ok := b.(bool); ok {
			return ab == bb
		}
	}
	// String fallback
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	}
	return 0, false
}

func toFloat64Pair(a, b any) (float64, float64, bool) {
	af, aok := toFloat64(a)
	bf, bok := toFloat64(b)
	return af, bf, aok && bok
}

// Eq is a helper for building a Condition with the "eq" operator.
func Eq(key string, value any) Condition {
	return Condition{Key: key, Operator: "eq", Value: value}
}

// Neq builds a not-equal condition.
func Neq(key string, value any) Condition {
	return Condition{Key: key, Operator: "neq", Value: value}
}

// Exists builds an exists condition.
func Exists(key string) Condition {
	return Condition{Key: key, Operator: "exists"}
}

// Always returns an empty Condition slice, meaning unconditional match.
// Use as the last transition to implement a default/else path.
func Always() []Condition { return nil }
