package version

import "go.uber.org/fx"

// Module provides version information to the Fx dependency graph.
var Module = fx.Module("version",
	fx.Provide(Info),
)
