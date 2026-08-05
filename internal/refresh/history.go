package refresh

import (
	"sort"

	"github.com/sboborikin/openrouter-model-tracker/internal/pricehistory"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

type PriceChange struct {
	Slug     string
	Previous pricehistory.Price
	Current  pricehistory.Price
}

func priceChanges(history *pricehistory.History, prices map[string]sources.PriceInfo, pricesOK bool) []PriceChange {
	if !pricesOK || history == nil || len(history.Observations) == 0 {
		return nil
	}
	previous := history.Observations[len(history.Observations)-1].Prices
	current := pricehistory.FromPrices(prices)
	changes := make([]PriceChange, 0)
	for slug, now := range current {
		before, ok := previous[slug]
		if ok && !pricehistory.Equal(before, now) {
			changes = append(changes, PriceChange{Slug: slug, Previous: before, Current: now})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Slug < changes[j].Slug })
	return changes
}
