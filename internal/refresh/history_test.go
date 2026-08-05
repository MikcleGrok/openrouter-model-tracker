package refresh

import (
	"testing"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/pricehistory"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

func TestPriceChangesUsesLatestObservation(t *testing.T) {
	history := &pricehistory.History{Observations: []pricehistory.Observation{{ObservedAt: time.Unix(1, 0).UTC(), Prices: map[string]pricehistory.Price{
		"a/model": {Found: true, InPerM: 1, OutPerM: 2, Context: 1000},
	}}}}
	changes := priceChanges(history, map[string]sources.PriceInfo{
		"a/model": {Slug: "a/model", Found: true, InPerM: 1.5, OutPerM: 2, Context: 1000},
		"b/model": {Slug: "b/model", Found: true, InPerM: 2, OutPerM: 3, Context: 2000},
	}, true)
	if len(changes) != 1 || changes[0].Slug != "a/model" || changes[0].Previous.InPerM != 1 || changes[0].Current.InPerM != 1.5 {
		t.Fatalf("changes = %+v", changes)
	}
	if got := priceChanges(history, nil, false); got != nil {
		t.Fatalf("failed live lookup changes = %+v, want nil", got)
	}
}

func TestPriceChangesReportsOverrideOnlyChange(t *testing.T) {
	history := &pricehistory.History{Observations: []pricehistory.Observation{{Prices: map[string]pricehistory.Price{
		"a/model": {Found: true, InPerM: 1, OutPerM: 2, Context: 1000, HasOverride: true, OverrideMinTokens: 500000, OverrideInPerM: 1.5, OverrideOutPerM: 4},
	}}}}
	changes := priceChanges(history, map[string]sources.PriceInfo{
		"a/model": {Slug: "a/model", Found: true, InPerM: 1, OutPerM: 2, Context: 1000, HasOverride: true, OverrideMinTokens: 500000, OverrideInPerM: 2, OverrideOutPerM: 4},
	}, true)
	if len(changes) != 1 || changes[0].Previous.OverrideInPerM != 1.5 || changes[0].Current.OverrideInPerM != 2 {
		t.Fatalf("override-only changes = %+v", changes)
	}
}
