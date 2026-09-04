// Parameter extractors: convert raw JSON params to typed option structs.
package engine

import "github.com/diff-mem/diff-mem/internal/model"

func extractCreate(params map[string]interface{}) model.CreateOptions {
	opts := model.CreateOptions{}
	if v, ok := params["path"].(string); ok {
		opts.Path = NormalizePath(v)
	}
	if v, ok := params["title"].(string); ok {
		opts.Title = v
	}
	if v, ok := params["summary"].(string); ok {
		opts.Summary = v
	}
	if v, ok := params["reason"].(string); ok {
		opts.Reason = v
	}
	if v, ok := params["tags"].([]interface{}); ok {
		for _, t := range v {
			if s, ok := t.(string); ok {
				opts.Tags = append(opts.Tags, s)
			}
		}
	}
	if v, ok := params["initial_events"].([]interface{}); ok {
		for _, ev := range v {
			if s, ok := ev.(string); ok {
				opts.InitialEvents = append(opts.InitialEvents, s)
			}
		}
	}
	return opts
}

func extractAppend(params map[string]interface{}) model.AppendOptions {
	opts := model.AppendOptions{}
	if v, ok := params["path"].(string); ok {
		opts.Path = NormalizePath(v)
	}
	if v, ok := params["event"].(string); ok {
		opts.Event = v
	}
	if v, ok := params["reason"].(string); ok {
		opts.Reason = v
	}
	return opts
}

func extractUpdateField(params map[string]interface{}) model.UpdateFieldOptions {
	opts := model.UpdateFieldOptions{}
	if v, ok := params["path"].(string); ok {
		opts.Path = NormalizePath(v)
	}
	if v, ok := params["field"].(string); ok {
		opts.Field = v
	}
	if v, ok := params["value"].(string); ok {
		opts.Value = v
	}
	if v, ok := params["reason"].(string); ok {
		opts.Reason = v
	}
	return opts
}

func extractArchive(params map[string]interface{}) model.ArchiveOptions {
	opts := model.ArchiveOptions{}
	if v, ok := params["path"].(string); ok {
		opts.Path = NormalizePath(v)
	}
	if v, ok := params["reason"].(string); ok {
		opts.Reason = v
	}
	return opts
}

func extractUpdate(params map[string]interface{}) model.UpdateOptions {
	opts := model.UpdateOptions{}
	if v, ok := params["path"].(string); ok {
		opts.Path = NormalizePath(v)
	}
	if v, ok := params["reason"].(string); ok {
		opts.FieldReason = v
	}
	if v, ok := params["fields"].(map[string]interface{}); ok {
		opts.Fields = make(map[string]string, len(v))
		for k, val := range v {
			if s, ok := val.(string); ok {
				opts.Fields[k] = s
			}
		}
	}
	if v, ok := params["summary"].(map[string]interface{}); ok {
		if s, ok := v["old"].(string); ok {
			opts.OldSummary = s
		}
		if s, ok := v["new"].(string); ok {
			opts.NewSummary = s
		}
		if s, ok := v["reason"].(string); ok {
			opts.SummaryReason = s
		}
	}
	return opts
}

func extractLifecycle(params map[string]interface{}) (string, model.ArchiveOptions) {
	action, _ := params["action"].(string)
	return action, extractArchive(params)
}

func extractSearch(params map[string]interface{}) model.SearchOptions {
	opts := model.SearchOptions{}
	if v, ok := params["keywords"].(string); ok {
		opts.Keywords = v
	}
	if v, ok := params["limit"].(float64); ok {
		opts.Limit = int(v)
	}
	if v, ok := params["tags"].([]interface{}); ok {
		for _, t := range v {
			if s, ok := t.(string); ok {
				opts.Tags = append(opts.Tags, s)
			}
		}
	}
	return opts
}
