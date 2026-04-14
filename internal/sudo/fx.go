package sudo

import "go.uber.org/fx"

// Module provides production sudo implementations via Fx.
var Module = fx.Module("sudo",
	fx.Provide(
		fx.Annotate(
			NewManager,
			fx.As(new(Acquirer)),
		),
		fx.Annotate(
			func() *Builder { return &Builder{} },
			fx.As(new(CmdBuilder)),
		),
	),
)

// TestModule provides noop sudo implementations for unit tests.
var TestModule = fx.Module("sudo-test",
	fx.Provide(
		fx.Annotate(
			func() NoopAcquirer { return NoopAcquirer{} },
			fx.As(new(Acquirer)),
		),
		fx.Annotate(
			func() DirectBuilder { return DirectBuilder{} },
			fx.As(new(CmdBuilder)),
		),
	),
)
