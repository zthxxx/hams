package provider

// GlobalFlags holds flags that appear between `hams` and the provider name.
type GlobalFlags struct {
	Debug   bool
	DryRun  bool
	JSON    bool
	NoColor bool
	Config  string
	Store   string
	Profile string
}
