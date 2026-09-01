// Parameter extractors: convert raw JSON params to typed option structs.
package engine

import "github.com/diff-mem/diff-mem/internal/model"

func extractCreate(params map[string]interface{}) model.CreateOptions {
	opts := model.CreateOptions{}
	if v, ok := params["path"].(string); ok {
		opts.Path = v
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
	return opts
}

func extractAppend(params map[string]interface{}) model.AppendOptions {
	opts := model.AppendOptions{}
	if v, ok := params["path"].(string); ok {
		opts.Path = v
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
		opts.Path = v
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
		opts.Path = v
	}
	if v, ok := params["reason"].(string); ok {
		opts.Reason = v
	}
	return opts
}

func extractUpdateSummary(params map[string]interface{}) model.UpdateSummaryOptions {
	opts := model.UpdateSummaryOptions{}
	if v, ok := params["path"].(string); ok {
		opts.Path = v
	}
	if v, ok := params["old_summary"].(string); ok {
		opts.OldSummary = v
	}
	if v, ok := params["new_summary"].(string); ok {
		opts.NewSummary = v
	}
	if v, ok := params["reason"].(string); ok {
		opts.Reason = v
	}
	return opts
}

func extractSearch(params map[string]interface{}) model.SearchOptions {
	opts := model.SearchOptions{}
	if v, ok := params["keywords"].(string); ok {
		opts.Keywords = v
	}
	if v, ok := params["limit"].(float64); ok {
		opts.Limit = int(v)
	}
	if v, ok := params["include_archived"].(bool); ok {
		opts.IncludeArchived = v
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

func extractLink(params map[string]interface{}) model.LinkOptions {
	opts := model.LinkOptions{}
	if v, ok := params["from"].(string); ok {
		opts.From = v
	}
	if v, ok := params["to"].(string); ok {
		opts.To = v
	}
	if v, ok := params["type"].(string); ok {
		opts.Type = model.EdgeType(v)
	}
	if v, ok := params["reason"].(string); ok {
		opts.Reason = v
	}
	return opts
}
