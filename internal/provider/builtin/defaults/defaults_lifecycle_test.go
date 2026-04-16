package defaults

import (
	"context"
	"errors"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// defaults lifecycle tests modeled after apt's U-pattern. Resource ID
// format is "domain.key=type:value" per parseDefaultsResource.

const (
	sampleDomain = "com.apple.dock"
	sampleKey    = "autohide"
	sampleType   = "bool"
	sampleValue  = "true"
)

// resourceID returns the canonical ID form for a (domain, key, type,
// value) tuple. Declared at package scope so the test helper's
// argument structure matches the production parser (parseDefaultsResource)
// even though most tests pass the sample constants — the missing-key
// case in U6 passes a non-sample key, and keeping the signature general
// makes the test's "build an ID" intent visually explicit.
var resourceID = func(domain, key, typeStr, value string) string {
	return domain + "." + key + "=" + typeStr + ":" + value
}

// U1 — Apply writes a value via the runner and the fake records the
// new association.
func TestU1_Apply_WritesValue(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(nil, fake)

	id := resourceID(sampleDomain, sampleKey, sampleType, sampleValue)
	if err := p.Apply(context.Background(), provider.Action{ID: id}); err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if got := fake.ValueOf(sampleDomain, sampleKey); got != sampleValue {
		t.Errorf("post-Apply: %s.%s = %q, want %q", sampleDomain, sampleKey, got, sampleValue)
	}
	if got := fake.CallCount(fakeOpWrite, sampleDomain+":"+sampleKey); got != 1 {
		t.Errorf("Write call count = %d, want 1", got)
	}
}

// U2 — Apply with malformed ID returns the parser error without
// invoking the runner.
func TestU2_Apply_RejectsMalformedID(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(nil, fake)

	err := p.Apply(context.Background(), provider.Action{ID: "not-a-valid-id"})
	if err == nil {
		t.Fatal("expected error for malformed ID")
	}
	if got := fake.CallCount(fakeOpWrite, ""); got != 0 {
		t.Errorf("Write should not have been called for malformed ID; got %d calls", got)
	}
}

// U3 — Apply propagates Write errors.
func TestU3_Apply_FailurePropagated(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated write failure")
	fake := NewFakeCmdRunner().WithWriteError(sampleDomain, sampleKey, wantErr)
	p := New(nil, fake)

	id := resourceID(sampleDomain, sampleKey, sampleType, sampleValue)
	gotErr := p.Apply(context.Background(), provider.Action{ID: id})
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Apply error = %v, want %v", gotErr, wantErr)
	}
}

// U4 — Remove calls Delete on the runner and the fake drops the pref.
func TestU4_Remove_DeletesValue(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().Seed(sampleDomain, sampleKey, sampleValue)
	p := New(nil, fake)

	id := resourceID(sampleDomain, sampleKey, sampleType, sampleValue)
	if err := p.Remove(context.Background(), id); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if got := fake.ValueOf(sampleDomain, sampleKey); got != "" {
		t.Errorf("post-Remove: %s.%s = %q, want empty", sampleDomain, sampleKey, got)
	}
	if got := fake.CallCount(fakeOpDelete, sampleDomain+":"+sampleKey); got != 1 {
		t.Errorf("Delete call count = %d, want 1", got)
	}
}

// U5 — Remove failure is propagated; the pref stays.
func TestU5_Remove_FailureLeavesValue(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated delete failure")
	fake := NewFakeCmdRunner().
		Seed(sampleDomain, sampleKey, sampleValue).
		WithDeleteError(sampleDomain, sampleKey, wantErr)
	p := New(nil, fake)

	id := resourceID(sampleDomain, sampleKey, sampleType, sampleValue)
	gotErr := p.Remove(context.Background(), id)
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Remove error = %v, want %v", gotErr, wantErr)
	}
	if got := fake.ValueOf(sampleDomain, sampleKey); got != sampleValue {
		t.Errorf("on Remove failure, pref should remain %q, got %q", sampleValue, got)
	}
}

// U6 — Probe reads current values via the runner and reports StateOK
// with the value, or StateFailed when the key is absent.
func TestU6_Probe_OKAndFailedClassification(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().Seed(sampleDomain, sampleKey, sampleValue)
	p := New(nil, fake)

	sf := state.New("defaults", "test-machine")
	sf.SetResource(resourceID(sampleDomain, sampleKey, sampleType, sampleValue), state.StateOK)
	sf.SetResource(resourceID(sampleDomain, "missing-key", sampleType, sampleValue), state.StateOK)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Probe returned %d results, want 2", len(results))
	}

	byID := map[string]provider.ProbeResult{}
	for _, r := range results {
		byID[r.ID] = r
	}
	present := byID[resourceID(sampleDomain, sampleKey, sampleType, sampleValue)]
	if present.State != state.StateOK || present.Value != sampleValue {
		t.Errorf("present: state=%v value=%q, want StateOK %q", present.State, present.Value, sampleValue)
	}
	missing := byID[resourceID(sampleDomain, "missing-key", sampleType, sampleValue)]
	if missing.State != state.StateFailed {
		t.Errorf("missing: state=%v, want StateFailed", missing.State)
	}
}

// U7 — Probe flags malformed resource IDs as StateFailed without
// invoking the runner.
func TestU7_Probe_MalformedIDReportedAsFailed(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(nil, fake)

	sf := state.New("defaults", "test-machine")
	sf.SetResource("garbage", state.StateOK)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Probe returned %d results, want 1", len(results))
	}
	if results[0].State != state.StateFailed {
		t.Errorf("state = %v, want StateFailed", results[0].State)
	}
	if got := fake.CallCount(fakeOpRead, ""); got != 0 {
		t.Errorf("Read should not be called for malformed ID; got %d calls", got)
	}
}

// U8 — Probe skips StateRemoved entries.
func TestU8_Probe_SkipsRemovedResources(t *testing.T) {
	t.Parallel()
	p := New(nil, NewFakeCmdRunner())
	sf := state.New("defaults", "test-machine")
	sf.SetResource(resourceID(sampleDomain, sampleKey, sampleType, sampleValue), state.StateRemoved)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Probe returned %d results for StateRemoved, want 0", len(results))
	}
}

// U9 — Bootstrap delegates to runner.LookPath.
func TestU9_Bootstrap_DelegatesLookPath(t *testing.T) {
	t.Parallel()

	t.Run("present", func(t *testing.T) {
		t.Parallel()
		p := New(nil, NewFakeCmdRunner())
		if err := p.Bootstrap(context.Background()); err != nil {
			t.Errorf("Bootstrap error = %v, want nil", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		want := errors.New("defaults not found (non-macOS host)")
		p := New(nil, NewFakeCmdRunner().WithLookPathError(want))
		if err := p.Bootstrap(context.Background()); !errors.Is(err, want) {
			t.Errorf("Bootstrap error = %v, want %v", err, want)
		}
	})
}

// TestU10_Plan_WrapsComputePlanWithHooks drives Plan end-to-end:
// desired hamsfile → ComputePlan → PopulateActionHooks. Regression
// guard for a previously 0% branch. Verifies actions are produced
// and empty hamsfile yields empty actions (not an error).
func TestU10_Plan_WrapsComputePlanWithHooks(t *testing.T) {
	t.Parallel()

	t.Run("populated", func(t *testing.T) {
		t.Parallel()
		yamlDoc := `
dock:
  - urn: urn:hams:defaults:com.apple.dock.autohide=bool:true
  - urn: urn:hams:defaults:com.apple.dock.tilesize=int:36
`
		var root yaml.Node
		if err := yaml.Unmarshal([]byte(yamlDoc), &root); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		hf := &hamsfile.File{Path: "test.yaml", Root: &root}

		p := New(nil, NewFakeCmdRunner())
		observed := state.New("defaults", "test")
		actions, err := p.Plan(context.Background(), hf, observed)
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		if len(actions) != 2 {
			t.Fatalf("got %d actions, want 2", len(actions))
		}
		for _, a := range actions {
			if a.Type != provider.ActionInstall {
				t.Errorf("action %q has Type=%v, want Install (no observed state)", a.ID, a.Type)
			}
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		empty := &hamsfile.File{Path: "empty.yaml", Root: &yaml.Node{Kind: yaml.DocumentNode}}
		p := New(nil, NewFakeCmdRunner())
		observed := state.New("defaults", "test")
		actions, err := p.Plan(context.Background(), empty, observed)
		if err != nil {
			t.Fatalf("Plan(empty): %v", err)
		}
		if len(actions) != 0 {
			t.Errorf("empty hamsfile produced %d actions, want 0", len(actions))
		}
	})
}
