package repository

import (
	"context"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func applyHostedResourceFilter(ctx context.Context, selector *entsql.Selector, resourceColumn string) {
	hosted, enabled := service.HostedResourceFilterFromContext(ctx)
	if !enabled {
		return
	}
	bindings := entsql.Table("hosted_openai_accounts")
	match := entsql.Exists(entsql.Select(bindings.C(resourceColumn)).From(bindings).
		Where(entsql.ColumnsEQ(bindings.C(resourceColumn), selector.C("id"))))
	if !hosted {
		match = entsql.Not(match)
	}
	selector.Where(match)
}
