package ablation

import "testing"

func TestDefaultConfigs_ContainsFullBaseline(t *testing.T) {
	if _, ok := FindConfig("full"); !ok {
		t.Fatal("DefaultConfigs must include a 'full' baseline")
	}
	full, _ := FindConfig("full")
	if len(full.Flags) != 0 {
		t.Fatalf("'full' baseline must pass no flags, got %v", full.Flags)
	}
}

func TestFindConfig(t *testing.T) {
	if _, ok := FindConfig("no-types"); !ok {
		t.Fatal("expected no-types config")
	}
	if _, ok := FindConfig("does-not-exist"); ok {
		t.Fatal("unexpected hit for unknown config")
	}
}

func TestSelectConfigs(t *testing.T) {
	all, missing := SelectConfigs(nil)
	if len(all) != len(DefaultConfigs) {
		t.Fatalf("empty list must yield all defaults, got %d", len(all))
	}
	if missing != nil {
		t.Fatalf("expected no missing, got %v", missing)
	}

	sel, missing := SelectConfigs([]string{"full", "no-types", "bogus"})
	if len(sel) != 2 {
		t.Fatalf("expected 2 valid configs, got %d", len(sel))
	}
	if len(missing) != 1 || missing[0] != "bogus" {
		t.Fatalf("expected [bogus] missing, got %v", missing)
	}
}

func TestConfigOrder_FullFirst(t *testing.T) {
	if configOrder("full") != 0 {
		t.Fatalf("'full' must sort first, got %d", configOrder("full"))
	}
	if configOrder("no-types") <= configOrder("full") {
		t.Fatal("non-baseline configs must sort after 'full'")
	}
	if configOrder("unknown-xyz") < 100 {
		t.Fatalf("unknown configs must sort after known, got %d", configOrder("unknown-xyz"))
	}
}
