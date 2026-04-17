package cli

import "github.com/zthxxx/hams/internal/provider"

// acquireMutationLock is the package-level seam for the single-writer
// state lock per the cli-architecture spec. Production wires it to
// provider.AcquireMutationLock (cycle 221's helper). Tests that need
// to exercise downstream code paths without contending for the real
// .lock file (e.g., the save-failure-ordering scenario in
// refresh_test.go) override this with a no-op.
//
// Override pattern in tests:
//
//	original := acquireMutationLock
//	t.Cleanup(func() { acquireMutationLock = original })
//	acquireMutationLock = func(_, _ string) (func(), error) {
//	    return func() {}, nil
//	}
var acquireMutationLock = provider.AcquireMutationLock
